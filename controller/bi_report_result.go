package controller

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/sirinibin/startpos/backend/env"
	"github.com/sirinibin/startpos/backend/models"
)

// cronAPIKeyValid returns true when the request carries the correct BI cron API key.
// Checked first so long-running cron writes don't hit the JWT Redis lookup.
func cronAPIKeyValid(r *http.Request) bool {
	key := env.Getenv("BI_CRON_API_KEY", "")
	if key == "" {
		return false
	}
	if v := r.Header.Get("X-BI-Cron-Key"); v == key {
		return true
	}
	if v := r.URL.Query().Get("cron_api_key"); v == key {
		return true
	}
	return false
}

// biCronOrJWT authenticates by cron API key first, then falls back to JWT.
// Returns the resolved store and whether auth succeeded.
func biCronOrJWT(w http.ResponseWriter, r *http.Request) (*models.Store, bool) {
	w.Header().Set("Content-Type", "application/json")
	var resp models.Response
	resp.Errors = make(map[string]string)

	if cronAPIKeyValid(r) {
		// Cron key path — look up store directly from query param
		store, err := ParseStore(r)
		if err != nil {
			resp.Status = false
			resp.Errors["store_id"] = "Invalid store_id: " + err.Error()
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(resp)
			return nil, false
		}
		return store, true
	}

	// JWT path
	if _, err := models.AuthenticateByAccessToken(r); err != nil {
		resp.Status = false
		resp.Errors["access_token"] = "Invalid access token: " + err.Error()
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(resp)
		return nil, false
	}
	store, err := ParseStore(r)
	if err != nil {
		resp.Status = false
		resp.Errors["store_id"] = "Invalid store_id: " + err.Error()
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(resp)
		return nil, false
	}
	return store, true
}

// ── POST /v1/bi/report-result  (cron key) ─────────────────────────────────────

type saveReportResultReq struct {
	StoreID    string `json:"store_id"`
	ReportKey  string `json:"report_key"`
	CSVContent string `json:"csv_content"`
	PDFContent string `json:"pdf_content"` // base64-encoded
	RowCount   int    `json:"row_count"`
}

func SaveBIReportResult(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var resp models.Response
	resp.Errors = make(map[string]string)

	if _, err := models.AuthenticateByAccessToken(r); err != nil {
		resp.Status = false
		resp.Errors["access_token"] = "Invalid access token: " + err.Error()
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(resp)
		return
	}

	var req saveReportResultReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		resp.Status = false
		resp.Errors["body"] = "Invalid JSON: " + err.Error()
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(resp)
		return
	}

	store, err := models.FindStoreByIDString(req.StoreID)
	if err != nil {
		resp.Status = false
		resp.Errors["store_id"] = "Store not found: " + err.Error()
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(resp)
		return
	}

	var pdfBytes []byte
	if req.PDFContent != "" {
		pdfBytes, err = base64.StdEncoding.DecodeString(req.PDFContent)
		if err != nil {
			resp.Status = false
			resp.Errors["pdf_content"] = "Invalid base64: " + err.Error()
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(resp)
			return
		}
	}

	if err := store.UpsertBIReportResult(req.ReportKey, req.CSVContent, pdfBytes, req.RowCount); err != nil {
		resp.Status = false
		resp.Errors["save"] = "Failed to save: " + err.Error()
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(resp)
		return
	}

	resp.Status = true
	json.NewEncoder(w).Encode(resp)
}

// ── DELETE /v1/bi/report-result  (JWT) ───────────────────────────────────────

func DeleteBIReportResult(w http.ResponseWriter, r *http.Request) {
	store, ok := biCronOrJWT(w, r)
	if !ok {
		return
	}
	reportKey := r.URL.Query().Get("report_key")
	if reportKey == "" {
		var resp models.Response
		resp.Status = false
		resp.Errors = map[string]string{"report_key": "required"}
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(resp)
		return
	}
	if err := store.DeleteBIReportResult(reportKey); err != nil {
		var resp models.Response
		resp.Status = false
		resp.Errors = map[string]string{"delete": err.Error()}
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(resp)
		return
	}
	var resp models.Response
	resp.Status = true
	json.NewEncoder(w).Encode(resp)
}

