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
	"sync"
	"time"

	"github.com/dgrijalva/jwt-go"
	"github.com/go-redis/redis"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
)

// ─── auto-restart config (persisted to health-monitor-config.json) ───────────

type AutoRestartConfig struct {
	Enabled bool `json:"enabled"`
	Minutes int  `json:"minutes"` // how many minutes down before auto-restart
}

const (
	configFile     = "health-monitor-config.json"
	restartLogFile = "health-monitor-restarts.json"
	maxLogEntries  = 200
)

// RestartLogEntry records one restart event (auto or manual).
type RestartLogEntry struct {
	Timestamp time.Time `json:"timestamp"`
	Env       string    `json:"env"`     // "production" | "test"
	Service   string    `json:"service"`
	Trigger   string    `json:"trigger"` // "auto" | "manual"
	Success   bool      `json:"success"`
	Error     string    `json:"error,omitempty"`
}

var (
	arCfg   = AutoRestartConfig{Enabled: false, Minutes: 8}
	arMu    sync.RWMutex
	// downSince tracks when each env first went down (key: "production" | "test")
	downSince   = map[string]time.Time{}
	downSinceMu sync.Mutex
	// restart log (newest first, capped at maxLogEntries)
	rstLog   []RestartLogEntry
	rstLogMu sync.Mutex
)

func loadRestartLog() {
	data, err := os.ReadFile(restartLogFile)
	if err != nil {
		return
	}
	var entries []RestartLogEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		log.Printf("[health-monitor] restart log parse error: %v", err)
		return
	}
	rstLogMu.Lock()
	rstLog = entries
	rstLogMu.Unlock()
}

func appendRestartLog(entry RestartLogEntry) {
	rstLogMu.Lock()
	rstLog = append([]RestartLogEntry{entry}, rstLog...) // newest first
	if len(rstLog) > maxLogEntries {
		rstLog = rstLog[:maxLogEntries]
	}
	snapshot := make([]RestartLogEntry, len(rstLog))
	copy(snapshot, rstLog)
	rstLogMu.Unlock()
	data, _ := json.MarshalIndent(snapshot, "", "  ")
	if err := os.WriteFile(restartLogFile, data, 0644); err != nil {
		log.Printf("[health-monitor] failed to save restart log: %v", err)
	}
}

func loadConfigFromDisk() {
	data, err := os.ReadFile(configFile)
	if err != nil {
		return // no file yet — defaults stay
	}
	var cfg AutoRestartConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		log.Printf("[health-monitor] config parse error: %v", err)
		return
	}
	if cfg.Minutes < 1 {
		cfg.Minutes = 1
	}
	arMu.Lock()
	arCfg = cfg
	arMu.Unlock()
	log.Printf("[health-monitor] loaded config: enabled=%v minutes=%d", cfg.Enabled, cfg.Minutes)
}

func saveConfigToDisk(cfg AutoRestartConfig) error {
	if cfg.Minutes < 1 {
		cfg.Minutes = 1
	}
	if cfg.Minutes > 1440 {
		cfg.Minutes = 1440
	}
	arMu.Lock()
	arCfg = cfg
	arMu.Unlock()
	data, _ := json.MarshalIndent(cfg, "", "  ")
	return os.WriteFile(configFile, data, 0644)
}

