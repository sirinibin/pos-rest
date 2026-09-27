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
	"path/filepath"
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
	Enabled                bool     `json:"enabled"`
	Minutes                int      `json:"minutes"`                  // how many minutes down before auto-restart
	AllowedUsers           []string `json:"allowed_users"`            // non-admin user IDs permitted to view status
	RedisService           string   `json:"redis_service"`            // default: "redis.service"
	MongoService           string   `json:"mongo_service"`            // default: "mongod.service"
	FrontendAutoFix        bool     `json:"frontend_auto_fix"`        // auto-repair build/ when index.html missing
	FrontendAutoFixMinutes int      `json:"frontend_auto_fix_minutes"` // minutes before repair triggers (default 3)
}

func (c *AutoRestartConfig) redisService() string {
	if c.RedisService != "" {
		return c.RedisService
	}
	return "redis.service"
}

func (c *AutoRestartConfig) mongoService() string {
	if c.MongoService != "" {
		return c.MongoService
	}
	return "mongod.service"
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
	arCfg   = AutoRestartConfig{Enabled: false, Minutes: 8, FrontendAutoFixMinutes: 3}
	arMu    sync.RWMutex
	// downSince tracks when each env first went down (key: "production" | "test")
	downSince   = map[string]time.Time{}
	downSinceMu sync.Mutex
	// frontendDownSince tracks when each env's frontend first went missing
	frontendDownSince   = map[string]time.Time{}
	frontendDownSinceMu sync.Mutex
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

// tryAutoRestart checks whether a named service (by downSince key) is down.
// If it has been down longer than threshold it issues systemctl restart and logs the result.
// Returns true if the service is currently active (no restart needed).
func tryAutoRestart(key, service, envName string, threshold time.Duration, cfg AutoRestartConfig) {
	state := checkServiceState(service)
	if state == "active" {
		downSinceMu.Lock()
		delete(downSince, key)
		downSinceMu.Unlock()
		return
	}
	downSinceMu.Lock()
	if _, tracked := downSince[key]; !tracked {
		downSince[key] = time.Now()
		downSinceMu.Unlock()
		log.Printf("[health-monitor] %s is down, will auto-restart in %dm if still down", service, cfg.Minutes)
		return
	}
	elapsed := time.Since(downSince[key])
	if elapsed < threshold {
		downSinceMu.Unlock()
		return
	}
	delete(downSince, key)
	downSinceMu.Unlock()
	log.Printf("[health-monitor] auto-restart %s (down for %v >= %dm)", service, elapsed.Round(time.Second), cfg.Minutes)
	out, err := exec.Command("systemctl", "restart", service).CombinedOutput()
	entry := RestartLogEntry{
		Timestamp: time.Now(),
		Env:       envName,
		Service:   service,
		Trigger:   "auto",
		Success:   err == nil,
	}
	if err != nil {
		entry.Error = strings.TrimSpace(string(out))
		log.Printf("[health-monitor] auto-restart %s FAILED: %v — %s", service, err, out)
	} else {
		log.Printf("[health-monitor] auto-restart %s OK", service)
	}
	appendRestartLog(entry)
}

// autoRestartLoop runs in the background every 30s.
// Checks API services per-environment, plus the shared Redis and MongoDB services.
// Only restarts services that are actually down and have been for >= configured minutes.
func autoRestartLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		arMu.RLock()
		cfg := arCfg
		arMu.RUnlock()
		if !cfg.Enabled {
			downSinceMu.Lock()
			for k := range downSince {
				delete(downSince, k)
			}
			downSinceMu.Unlock()
			continue
		}
		threshold := time.Duration(cfg.Minutes) * time.Minute
		// Per-environment API services
		tryAutoRestart("production-api", "start-api.service", "production", threshold, cfg)
		tryAutoRestart("test-api", "start-api-test.service", "test", threshold, cfg)
		// Shared infrastructure — only restart once per cycle, not per env
		tryAutoRestart("redis", cfg.redisService(), "shared", threshold, cfg)
		tryAutoRestart("mongo", cfg.mongoService(), "shared", threshold, cfg)
	}
}