// ── GET /v1/bi/report-result  (JWT or cron key) ───────────────────────────────

func GetBIReportResult(w http.ResponseWriter, r *http.Request) {
	store, ok := biCronOrJWT(w, r)
	if !ok {
		return
	}

	reportKey := r.URL.Query().Get("report_key")
	if reportKey == "" {
		var resp models.Response
		resp.Errors = map[string]string{"report_key": "required"}
		resp.Status = false
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(resp)
		return
	}

	result, err := store.GetBIReportResult(reportKey)
	if err != nil {
		var resp models.Response
		resp.Status = false
		resp.Errors = map[string]string{"find": "No data found: " + err.Error()}
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(resp)
		return
	}

	var resp models.Response
	resp.Status = true
	resp.Result = result
	json.NewEncoder(w).Encode(resp)
}

// ── GET /v1/bi/report-result/download  (JWT) ─────────────────────────────────

func DownloadBIReportResult(w http.ResponseWriter, r *http.Request) {
	store, ok := biAuthAndStore(w, r)
	if !ok {
		return
	}

	reportKey := r.URL.Query().Get("report_key")
	format := strings.ToLower(r.URL.Query().Get("format"))
	if reportKey == "" {
		http.Error(w, "report_key required", http.StatusBadRequest)
		return
	}

	result, err := store.GetBIReportResultWithPDF(reportKey)
	if err != nil {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	if format == "pdf" {
		if len(result.PDFContent) == 0 {
			http.Error(w, "No PDF available", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/pdf")
		w.Header().Set("Content-Disposition", `attachment; filename="`+reportKey+`_report.pdf"`)
		w.Write(result.PDFContent)
		return
	}

	// Default: CSV
	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", `attachment; filename="`+reportKey+`.csv"`)
	w.Write([]byte(result.CSVContent))
}

// ── POST /v1/bi/cron-log  (cron key) ─────────────────────────────────────────

func SaveBICronLog(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var resp models.Response
	resp.Errors = make(map[string]string)

	if _, err := models.AuthenticateByAccessToken(r); err != nil {
		resp.Status = false
		resp.Errors["access_token"] = "Invalid access token: " + err.Error()
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(resp)
		return
	}

	var cronLog models.BICronLog
	if err := json.NewDecoder(r.Body).Decode(&cronLog); err != nil {
		resp.Status = false
		resp.Errors["body"] = "Invalid JSON: " + err.Error()
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(resp)
		return
	}

	store, err := models.FindStoreByIDString(cronLog.StoreID.Hex())
	if err != nil {
		resp.Status = false
		resp.Errors["store_id"] = "Store not found: " + err.Error()
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(resp)
		return
	}

	if err := store.UpsertBICronLog(&cronLog); err != nil {
		resp.Status = false
		resp.Errors["save"] = err.Error()
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(resp)
		return
	}

	resp.Status = true
	json.NewEncoder(w).Encode(resp)
}

// ── DELETE /v1/bi/cron-log  (JWT) ────────────────────────────────────────────

func DeleteBICronLog(w http.ResponseWriter, r *http.Request) {
	store, ok := biAuthAndStore(w, r)
	if !ok {
		return
	}
	if err := store.DeleteBICronLog(); err != nil {
		var resp models.Response
		resp.Status = false
		resp.Errors = map[string]string{"delete": err.Error()}
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(resp)
		return
	}
	var resp models.Response
	resp.Status = true
	json.NewEncoder(w).Encode(resp)
}

// ── GET /v1/bi/cron-log  (JWT) ────────────────────────────────────────────────

func GetBICronLog(w http.ResponseWriter, r *http.Request) {
	store, ok := biAuthAndStore(w, r)
	if !ok {
		return
	}

	log, err := store.GetBICronLog()
	if err != nil {
		var resp models.Response
		resp.Status = false
		resp.Errors = map[string]string{"find": "No cron log found"}
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(resp)
		return
	}

	var resp models.Response
	resp.Status = true
	resp.Result = log
	json.NewEncoder(w).Encode(resp)
}

// ── POST /v1/bi/report-scores/abc-xyz  (cron key) ────────────────────────────

func SaveAbcXyzScores(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var resp models.Response
	resp.Errors = make(map[string]string)

	if !cronAPIKeyValid(r) {
		resp.Status = false
		resp.Errors["auth"] = "Invalid or missing cron API key"
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(resp)
		return
	}

	var req models.AbcXyzScoreRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		resp.Status = false; resp.Errors["body"] = err.Error()
		w.WriteHeader(http.StatusBadRequest); json.NewEncoder(w).Encode(resp); return
	}
	store, err := models.FindStoreByIDString(req.StoreID)
	if err != nil {
		resp.Status = false; resp.Errors["store_id"] = err.Error()
		w.WriteHeader(http.StatusBadRequest); json.NewEncoder(w).Encode(resp); return
	}
	if err := store.BulkUpsertAbcXyzScores(req); err != nil {
		resp.Status = false; resp.Errors["upsert"] = err.Error()
		w.WriteHeader(http.StatusInternalServerError); json.NewEncoder(w).Encode(resp); return
	}
	resp.Status = true; json.NewEncoder(w).Encode(resp)
}

// ── POST /v1/bi/report-scores/velocity  (cron key) ───────────────────────────

func SaveVelocityScores(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var resp models.Response
	resp.Errors = make(map[string]string)

	if _, err := models.AuthenticateByAccessToken(r); err != nil {
		resp.Status = false; resp.Errors["access_token"] = "Invalid access token: " + err.Error()
		w.WriteHeader(http.StatusUnauthorized); json.NewEncoder(w).Encode(resp); return
	}

	var req models.VelocityScoreRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		resp.Status = false; resp.Errors["body"] = err.Error()
		w.WriteHeader(http.StatusBadRequest); json.NewEncoder(w).Encode(resp); return
	}
	store, err := models.FindStoreByIDString(req.StoreID)
	if err != nil {
		resp.Status = false; resp.Errors["store_id"] = err.Error()
		w.WriteHeader(http.StatusBadRequest); json.NewEncoder(w).Encode(resp); return
	}
	if err := store.BulkUpsertVelocityScores(req); err != nil {
		resp.Status = false; resp.Errors["upsert"] = err.Error()
		w.WriteHeader(http.StatusInternalServerError); json.NewEncoder(w).Encode(resp); return
	}
	resp.Status = true; json.NewEncoder(w).Encode(resp)
}

// ── POST /v1/bi/report-scores/clv  (cron key) ────────────────────────────────

func SaveCLVScores(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var resp models.Response
	resp.Errors = make(map[string]string)

	if _, err := models.AuthenticateByAccessToken(r); err != nil {
		resp.Status = false; resp.Errors["access_token"] = "Invalid access token: " + err.Error()
		w.WriteHeader(http.StatusUnauthorized); json.NewEncoder(w).Encode(resp); return
	}

	var req models.CLVScoreRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		resp.Status = false; resp.Errors["body"] = err.Error()
		w.WriteHeader(http.StatusBadRequest); json.NewEncoder(w).Encode(resp); return
	}
	store, err := models.FindStoreByIDString(req.StoreID)
	if err != nil {
		resp.Status = false; resp.Errors["store_id"] = err.Error()
		w.WriteHeader(http.StatusBadRequest); json.NewEncoder(w).Encode(resp); return
	}
	if err := store.BulkUpsertCLVScores(req); err != nil {
		resp.Status = false; resp.Errors["upsert"] = err.Error()
		w.WriteHeader(http.StatusInternalServerError); json.NewEncoder(w).Encode(resp); return
	}
	resp.Status = true; json.NewEncoder(w).Encode(resp)
}

