package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
	"github.com/gorilla/mux"
	"github.com/sirinibin/startpos/backend/env"
	"github.com/sirinibin/startpos/backend/models"
)

// ReportPrintData returns the stored report print job for chromedp's React page.
// No authentication required — the key itself is an unguessable random secret.
// GET /v1/report/print-data/{key}
func ReportPrintData(w http.ResponseWriter, r *http.Request) {
	key := mux.Vars(r)["key"]
	val, ok := printJobStore.Load(key)
	if !ok {
		http.Error(w, `{"error":"not found or expired"}`, http.StatusNotFound)
		return
	}
	job := val.(printJobData)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"model":     job.Model,
		"modelName": job.ModelName,
		"fontSizes": job.FontSizes,
	})
}

// ReportPDF accepts fully-loaded report data from React, stores it under a random
// key, then uses headless Chrome to render the React /report-print page (which
// reads the data via that key) and captures an A4 PDF.
//
// POST /v1/report/pdf
// Body: { "model": {...}, "modelName": "sales_report", "fontSizes": {...}, "filename": "..." }
func ReportPDF(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var response models.Response
	response.Errors = make(map[string]string)

	_, err := models.AuthenticateByAccessToken(r)
	if err != nil {
		response.Status = false
		response.Errors["access_token"] = "Invalid Access token:" + err.Error()
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(response)
		return
	}

	var reqBody struct {
		Model     json.RawMessage `json:"model"`
		ModelName string          `json:"modelName"`
		FontSizes json.RawMessage `json:"fontSizes"`
		Filename  string          `json:"filename"`
	}
	if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
		response.Status = false
		response.Errors["body"] = "Invalid request body: " + err.Error()
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}
	if len(reqBody.Model) == 0 || reqBody.ModelName == "" {
		response.Status = false
		response.Errors["params"] = "model and modelName are required"
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	chromeBin := chromePath()
	if chromeBin == "" {
		response.Status = false
		response.Errors["chrome"] = "Chrome/Chromium not found. Install Google Chrome to use PDF generation."
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	key, err := generatePrintKey()
	if err != nil {
		response.Status = false
		response.Errors["key"] = "Failed to generate print key: " + err.Error()
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	printJobStore.Store(key, printJobData{
		Model:     reqBody.Model,
		ModelName: reqBody.ModelName,
		FontSizes: reqBody.FontSizes,
		CreatedAt: time.Now(),
	})
	defer printJobStore.Delete(key)

	apiPort := env.Getenv("API_PORT", "2000")
	printURL := fmt.Sprintf("http://localhost:%s/report-print?key=%s", apiPort, key)

	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.ExecPath(chromeBin),
		chromedp.Flag("headless", true),
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("no-sandbox", true),
		chromedp.WindowSize(794, 1123), // A4 at 96dpi
	)

	allocCtx, cancelAlloc := chromedp.NewExecAllocator(context.Background(), opts...)
	defer cancelAlloc()

	ctx, cancelCtx := chromedp.NewContext(allocCtx)
	defer cancelCtx()

	ctx, cancelTimeout := context.WithTimeout(ctx, 60*time.Second)
	defer cancelTimeout()

	var pdfBuf []byte
	err = chromedp.Run(ctx,
		chromedp.Navigate(printURL),
		// React page sets this attribute when all data is rendered and ready
		chromedp.WaitVisible(`body[data-print-ready="true"]`, chromedp.ByQuery),
		// Extra settle time for fonts and images
		chromedp.Sleep(500*time.Millisecond),
		chromedp.ActionFunc(func(ctx context.Context) error {
			buf, _, err := page.PrintToPDF().
				WithPrintBackground(true).
				WithPaperWidth(8.27).   // A4 width in inches  (210 mm)
				WithPaperHeight(11.69). // A4 height in inches (297 mm)
				WithMarginTop(0).
				WithMarginBottom(0).
				WithMarginLeft(0).
				WithMarginRight(0).
				WithPreferCSSPageSize(true).
				Do(ctx)
			if err != nil {
				return err
			}
			pdfBuf = buf
			return nil
		}),
	)
	if err != nil {
		response.Status = false
		response.Errors["pdf"] = "PDF generation failed: " + err.Error()
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	// Save to ~/Downloads/{filename}.pdf
	savedPath := ""
	saveFilename := reqBody.Filename
	if saveFilename == "" {
		saveFilename = fmt.Sprintf("report_%s_%d", reqBody.ModelName, time.Now().Unix())
	}
	if homeDir, err := os.UserHomeDir(); err == nil {
		savePath := fmt.Sprintf("%s/Downloads/%s.pdf", homeDir, saveFilename)
		if writeErr := os.WriteFile(savePath, pdfBuf, 0644); writeErr == nil {
			savedPath = savePath
		}
	}

	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.pdf"`, saveFilename))
	if savedPath != "" {
		w.Header().Set("X-Saved-To", savedPath)
	}
	w.WriteHeader(http.StatusOK)
	w.Write(pdfBuf)
}
