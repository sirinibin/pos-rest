package controller

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/sirinibin/startpos/backend/db"
	"go.mongodb.org/mongo-driver/bson"
)

// ServiceHealth holds the result of a single service check
type ServiceHealth struct {
	Status  string `json:"status"`            // "ok" or "error"
	Port    string `json:"port"`              // port the service is running on
	Message string `json:"message"`           // human-readable detail
	Latency string `json:"latency,omitempty"` // round-trip time
}

// HealthResponse is the full /v1/health response body
type HealthResponse struct {
	OK      bool          `json:"ok"` // true only if ALL services are healthy
	Redis   ServiceHealth `json:"redis"`
	MongoDB ServiceHealth `json:"mongodb"`
	Checked string        `json:"checked_at"` // RFC3339 timestamp
}

// redisPort extracts the port from the REDIS_DSN env var ("host:port")
func redisPort() string {
	dsn := os.Getenv("REDIS_DSN")
	if dsn == "" {
		return "6379"
	}
	parts := strings.SplitN(dsn, ":", 2)
	if len(parts) == 2 {
		return parts[1]
	}
	return dsn
}

// mongoPort returns the MONGO_PORT env var (defaults to 27017)
func mongoPort() string {
	if p := os.Getenv("MONGO_PORT"); p != "" {
		return p
	}
	return "27017"
}

// HealthCheck godoc
// GET /v1/health — no authentication required
// Returns the connection status of Redis and MongoDB.
func HealthCheck(w http.ResponseWriter, r *http.Request) {
	resp := HealthResponse{
		Checked: time.Now().UTC().Format(time.RFC3339),
	}

	// ── Redis ──────────────────────────────────────────────────────────────
	redisStart := time.Now()
	if db.RedisClient == nil {
		resp.Redis = ServiceHealth{Status: "error", Port: redisPort(), Message: "Redis client not initialised"}
	} else {
		_, err := db.RedisClient.Ping().Result()
		latency := time.Since(redisStart).Round(time.Millisecond).String()
		if err != nil {
			resp.Redis = ServiceHealth{Status: "error", Port: redisPort(), Message: err.Error(), Latency: latency}
		} else {
			resp.Redis = ServiceHealth{Status: "ok", Port: redisPort(), Message: "pong", Latency: latency}
		}
	}

	// ── MongoDB ────────────────────────────────────────────────────────────
	mongoStart := time.Now()
	mongoClient := db.Client("")
	if mongoClient == nil {
		resp.MongoDB = ServiceHealth{Status: "error", Port: mongoPort(), Message: "MongoDB client not initialised"}
	} else {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		err := mongoClient.Database(db.GetPosDB()).RunCommand(ctx, bson.D{{Key: "ping", Value: 1}}).Err()
		latency := time.Since(mongoStart).Round(time.Millisecond).String()
		if err != nil {
			resp.MongoDB = ServiceHealth{Status: "error", Port: mongoPort(), Message: err.Error(), Latency: latency}
		} else {
			resp.MongoDB = ServiceHealth{Status: "ok", Port: mongoPort(), Message: "pong", Latency: latency}
		}
	}

	// ── Overall ────────────────────────────────────────────────────────────
	resp.OK = resp.Redis.Status == "ok" && resp.MongoDB.Status == "ok"

	w.Header().Set("Content-Type", "application/json")
	if resp.OK {
		w.WriteHeader(http.StatusOK)
	} else {
		w.WriteHeader(http.StatusServiceUnavailable)
	}
	json.NewEncoder(w).Encode(resp)
}
