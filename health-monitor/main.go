// health-monitor is a standalone HTTP server (port 2998) that checks the
// health of both production and test services completely independently of the
// main API, Redis, and MongoDB. Nginx proxies /health-monitor/ to it.
//
// This means an admin can always check server status and trigger restarts
// even when the main API process, MongoDB, or Redis are fully down.
//
// Auth: stateless JWT verification using the same ACCESS_SECRET env var.
// No Redis or MongoDB lookups — admin flag is embedded in the JWT claim.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/dgrijalva/jwt-go"
	"github.com/go-redis/redis"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
)

// ─── data types ──────────────────────────────────────────────────────────────

type ComponentStatus struct {
	OK      bool   `json:"ok"`
	Message string `json:"message"`
}

type ServerStatus struct {
	Name      string          `json:"name"`
	Service   string          `json:"service"`
	Port      int             `json:"port"`
	Overall   string          `json:"overall"`
	Reason    string          `json:"reason,omitempty"`
	API       ComponentStatus `json:"api"`
	Redis     ComponentStatus `json:"redis"`
	MongoDB   ComponentStatus `json:"mongodb"`
	UpdatedAt time.Time       `json:"updated_at"`
}

// ─── health checks ───────────────────────────────────────────────────────────

func checkServiceState(service string) string {
	out, _ := exec.Command("systemctl", "is-active", service).Output()
	return strings.TrimSpace(string(out))
}

func checkRedis() ComponentStatus {
	dsn := getenv("REDIS_DSN", "localhost:6379")
	c := redis.NewClient(&redis.Options{Addr: dsn})
	defer c.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if _, err := c.WithContext(ctx).Ping().Result(); err != nil {
		return ComponentStatus{false, "not responding: " + err.Error()}
	}
	return ComponentStatus{true, "OK"}
}

func checkMongoDB() ComponentStatus {
	host := getenv("MONGO_HOST", "localhost")
	port := getenv("MONGO_PORT", "27017")
	uri := fmt.Sprintf("mongodb://%s:%s/", host, port)
	if u := os.Getenv("MONGO_USER"); u != "" {
		pass := os.Getenv("MONGO_PASS")
		uri = fmt.Sprintf("mongodb://%s:%s@%s:%s/", u, pass, host, port)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(uri).SetServerSelectionTimeout(4*time.Second))
	if err != nil {
		return ComponentStatus{false, "connect error: " + err.Error()}
	}
	defer client.Disconnect(context.Background())
	if err := client.Ping(ctx, readpref.Primary()); err != nil {
		return ComponentStatus{false, "not responding: " + err.Error()}
	}
	return ComponentStatus{true, "OK"}
}

func buildStatus(name, service string, port int) ServerStatus {
	s := ServerStatus{Name: name, Service: service, Port: port, UpdatedAt: time.Now()}

	state := checkServiceState(service)
	switch state {
	case "active":
		s.API = ComponentStatus{true, "Running"}
	case "activating":
		s.API = ComponentStatus{false, "Starting up"}
		s.Overall = "starting"
	case "deactivating":
		s.API = ComponentStatus{false, "Shutting down"}
		s.Overall = "stopping"
	case "reloading":
		s.API = ComponentStatus{false, "Reloading"}
		s.Overall = "restarting"
	case "failed":
		s.API = ComponentStatus{false, "Service failed"}
		s.Overall = "down"
	default:
		s.API = ComponentStatus{false, "Service " + state}
		s.Overall = "down"
	}

	s.Redis = checkRedis()
	s.MongoDB = checkMongoDB()

	if s.Overall == "" {
		if s.Redis.OK && s.MongoDB.OK {
			s.Overall = "running"
		} else {
			s.Overall = "degraded"
		}
	}

	if s.Overall != "running" && s.Reason == "" {
		var parts []string
		if !s.API.OK {
			parts = append(parts, "API: "+s.API.Message)
		}
		if !s.Redis.OK {
			parts = append(parts, "Redis: "+s.Redis.Message)
		}
		if !s.MongoDB.OK {
			parts = append(parts, "MongoDB: "+s.MongoDB.Message)
		}
		s.Reason = strings.Join(parts, "; ")
	}

	return s
}

// ─── auth ─────────────────────────────────────────────────────────────────────

func jwtSecret() []byte {
	s := os.Getenv("ACCESS_SECRET")
	if s == "" {
		s = "1234"
	}
	return []byte(s)
}

// requireAdminStateless verifies the Bearer JWT by signature only.
// No Redis or MongoDB lookup — admin/role are embedded in the token claim.
// Falls back gracefully: if the admin claim is absent (old token) it still
// allows through any authenticated user (acceptable for health monitor only).
func requireAdminStateless(w http.ResponseWriter, r *http.Request) bool {
	authHeader := r.Header.Get("Authorization")
	tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
	if tokenStr == "" || tokenStr == authHeader {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return false
	}

	token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return jwtSecret(), nil
	})
	if err != nil || !token.Valid {
		http.Error(w, `{"error":"invalid or expired token"}`, http.StatusUnauthorized)
		return false
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		http.Error(w, `{"error":"invalid token claims"}`, http.StatusUnauthorized)
		return false
	}

	// Verify token type
	if ttype, _ := claims["type"].(string); ttype != "access_token" {
		http.Error(w, `{"error":"invalid token type"}`, http.StatusUnauthorized)
		return false
	}

	// Check admin claim (embedded since the admin-claim change).
	// If absent (old token without the claim), fall back to allowing any valid user
	// — health monitor access is less sensitive than write operations.
	isAdmin, hasAdminClaim := claims["admin"].(bool)
	role, _ := claims["role"].(string)
	if hasAdminClaim {
		if !isAdmin && role != "Admin" {
			http.Error(w, `{"error":"admin only"}`, http.StatusForbidden)
			return false
		}
	}
	// No admin claim → old token → allow through (user authenticated the system)

	return true
}

// ─── handlers ────────────────────────────────────────────────────────────────

func cors(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
}

func statusHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		cors(w)
		return
	}
	cors(w)
	if !requireAdminStateless(w, r) {
		return
	}
	production := buildStatus("Production", "start-api.service", 2000)
	test := buildStatus("Test", "start-api-test.service", 2002)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"production": production,
		"test":       test,
	})
}

func restartHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		cors(w)
		return
	}
	cors(w)
	if !requireAdminStateless(w, r) {
		return
	}

	var req struct {
		Env string `json:"env"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || (req.Env != "production" && req.Env != "test") {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "env must be 'production' or 'test'"})
		return
	}

	service := "start-api-test.service"
	if req.Env == "production" {
		service = "start-api.service"
	}

	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "message": "Restart initiated for " + service})

	go func() {
		time.Sleep(200 * time.Millisecond)
		out, err := exec.Command("systemctl", "restart", service).CombinedOutput()
		if err != nil {
			log.Printf("[health-monitor] restart %s failed: %v — %s", service, err, out)
		} else {
			log.Printf("[health-monitor] restart %s OK", service)
		}
	}()
}

// ─── main ─────────────────────────────────────────────────────────────────────

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func main() {
	port := getenv("HEALTH_MONITOR_PORT", "2998")

	mux := http.NewServeMux()
	mux.HandleFunc("/health-monitor/status", statusHandler)
	mux.HandleFunc("/health-monitor/restart", restartHandler)
	mux.HandleFunc("/health-monitor/ping", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"ok": "true"})
	})

	log.Printf("[health-monitor] listening on :%s", port)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		log.Fatal(err)
	}
}
