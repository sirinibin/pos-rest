package controller

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

const uploadDir = "./pdfs"

func SavePdf(w http.ResponseWriter, r *http.Request) {
	err := r.ParseMultipartForm(100 << 20) // Max 10MB
	if err != nil {
		http.Error(w, "Error parsing form", http.StatusBadRequest)
		return
	}

	file, handler, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "Error retrieving file", http.StatusBadRequest)
		return
	}
	defer file.Close()

	// Create upload directory if not exists
	if _, err := os.Stat(uploadDir); os.IsNotExist(err) {
		os.MkdirAll(uploadDir, os.ModePerm)
	}

	filename := fmt.Sprintf("%s", handler.Filename)
	savePath := filepath.Join(uploadDir, filename)

	dst, err := os.Create(savePath)
	if err != nil {
		http.Error(w, "Error saving file", http.StatusInternalServerError)
		return
	}
	defer dst.Close()

	_, err = io.Copy(dst, file)
	if err != nil {
		http.Error(w, "Error writing file", http.StatusInternalServerError)
		return
	}

	// Replace this with your actual public server domain or IP
	publicURL := fmt.Sprintf("http://localhost:3004/pdfs/%s", filename)
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"fileUrl": "%s"}`, publicURL)
}

// SharePdf uploads a PDF to filebin.net and returns the public URL.
// This proxies the request server-side to avoid CORS restrictions in WKWebView.
func SharePdf(w http.ResponseWriter, r *http.Request) {
	err := r.ParseMultipartForm(100 << 20)
	if err != nil {
		http.Error(w, "Error parsing form", http.StatusBadRequest)
		return
	}

	file, handler, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "Error retrieving file", http.StatusBadRequest)
		return
	}
	defer file.Close()

	pdfBytes, err := io.ReadAll(file)
	if err != nil {
		http.Error(w, "Error reading file", http.StatusInternalServerError)
		return
	}

	binId := fmt.Sprintf("startpos-%d", time.Now().UnixMilli())
	filename := handler.Filename

	uploadURL := fmt.Sprintf("https://filebin.net/%s/%s", binId, filename)

	req, err := http.NewRequest("POST", uploadURL, bytes.NewReader(pdfBytes))
	if err != nil {
		http.Error(w, "Error creating upload request", http.StatusInternalServerError)
		return
	}
	hash := sha256.Sum256(pdfBytes)
	req.Header.Set("Content-SHA256", hex.EncodeToString(hash[:]))
	req.Header.Set("Content-Type", "application/pdf")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		http.Error(w, fmt.Sprintf("filebin.net upload failed: %v", err), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		http.Error(w, fmt.Sprintf("filebin.net returned %d: %s", resp.StatusCode, string(body)), http.StatusBadGateway)
		return
	}

	publicURL := uploadURL
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"fileUrl": "%s"}`, publicURL)
}
