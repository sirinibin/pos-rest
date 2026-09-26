package controller

import (
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// ProxyImageHandler fetches an external image URL and streams it back to the client.
// GET /v1/proxy-image?url=<encoded-url>
// This allows email body images (which may be HTTP or require specific headers)
// to load without mixed-content blocking on HTTPS pages.
func ProxyImageHandler(w http.ResponseWriter, r *http.Request) {
	rawURL := r.URL.Query().Get("url")
	if rawURL == "" {
		http.Error(w, "url parameter required", http.StatusBadRequest)
		return
	}

	parsed, err := url.Parse(rawURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		http.Error(w, "invalid url", http.StatusBadRequest)
		return
	}

	// Block requests to private/loopback addresses for SSRF protection
	host := strings.ToLower(parsed.Hostname())
	if host == "localhost" || host == "127.0.0.1" || host == "::1" ||
		strings.HasPrefix(host, "192.168.") || strings.HasPrefix(host, "10.") ||
		strings.HasPrefix(host, "172.") {
		http.Error(w, "private addresses not allowed", http.StatusForbidden)
		return
	}

	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest("GET", rawURL, nil)
	if err != nil {
		http.Error(w, "request error", http.StatusBadGateway)
		return
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; StartPOS/1.0)")

	resp, err := client.Do(req)
	if err != nil {
		http.Error(w, "fetch error", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	ct := resp.Header.Get("Content-Type")
	if ct == "" {
		ct = "image/jpeg"
	}
	// Only allow image content types
	if !strings.HasPrefix(ct, "image/") {
		http.Error(w, "not an image", http.StatusForbidden)
		return
	}

	w.Header().Set("Content-Type", ct)
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}
