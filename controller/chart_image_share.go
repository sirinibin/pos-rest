package controller

import (
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/sirinibin/startpos/backend/models"
)

// ShareChartImage uploads a PNG from the request body to filebin.net,
// shortens the URL, and returns it so the frontend can open WhatsApp.
// POST /v1/chart-image-share?filename=my_chart.png
func ShareChartImage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var response models.Response
	response.Errors = make(map[string]string)

	_, err := models.AuthenticateByAccessToken(r)
	if err != nil {
		response.Status = false
		response.Errors["access_token"] = "Invalid access token: " + err.Error()
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(response)
		return
	}

	imageData, err := io.ReadAll(io.LimitReader(r.Body, 10<<20)) // 10 MB limit
	if err != nil || len(imageData) == 0 {
		response.Status = false
		response.Errors["image"] = "Failed to read image data"
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	filename := r.URL.Query().Get("filename")
	if filename == "" {
		filename = fmt.Sprintf("chart_%d.png", time.Now().Unix())
	}

	fileURL, err := uploadToFilebin(filename, imageData)
	if err != nil {
		response.Status = false
		response.Errors["upload"] = "Failed to upload image: " + err.Error()
		w.WriteHeader(http.StatusBadGateway)
		json.NewEncoder(w).Encode(response)
		return
	}

	shortURL := shortenURLViaAPI(fileURL)

	response.Status = true
	response.Result = map[string]string{"url": shortURL}
	json.NewEncoder(w).Encode(response)
}

func uploadToFilebin(filename string, data []byte) (string, error) {
	const chars = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, 8)
	for i := range b {
		b[i] = chars[rand.Intn(len(chars))]
	}
	binID := fmt.Sprintf("sp-%d-%s", time.Now().UnixMilli(), string(b))

	uploadURL := fmt.Sprintf("https://filebin.net/%s/%s", binID, filename)

	req, err := http.NewRequest("POST", uploadURL, strings.NewReader(string(data)))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "image/png")
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("filebin returned %d", resp.StatusCode)
	}

	return uploadURL, nil
}

func shortenURLViaAPI(longURL string) string {
	client := &http.Client{Timeout: 5 * time.Second}

	// Try is.gd first
	resp, err := client.Get("https://is.gd/create.php?format=simple&url=" + url.QueryEscape(longURL))
	if err == nil && resp.StatusCode == http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		short := strings.TrimSpace(string(body))
		if strings.HasPrefix(short, "https://is.gd/") {
			return short
		}
	}

	// Fallback: TinyURL
	resp, err = client.Get("https://tinyurl.com/api-create.php?url=" + url.QueryEscape(longURL))
	if err == nil && resp.StatusCode == http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		short := strings.TrimSpace(string(body))
		if strings.HasPrefix(short, "https://tinyurl.com/") {
			return short
		}
	}

	return longURL // both shorteners failed — return original
}