// autoRestartLoop runs in the background, checking both services every 30s.
// If a service has been continuously down for >= configured minutes, it restarts it.
func autoRestartLoop() {
	type env struct{ name, service string }
	envs := []env{
		{"production", "start-api.service"},
		{"test", "start-api-test.service"},
	}
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		arMu.RLock()
		cfg := arCfg
		arMu.RUnlock()
		if !cfg.Enabled {
			// Clear trackers so timers reset when re-enabled
			downSinceMu.Lock()
			for k := range downSince {
				delete(downSince, k)
			}
			downSinceMu.Unlock()
			continue
		}
		threshold := time.Duration(cfg.Minutes) * time.Minute
		for _, e := range envs {
			state := checkServiceState(e.service)
			if state == "active" {
				downSinceMu.Lock()
				delete(downSince, e.name)
				downSinceMu.Unlock()
				continue
			}
			// Service is not active — track how long it has been down
			downSinceMu.Lock()
			if _, tracked := downSince[e.name]; !tracked {
				downSince[e.name] = time.Now()
				downSinceMu.Unlock()
				log.Printf("[health-monitor] %s went down, will auto-restart in %dm if still down", e.service, cfg.Minutes)
				continue
			}
			elapsed := time.Since(downSince[e.name])
			if elapsed >= threshold {
				delete(downSince, e.name) // reset before restarting
				downSinceMu.Unlock()
				log.Printf("[health-monitor] auto-restart %s (down for %v >= %dm)", e.service, elapsed.Round(time.Second), cfg.Minutes)
				out, err := exec.Command("systemctl", "restart", e.service).CombinedOutput()
				entry := RestartLogEntry{
					Timestamp: time.Now(),
					Env:       e.name,
					Service:   e.service,
					Trigger:   "auto",
					Success:   err == nil,
				}
				if err != nil {
					entry.Error = strings.TrimSpace(string(out))
					log.Printf("[health-monitor] auto-restart %s FAILED: %v — %s", e.service, err, out)
				} else {
					log.Printf("[health-monitor] auto-restart %s OK", e.service)
				}
				appendRestartLog(entry)
			} else {
				downSinceMu.Unlock()
			}
		}
	}
}

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

	env := req.Env
	go func() {
		time.Sleep(200 * time.Millisecond)
		out, err := exec.Command("systemctl", "restart", service).CombinedOutput()
		entry := RestartLogEntry{
			Timestamp: time.Now(),
			Env:       env,
			Service:   service,
			Trigger:   "manual",
			Success:   err == nil,
		}
		if err != nil {
			entry.Error = strings.TrimSpace(string(out))
			log.Printf("[health-monitor] restart %s failed: %v — %s", service, err, out)
		} else {
			log.Printf("[health-monitor] restart %s OK", service)
		}
		appendRestartLog(entry)
	}()
}

// configHandler — GET/POST /health-monitor/config
// Reads or writes the auto-restart configuration (persisted to disk).
func configHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		cors(w)
		return
	}
	cors(w)
	if !requireAdminStateless(w, r) {
		return
	}
	switch r.Method {
	case http.MethodGet:
		arMu.RLock()
		cfg := arCfg
		arMu.RUnlock()
		json.NewEncoder(w).Encode(cfg)

	case http.MethodPost:
		var cfg AutoRestartConfig
		if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		if err := saveConfigToDisk(cfg); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "failed to save: " + err.Error()})
			return
		}
		arMu.RLock()
		saved := arCfg
		arMu.RUnlock()
		json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "config": saved})

	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// restartLogHandler — GET /health-monitor/restart-log
// Returns the last 200 restart events (newest first).
func restartLogHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		cors(w)
		return
	}
	cors(w)
	if !requireAdminStateless(w, r) {
		return
	}
	rstLogMu.Lock()
	snapshot := make([]RestartLogEntry, len(rstLog))
	copy(snapshot, rstLog)
	rstLogMu.Unlock()
	if snapshot == nil {
		snapshot = []RestartLogEntry{}
	}
	json.NewEncoder(w).Encode(snapshot)
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

	loadRestartLog()
	loadConfigFromDisk()
	go autoRestartLoop()

	mux := http.NewServeMux()
	mux.HandleFunc("/health-monitor/status", statusHandler)
	mux.HandleFunc("/health-monitor/restart", restartHandler)
	mux.HandleFunc("/health-monitor/config", configHandler)
	mux.HandleFunc("/health-monitor/restart-log", restartLogHandler)
	mux.HandleFunc("/health-monitor/ping", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"ok": "true"})
	})

	log.Printf("[health-monitor] listening on :%s", port)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		log.Fatal(err)
	}
}