// frontendAutoFixLoop runs every 5 seconds.
// When FrontendAutoFix is enabled and an env's build directory has been missing
// its index.html for >= FrontendAutoFixMinutes, it calls repairFrontendBuild.
func frontendAutoFixLoop() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		arMu.RLock()
		cfg := arCfg
		arMu.RUnlock()
		if !cfg.FrontendAutoFix {
			frontendDownSinceMu.Lock()
			for k := range frontendDownSince {
				delete(frontendDownSince, k)
			}
			frontendDownSinceMu.Unlock()
			continue
		}
		minutes := cfg.FrontendAutoFixMinutes
		if minutes < 1 {
			minutes = 3
		}
		threshold := time.Duration(minutes) * time.Minute
		for _, env := range []string{"production", "test"} {
			buildDir := frontendBuildDir(env)
			st := checkFrontendBuild(buildDir)
			if st.OK {
				frontendDownSinceMu.Lock()
				delete(frontendDownSince, env)
				frontendDownSinceMu.Unlock()
				continue
			}
			frontendDownSinceMu.Lock()
			if _, tracked := frontendDownSince[env]; !tracked {
				frontendDownSince[env] = time.Now()
				frontendDownSinceMu.Unlock()
				log.Printf("[health-monitor] frontend %s is broken, will auto-fix in %dm if still broken", env, minutes)
				continue
			}
			elapsed := time.Since(frontendDownSince[env])
			if elapsed < threshold {
				frontendDownSinceMu.Unlock()
				continue
			}
			delete(frontendDownSince, env)
			frontendDownSinceMu.Unlock()
			log.Printf("[health-monitor] auto-fixing frontend %s (broken for %v >= %dm)", env, elapsed.Round(time.Second), minutes)
			result := repairFrontendBuild(buildDir)
			if result.Fixed {
				log.Printf("[health-monitor] frontend %s auto-fix OK: %s", env, result.Message)
			} else {
				log.Printf("[health-monitor] frontend %s auto-fix FAILED: %s", env, result.Message)
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
	Frontend  ComponentStatus `json:"frontend"`
	UpdatedAt time.Time       `json:"updated_at"`
}

func frontendBuildDir(env string) string {
	switch env {
	case "production":
		return "/home/ubuntu/reactjs-pos/build"
	case "test":
		return "/home/ubuntu/reactjs-pos-test/build"
	}
	return ""
}

// isBuildHealthy returns true only when the directory has:
//   - a non-empty index.html
//   - at least one .js  file under static/js/
//   - at least one .css file under static/css/
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

type repairResult struct {
	Fixed     bool   `json:"fixed"`
	AlreadyOK bool   `json:"already_ok"`
	Message   string `json:"message"`
}

func repairFrontendBuild(buildDir string) repairResult {
	if ok, _ := isBuildHealthy(buildDir); ok {
		return repairResult{AlreadyOK: true, Message: "Build directory is healthy, no action needed."}
	}
	if err := os.MkdirAll(buildDir, 0755); err != nil {
		return repairResult{Message: "Failed to create build directory: " + err.Error()}
	}
	base := filepath.Dir(buildDir)
	for _, src := range []string{
		filepath.Join(base, "build_new"),
		filepath.Join(base, "build_old"),
	} {
		if ok, reason := isBuildHealthy(src); !ok {
			log.Printf("[health-monitor] skipping %s as restore source: %s", filepath.Base(src), reason)
			continue
		}
		out, err := exec.Command("cp", "-a", src+"/.", buildDir+"/").CombinedOutput()
		if err != nil {
			return repairResult{Message: "cp failed from " + src + ": " + err.Error() + " — " + string(out)}
		}
		return repairResult{Fixed: true, Message: "Restored from " + filepath.Base(src) + "."}
	}
	return repairResult{Message: "No valid source found (build_new and build_old are both missing or incomplete)."}
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

func buildStatus(name, service string, port int, buildDir string) ServerStatus {
	s := ServerStatus{Name: name, Service: service, Port: port, UpdatedAt: time.Now()}
	s.Frontend = checkFrontendBuild(buildDir)

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

// parseJWTClaims validates the Bearer token and returns its claims.
// Returns nil claims and writes the error response if invalid.
func parseJWTClaims(w http.ResponseWriter, r *http.Request) jwt.MapClaims {
	authHeader := r.Header.Get("Authorization")
	tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
	if tokenStr == "" || tokenStr == authHeader {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return nil
	}
	token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return jwtSecret(), nil
	})
	if err != nil || !token.Valid {
		http.Error(w, `{"error":"invalid or expired token"}`, http.StatusUnauthorized)
		return nil
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		http.Error(w, `{"error":"invalid token claims"}`, http.StatusUnauthorized)
		return nil
	}
	if ttype, _ := claims["type"].(string); ttype != "access_token" {
		http.Error(w, `{"error":"invalid token type"}`, http.StatusUnauthorized)
		return nil
	}
	return claims
}

// claimsHaveAdminFlag returns (isAdmin, hasFlag).
// hasFlag is false for tokens issued before the admin-claim change (no "admin" key at all).
func claimsHaveAdminFlag(claims jwt.MapClaims) (isAdminUser bool, hasFlag bool) {
	admin, hasFlag := claims["admin"].(bool)
	role, _ := claims["role"].(string)
	return admin || role == "Admin", hasFlag
}

// requireAdminStateless — write/action endpoints (config, restart).
// New tokens: must carry admin=true. Old tokens (no admin key): allowed through
// for backward compat — the valid JWT signature is sufficient evidence of a
// legitimate session, and we can't do a DB lookup from the health monitor.
func requireAdminStateless(w http.ResponseWriter, r *http.Request) bool {
	claims := parseJWTClaims(w, r)
	if claims == nil {
		return false
	}
	isAdminUser, hasFlag := claimsHaveAdminFlag(claims)
	if hasFlag && !isAdminUser {
		http.Error(w, `{"error":"admin only"}`, http.StatusForbidden)
		return false
	}
	// hasFlag=false → old token → allow through
	return true
}

// requireViewAccess — read-only status/log endpoints.
// Allows: admins (new tokens), users in AllowedUsers list, or any valid old token.
func requireViewAccess(w http.ResponseWriter, r *http.Request) bool {
	claims := parseJWTClaims(w, r)
	if claims == nil {
		return false
	}
	isAdminUser, hasFlag := claimsHaveAdminFlag(claims)
	// Admin user or old token without flag → allow
	if !hasFlag || isAdminUser {
		return true
	}
	// New non-admin token: check AllowedUsers list
	userID, _ := claims["user_id"].(string)
	arMu.RLock()
	allowed := arCfg.AllowedUsers
	arMu.RUnlock()
	for _, id := range allowed {
		if id == userID {
			return true
		}
	}
	http.Error(w, `{"error":"access denied"}`, http.StatusForbidden)
	return false
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
	if !requireViewAccess(w, r) {
		return
	}
	production := buildStatus("Production", "start-api.service", 2000, frontendBuildDir("production"))
	test := buildStatus("Test", "start-api-test.service", 2002, frontendBuildDir("test"))
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

	apiService := "start-api-test.service"
	if req.Env == "production" {
		apiService = "start-api.service"
	}

	arMu.RLock()
	cfg := arCfg
	arMu.RUnlock()

	// Determine which services are actually down — only restart those.
	type target struct{ service, key string }
	var toRestart []target
	var skipped []string

	if checkServiceState(apiService) != "active" {
		toRestart = append(toRestart, target{apiService, req.Env + "-api"})
	} else {
		skipped = append(skipped, apiService)
	}
	if checkServiceState(cfg.redisService()) != "active" {
		toRestart = append(toRestart, target{cfg.redisService(), "redis"})
	} else {
		skipped = append(skipped, cfg.redisService())
	}
	if checkServiceState(cfg.mongoService()) != "active" {
		toRestart = append(toRestart, target{cfg.mongoService(), "mongo"})
	} else {
		skipped = append(skipped, cfg.mongoService())
	}

	if len(toRestart) == 0 {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"ok":      true,
			"message": "All services are already running — nothing to restart.",
			"skipped": skipped,
		})
		return
	}

	restartNames := make([]string, len(toRestart))
	for i, t := range toRestart {
		restartNames[i] = t.service
	}
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"ok":        true,
		"restarted": restartNames,
		"skipped":   skipped,
	})
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}

	envName := req.Env
	go func() {
		time.Sleep(200 * time.Millisecond)
		for _, t := range toRestart {
			out, err := exec.Command("systemctl", "restart", t.service).CombinedOutput()
			entry := RestartLogEntry{
				Timestamp: time.Now(),
				Env:       envName,
				Service:   t.service,
				Trigger:   "manual",
				Success:   err == nil,
			}
			if err != nil {
				entry.Error = strings.TrimSpace(string(out))
				log.Printf("[health-monitor] restart %s failed: %v — %s", t.service, err, out)
			} else {
				// Clear auto-restart downSince so the loop doesn't re-trigger immediately
				downSinceMu.Lock()
				delete(downSince, t.key)
				downSinceMu.Unlock()
				log.Printf("[health-monitor] restart %s OK", t.service)
			}
			appendRestartLog(entry)
		}
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
// Returns the last 200 restart events (newest first). Allowed for all view-access users.
func restartLogHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		cors(w)
		return
	}
	cors(w)
	if !requireViewAccess(w, r) {
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

// repairFrontendHandler — POST /health-monitor/repair-frontend
// Body: {"env": "production" | "test" | "both"}
func repairFrontendHandler(w http.ResponseWriter, r *http.Request) {
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
		results[env] = repairFrontendBuild(frontendBuildDir(env))
	}
	json.NewEncoder(w).Encode(map[string]interface{}{"results": results})
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
	go frontendAutoFixLoop()

	mux := http.NewServeMux()
	mux.HandleFunc("/health-monitor/status", statusHandler)
	mux.HandleFunc("/health-monitor/restart", restartHandler)
	mux.HandleFunc("/health-monitor/config", configHandler)
	mux.HandleFunc("/health-monitor/restart-log", restartLogHandler)
	mux.HandleFunc("/health-monitor/repair-frontend", repairFrontendHandler)
	mux.HandleFunc("/health-monitor/ping", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"ok": "true"})
	})

	log.Printf("[health-monitor] listening on :%s", port)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		log.Fatal(err)
	}
}
