package models

import (
	"bytes"
	"context"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/sirinibin/startpos/backend/db"
	"github.com/sirinibin/startpos/backend/env"
)

// biScripts lists the Python BI scripts to run per store, in execution order.
// Each script accepts a --store_id <store_db_name> argument and writes its results
// directly into MongoDB (customer/product documents + history collections).
var biScripts = []string{
	"churn_prediction.py",
	"customer_lifetime_value.py",
	"customer_cohort_retention.py",
	"sales_velocity_trend.py",
	"abc_xyz_analysis.py",
}

// ansiRe matches ANSI terminal escape sequences (colours, cursor moves, etc.)
// so they can be stripped before writing to the application log.
var ansiRe = regexp.MustCompile(`\x1b(\[[0-9;]*[A-Za-z]|\(B)`)

// logWriter streams subprocess stdout/stderr line-by-line to the Go logger.
// It strips ANSI escape codes and skips blank lines so the log stays readable.
type logWriter struct {
	prefix string
	buf    []byte
}

func (lw *logWriter) Write(p []byte) (int, error) {
	lw.buf = append(lw.buf, p...)
	for {
		idx := bytes.IndexByte(lw.buf, '\n')
		if idx < 0 {
			break
		}
		raw := string(lw.buf[:idx])
		lw.buf = lw.buf[idx+1:]
		line := ansiRe.ReplaceAllString(raw, "")
		line = strings.ReplaceAll(line, "\r", "")
		line = strings.TrimSpace(line)
		if line != "" {
			log.Print(lw.prefix + line)
		}
	}
	return len(p), nil
}

// RunBIJobForAllStores iterates every store in the central `pos` database and
// runs all 5 BI Python scripts for each store.
// Called by the gocron scheduler in main.go every 3 hours.
func RunBIJobForAllStores() {
	biDir := env.Getenv("BI_SCRIPTS_DIR", "")
	if biDir == "" {
		log.Print("[BI] BI_SCRIPTS_DIR env var not set — skipping BI job")
		return
	}

	python := env.Getenv("BI_PYTHON", "python3")

	stores, err := GetAllStores()
	if err != nil {
		log.Printf("[BI] Failed to load stores: %v", err)
		return
	}

	log.Printf("[BI] Starting BI job for %d stores", len(stores))
	start := time.Now()

	for _, store := range stores {
		if store.Deleted {
			continue
		}
		storeDBName := "store_" + store.ID.Hex()
		log.Printf("[BI] Processing store: %s (%s)", store.Name, storeDBName)

		for _, script := range biScripts {
			scriptPath := filepath.Join(biDir, script)
			runBIScript(python, scriptPath, storeDBName)
		}
	}

	log.Printf("[BI] All stores processed in %s", time.Since(start).Round(time.Second))
}

// runBIScript executes a single Python BI script with STORE_DB_NAME injected
// via environment variable. Output (stdout+stderr) is streamed live to the app
// log with ANSI escape codes stripped.
func runBIScript(python, scriptPath, storeDBName string) {
	name := filepath.Base(scriptPath)
	log.Printf("[BI] Running %s [%s]", name, storeDBName)
	scriptStart := time.Now()

	cmd := exec.Command(python, scriptPath) // #nosec G204 — path from trusted env var
	cmd.Env = append(os.Environ(),
		"STORE_DB_NAME="+storeDBName,
		"STORE_DB_URI="+env.Getenv("MONGODB_URI", "mongodb://localhost:27017/"),
		"PYTHONUNBUFFERED=1", // disable Python output buffering for real-time streaming
		"NO_COLOR=1",         // suppress ANSI colour codes; rich degrades to plain text
	)
	w := &logWriter{prefix: "[BI:" + name + "] "}
	cmd.Stdout = w
	cmd.Stderr = w

	err := cmd.Run()

	elapsed := time.Since(scriptStart).Round(time.Millisecond)
	if err != nil {
		log.Printf("[BI] ERROR %s [%s] (%s): %v",
			name, storeDBName, elapsed, err)
		return
	}
	log.Printf("[BI] OK %s [%s] (%s)", name, storeDBName, elapsed)
}

// RunBIBackfillForAllStores runs the one-time historical backfill script for
// every active store. Call this once at startup (before the cron starts) via
// the BI_RUN_BACKFILL=true environment variable.
func RunBIBackfillForAllStores() {
	biDir := env.Getenv("BI_SCRIPTS_DIR", "")
	if biDir == "" {
		log.Print("[BI] BI_SCRIPTS_DIR not set — skipping backfill")
		return
	}

	python := env.Getenv("BI_PYTHON", "python3")
	backfillSc := filepath.Join(biDir, "bi_historical_backfill.py")

	stores, err := GetAllStores()
	if err != nil {
		log.Printf("[BI] Backfill: failed to load stores: %v", err)
		return
	}

	log.Printf("[BI] Starting historical backfill for %d stores", len(stores))
	start := time.Now()

	for _, store := range stores {
		if store.Deleted {
			continue
		}
		storeDBName := "store_" + store.ID.Hex()
		log.Printf("[BI] Backfill store: %s (%s)", store.Name, storeDBName)
		clearBackfillHistoryCollections(storeDBName)
		runBIScript(python, backfillSc, storeDBName)
	}

	log.Printf("[BI] Historical backfill complete in %s", time.Since(start).Round(time.Second))
}

// clearBackfillHistoryCollections drops the four BI history collections that
// bi_historical_backfill.py writes to, so repeated backfill runs don't
// accumulate duplicate entries.
var backfillHistoryCollections = []string{
	"customer_churn_risk_tier_history",
	"customer_predicted_12month_clv_history",
	"product_sales_trend_history",
	"product_abc_xyz_classification_history",
}

func clearBackfillHistoryCollections(storeDBName string) {
	storeDB := db.GetDB(storeDBName)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	for _, col := range backfillHistoryCollections {
		if err := storeDB.Collection(col).Drop(ctx); err != nil {
			log.Printf("[BI] Backfill: failed to drop %s in %s: %v", col, storeDBName, err)
		} else {
			log.Printf("[BI] Backfill: cleared %s in %s", col, storeDBName)
		}
	}
}

// RunBIIncrementalUpdateForAllStores runs Go-native BI aggregations for every
// active store. Called every 3 hours by the gocron scheduler. Updates the last
// 2 months of monthly data and does a full refresh of rolling-window collections.
func RunBIIncrementalUpdateForAllStores() {
	stores, err := GetAllStores()
	if err != nil {
		log.Printf("[BI] Incremental: failed to load stores: %v", err)
		return
	}
	log.Printf("[BI] Starting incremental BI update for %d stores", len(stores))
	start := time.Now()

	for _, store := range stores {
		if store.Deleted {
			continue
		}
		id := store.ID
		RunBIMonthlyRevenueUpdate(id, 24)
		RunBITopProductsUpdate(id)
		RunBITopCustomersUpdate(id)
		RunBIExpenseSummaryUpdate(id, 24)
		RunBIOutstandingUpdate(id)
		RunBIStockAlertsUpdate(id)
		RunBIVendorPerformanceUpdate(id)
		RunBIQuotationConversionUpdate(id, 24)
	}

	log.Printf("[BI] Incremental update complete in %s", time.Since(start).Round(time.Second))
}