// ── POST /v1/bi/report-scores/cohort  (cron key) ─────────────────────────────

func SaveCohortScores(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var resp models.Response
	resp.Errors = make(map[string]string)

	if _, err := models.AuthenticateByAccessToken(r); err != nil {
		resp.Status = false; resp.Errors["access_token"] = "Invalid access token: " + err.Error()
		w.WriteHeader(http.StatusUnauthorized); json.NewEncoder(w).Encode(resp); return
	}

	var req models.CohortScoreRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		resp.Status = false; resp.Errors["body"] = err.Error()
		w.WriteHeader(http.StatusBadRequest); json.NewEncoder(w).Encode(resp); return
	}
	store, err := models.FindStoreByIDString(req.StoreID)
	if err != nil {
		resp.Status = false; resp.Errors["store_id"] = err.Error()
		w.WriteHeader(http.StatusBadRequest); json.NewEncoder(w).Encode(resp); return
	}
	if err := store.BulkUpsertCohortScores(req); err != nil {
		resp.Status = false; resp.Errors["upsert"] = err.Error()
		w.WriteHeader(http.StatusInternalServerError); json.NewEncoder(w).Encode(resp); return
	}
	resp.Status = true; json.NewEncoder(w).Encode(resp)
}

// ── POST /v1/bi/batch-cost  (cron key) ───────────────────────────────────────

