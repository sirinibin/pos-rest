package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/sirinibin/startpos/backend/db"
	"github.com/sirinibin/startpos/backend/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// ComponentStatus holds the health result for one subsystem.
type ComponentStatus struct {
	OK      bool   `json:"ok"`
	Message string `json:"message"`
}

// ServerStatus is the full health picture for one environment (production or test).
type ServerStatus struct {
	Name      string          `json:"name"`
	Service   string          `json:"service"`
	Port      int             `json:"port"`
	BuildDir  string          `json:"build_dir"`
	Overall   string          `json:"overall"`          // running | degraded | down | starting | stopping | restarting
	Reason    string          `json:"reason,omitempty"` // non-empty only when not "running"
	API       ComponentStatus `json:"api"`
	Redis     ComponentStatus `json:"redis"`
	MongoDB   ComponentStatus `json:"mongodb"`
	Frontend  ComponentStatus `json:"frontend"`
	UpdatedAt time.Time       `json:"updated_at"`
}

// isBuildHealthy returns true only when the directory has a non-empty index.html,
// at least one JS bundle under static/js/, and one CSS bundle under static/css/.
func isBuildHealthy(dir string) (bool, string) {
	info, err := os.Stat(filepath.Join(dir, "index.html"))
	if err != nil || info.Size() == 0 {
		return false, "index.html missing or empty"
	}
	jsMatches, _ := filepath.Glob(filepath.Join(dir, "static", "js", "*.js"))
	if len(jsMatches) == 0 {
		return false, "no JS bundle found in static/js/"
	}
	cssMatches, _ := filepath.Glob(filepath.Join(dir, "static", "css", "*.css"))
	if len(cssMatches) == 0 {
		return false, "no CSS bundle found in static/css/"
	}
	return true, "OK"
}

// checkFrontendBuild reports whether the live build directory is fully intact.
func checkFrontendBuild(buildDir string) ComponentStatus {
	if buildDir == "" {
		return ComponentStatus{true, "N/A"}
	}
	ok, reason := isBuildHealthy(buildDir)
	if !ok {
		return ComponentStatus{false, reason + " — build directory not deployed"}
	}
	return ComponentStatus{true, "OK"}
}

// frontendBuildDir maps env name to its live build path on disk.
func frontendBuildDir(env string) string {
	switch env {
	case "production":
		return "/home/ubuntu/reactjs-pos/build"
	case "test":
		return "/home/ubuntu/reactjs-pos-test/build"
	}
	return ""
}

func frontendSiteURL(env string) string {
	switch env {
	case "production":
		return "https://startpos.gulfunionozone.com/"
	case "test":
		return "https://test.gulfunionozone.com/"
	}
	return ""
}

func checkSiteHTTP(url string) error {
	if url == "" {
		return nil
	}
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode >= 500 {
		return fmt.Errorf("site returned HTTP %d", resp.StatusCode)
	}
	return nil
}

// checkServiceState returns the raw systemctl is-active string (active, inactive, failed, etc.).
// Does not require sudo; read-only.
func checkServiceState(service string) string {
	out, _ := exec.Command("systemctl", "is-active", service).Output()
	return strings.TrimSpace(string(out))
}

// checkRedis pings the shared Redis instance.
func checkRedis() ComponentStatus {
	if db.RedisClient == nil {
		return ComponentStatus{false, "client not initialized"}
	}
	_, err := db.RedisClient.Ping().Result()
	if err != nil {
		return ComponentStatus{false, "not responding: " + err.Error()}
	}
	return ComponentStatus{true, "OK"}
}

// checkMongoDB pings the MongoDB instance using the already-connected posDB client.
func checkMongoDB() ComponentStatus {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	client := db.Client(db.GetPosDB())
	if client == nil {
		return ComponentStatus{false, "client not initialized"}
	}
	if err := client.Ping(ctx, nil); err != nil {
		return ComponentStatus{false, "not responding: " + err.Error()}
	}
	return ComponentStatus{true, "OK"}
}

