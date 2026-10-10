package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"runtime"
	"strings"
	"time"
)

// client talks to StartERP. Every call is outgoing HTTPS: no port is opened
// on the shop's network.
type client struct {
	server string
	token  string
	http   *http.Client
}

// apiError is an answer StartERP refused.
type apiError struct {
	Status  int
	Code    string
	Message string
}

func (e *apiError) Error() string { return e.Message }

// unpaired: the token no longer works (unpaired in Settings).
func unpaired(err error) bool {
	var ae *apiError
	return errors.As(err, &ae) && ae.Status == http.StatusUnauthorized
}

func newClient(server, token string) *client {
	return &client{server: strings.TrimRight(strings.TrimSpace(server), "/"), token: token,
		http: &http.Client{Timeout: 40 * time.Second}}
}

func (c *client) do(ctx context.Context, method, path string, body interface{}, out interface{}) (int, error) {
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.server+path, rd)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "StartERP-CardBridge/"+Version+" ("+runtime.GOOS+"; "+runtime.GOARCH+")")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		req.Header.Set("X-Card-Bridge-Token", c.token)
	}
	res, err := c.http.Do(req)
	if err != nil {
		return 0, err
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	if res.StatusCode >= 400 {
		var e struct {
			Error   interface{} `json:"error"`
			Code    string      `json:"code"`
			Message string      `json:"message"`
		}
		_ = json.Unmarshal(b, &e)
		msg := e.Message
		if m, ok := e.Error.(map[string]interface{}); ok {
			if s, ok := m["message"].(string); ok && msg == "" {
				msg = s
			}
			if s, ok := m["code"].(string); ok && e.Code == "" {
				e.Code = s
			}
		} else if s, ok := e.Error.(string); ok && msg == "" {
			msg = s
		}
		if msg == "" {
			msg = fmt.Sprintf("StartERP answered %d", res.StatusCode)
		}
		return res.StatusCode, &apiError{Status: res.StatusCode, Code: e.Code, Message: msg}
	}
	if out != nil && len(b) > 0 && res.StatusCode != http.StatusNoContent {
		if err := json.Unmarshal(b, out); err != nil {
			return res.StatusCode, fmt.Errorf("unexpected answer from StartERP: %v", err)
		}
	}
	return res.StatusCode, nil
}

type pairAnswer struct {
	Token     string `json:"token"`
	BridgeID  string `json:"bridgeId"`
	StoreID   string `json:"storeId"`
	StoreName string `json:"storeName"`
}

func hello(name string) map[string]interface{} {
	return map[string]interface{}{"name": name, "os": runtime.GOOS, "arch": runtime.GOARCH, "version": Version, "drivers": driverIDs()}
}

func (c *client) pair(ctx context.Context, code, name string) (pairAnswer, error) {
	body := hello(name)
	body["code"] = strings.ToUpper(strings.NewReplacer(" ", "", "-", "").Replace(strings.TrimSpace(code)))
	var a pairAnswer
	_, err := c.do(ctx, http.MethodPost, "/card-bridge/pair", body, &a)
	return a, err
}

func (c *client) hello(ctx context.Context, name string) (pairAnswer, error) {
	var a pairAnswer
	_, err := c.do(ctx, http.MethodPost, "/card-bridge/hello", hello(name), &a)
	return a, err
}

// next waits up to wait seconds for the next job (nil when none came).
func (c *client) next(ctx context.Context, wait int) (*Job, error) {
	var a struct {
		Job *Job `json:"job"`
	}
	st, err := c.do(ctx, http.MethodGet, fmt.Sprintf("/card-bridge/jobs?wait=%d", wait), nil, &a)
	if err != nil || st == http.StatusNoContent {
		return nil, err
	}
	return a.Job, nil
}

func (c *client) result(ctx context.Context, jobID string, r Result) error {
	var err error
	// the answer is money: keep trying for a while if the internet blinks
	for i := 0; i < 8; i++ {
		_, err = c.do(ctx, http.MethodPost, "/card-bridge/jobs/"+jobID+"/result", r, nil)
		var ae *apiError
		if err == nil || (errors.As(err, &ae) && ae.Status < 500) {
			return err
		}
		select {
		case <-ctx.Done():
			return err
		case <-time.After(time.Duration(1<<i) * 500 * time.Millisecond):
		}
	}
	return err
}