type saveBatchCostReq struct {
	ReportKey   string  `json:"report_key"`
	CostUSD     float64 `json:"cost_usd"`
	DurationSec int64   `json:"duration_sec"`
}

func SaveBIBatchCost(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var resp models.Response
	resp.Errors = make(map[string]string)

	if _, err := models.AuthenticateByAccessToken(r); err != nil {
		resp.Status = false
		resp.Errors["access_token"] = "Invalid access token: " + err.Error()
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(resp)
		return
	}

	var req saveBatchCostReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		resp.Status = false
		resp.Errors["body"] = "Invalid JSON: " + err.Error()
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(resp)
		return
	}
	if req.ReportKey == "" {
		resp.Status = false
		resp.Errors["report_key"] = "required"
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(resp)
		return
	}

	if err := models.AddBIBatchCost(req.ReportKey, req.CostUSD, req.DurationSec); err != nil {
		resp.Status = false
		resp.Errors["save"] = err.Error()
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(resp)
		return
	}

	resp.Status = true
	json.NewEncoder(w).Encode(resp)
}

// ── GET /v1/bi/batch-costs  (JWT) ────────────────────────────────────────────

func GetBIBatchCosts(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var resp models.Response
	resp.Errors = make(map[string]string)

	if _, err := models.AuthenticateByAccessToken(r); err != nil {
		resp.Status = false
		resp.Errors["access_token"] = "Invalid access token: " + err.Error()
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(resp)
		return
	}

	costs, err := models.GetAllBIBatchCosts()
	if err != nil {
		resp.Status = false
		resp.Errors["find"] = err.Error()
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(resp)
		return
	}

	var grandTotal float64
	for _, c := range costs {
		grandTotal += c.TotalCostUSD
	}

	resp.Status = true
	resp.Result = map[string]interface{}{
		"grand_total_usd": grandTotal,
		"reports":         costs,
	}
	json.NewEncoder(w).Encode(resp)
}

// ── POST /v1/bi/report-scores/churn  (cron key) ──────────────────────────────

func SaveChurnScores(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var resp models.Response
	resp.Errors = make(map[string]string)

	if _, err := models.AuthenticateByAccessToken(r); err != nil {
		resp.Status = false; resp.Errors["access_token"] = "Invalid access token: " + err.Error()
		w.WriteHeader(http.StatusUnauthorized); json.NewEncoder(w).Encode(resp); return
	}

	var req models.ChurnScoreRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		resp.Status = false; resp.Errors["body"] = err.Error()
		w.WriteHeader(http.StatusBadRequest); json.NewEncoder(w).Encode(resp); return
	}
	store, err := models.FindStoreByIDString(req.StoreID)
	if err != nil {
		resp.Status = false; resp.Errors["store_id"] = err.Error()
		w.WriteHeader(http.StatusBadRequest); json.NewEncoder(w).Encode(resp); return
	}
	if err := store.BulkUpsertChurnScores(req); err != nil {
		resp.Status = false; resp.Errors["upsert"] = err.Error()
		w.WriteHeader(http.StatusInternalServerError); json.NewEncoder(w).Encode(resp); return
	}
	resp.Status = true; json.NewEncoder(w).Encode(resp)
}