// buildServerStatus performs all health checks for one environment.
func buildServerStatus(name, service string, port int, buildDir string) ServerStatus {
	s := ServerStatus{
		Name:      name,
		Service:   service,
		Port:      port,
		BuildDir:  buildDir,
		UpdatedAt: time.Now(),
	}
	s.Frontend = checkFrontendBuild(buildDir)

	// --- API service state via systemctl ---
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
		s.API = ComponentStatus{false, "Service failed — check journalctl"}
		s.Overall = "down"
	case "inactive":
		s.API = ComponentStatus{false, "Service stopped"}
		s.Overall = "down"
	default:
		s.API = ComponentStatus{false, "Unknown state: " + state}
		s.Overall = "down"
	}

	// --- Redis and MongoDB (shared instance, same for both envs) ---
	s.Redis = checkRedis()
	s.MongoDB = checkMongoDB()

	// --- Compute overall if not already set by a transitional state ---
	if s.Overall == "" {
		// API is active
		if s.Redis.OK && s.MongoDB.OK {
			s.Overall = "running"
		} else {
			s.Overall = "degraded"
			var parts []string
			if !s.Redis.OK {
				parts = append(parts, "Redis: "+s.Redis.Message)
			}
			if !s.MongoDB.OK {
				parts = append(parts, "MongoDB: "+s.MongoDB.Message)
			}
			s.Reason = strings.Join(parts, "; ")
		}
	} else if s.Overall == "down" && s.Reason == "" {
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

// requireAdminStateless authenticates using JWT signature only — no Redis, no MongoDB.
// Works even when those services are degraded, which is exactly when this endpoint matters.
// Admin/role are embedded in the JWT at login time (see generateJWTToken).
// Falls back to a full DB lookup for tokens that pre-date the admin-claim change.
func requireAdminStateless(w http.ResponseWriter, r *http.Request) bool {
	tokenStr, err := models.ParseAccessTokenFromRequest(r)
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"})
		return false
	}

	jwtToken, err := models.IsJWTTokenValid(tokenStr)
	if err != nil || !jwtToken.Valid {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid or expired token"})
		return false
	}

	claims, _ := models.GetJWTClaims(jwtToken)

	// Fast path: admin flag is embedded in the JWT (tokens issued after the claim change).
	if claims.Admin || claims.Role == "Admin" {
		return true
	}

	// Slow path: old token without embedded admin claim — try MongoDB lookup.
	// If MongoDB is down this will fail, but that's acceptable for pre-change tokens.
	userID, err := primitive.ObjectIDFromHex(claims.UserID)
	if err != nil {
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]string{"error": "admin only"})
		return false
	}
	user, _ := models.FindUserByID(&userID, bson.M{})
	if user != nil && (user.Admin || user.Role == "Admin") {
		return true
	}

	w.WriteHeader(http.StatusForbidden)
	json.NewEncoder(w).Encode(map[string]string{"error": "admin only"})
	return false
}

// GetServerStatusHandler — GET /v1/admin/server-status
// Returns health for both production and test environments.
func GetServerStatusHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if !requireAdminStateless(w, r) {
		return
	}

	production := buildServerStatus("Production", "start-api.service", 2000, frontendBuildDir("production"))
	test := buildServerStatus("Test", "start-api-test.service", 2002, frontendBuildDir("test"))

	json.NewEncoder(w).Encode(map[string]interface{}{
		"production": production,
		"test":       test,
	})
}

// RestartServerHandler — POST /v1/admin/server-restart
// Body: {"env": "production" | "test"}
// Fires systemctl restart in a goroutine and returns 202 immediately.
// For production, the process will die during restart — the 202 is flushed first.
func RestartServerHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
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
	json.NewEncoder(w).Encode(map[string]interface{}{
		"ok":      true,
		"message": "Restart initiated for " + service,
	})
	// Flush response before we potentially kill ourselves (production restart)
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}

	go func() {
		time.Sleep(300 * time.Millisecond)
		exec.Command("systemctl", "restart", service).Run()
	}()
}

// RepairFrontendHandler — POST /v1/admin/repair-frontend
// Body: {"env": "production" | "test" | "both"}
// Ensures the live build directory exists and has an index.html.
// If the directory is empty or missing, restores from build_new or build_old.
func RepairFrontendHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if !requireAdminStateless(w, r) {
		return
	}

	var req struct {
		Env string `json:"env"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid body"})
		return
	}

	var envs []string
	switch req.Env {
	case "production", "test":
		envs = []string{req.Env}
	case "both", "":
		envs = []string{"production", "test"}
	default:
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "env must be production, test, or both"})
		return
	}

	results := map[string]interface{}{}
	for _, env := range envs {
		buildDir := frontendBuildDir(env)
		status := repairFrontendBuild(buildDir, frontendSiteURL(env))
		results[env] = status
	}
	json.NewEncoder(w).Encode(map[string]interface{}{"results": results})
}

type repairResult struct {
	Fixed   bool   `json:"fixed"`
	AlreadyOK bool `json:"already_ok"`
	Message string `json:"message"`
}

// repairFrontendBuild ensures the live build directory has a valid index.html.
// It creates the directory and copies content from build_new or build_old if needed.
func repairFrontendBuild(buildDir, siteURL string) repairResult {
	if ok, _ := isBuildHealthy(buildDir); ok {
		if err := checkSiteHTTP(siteURL); err == nil {
			return repairResult{AlreadyOK: true, Message: "Build directory is healthy, no action needed."}
		}
		// Files look fine but site is returning 5xx — fall through to restore
	}

	if err := os.MkdirAll(buildDir, 0755); err != nil {
		return repairResult{Message: "Failed to create build directory: " + err.Error()}
	}

	base := filepath.Dir(buildDir)
	for _, src := range []string{
		filepath.Join(base, "build_new"),
		filepath.Join(base, "build_old"),
	} {
		if ok, _ := isBuildHealthy(src); !ok {
			continue
		}
		out, err := exec.Command("cp", "-a", src+"/.", buildDir+"/").CombinedOutput()
		if err != nil {
			continue
		}
		_ = out
		if httpErr := checkSiteHTTP(siteURL); httpErr != nil {
			continue
		}
		return repairResult{Fixed: true, Message: "Restored from " + filepath.Base(src) + "."}
	}

	return repairResult{Message: "No valid source found (build_new and build_old are both missing, incomplete, or still return 500)."}
}
