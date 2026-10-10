package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// DefaultServer is StartERP's API (overridden at build time with
// -ldflags "-X main.DefaultServer=...", or on the pairing page).
var DefaultServer = "https://startpos-api-v2.gulfunionozone.com/v1/erp"

// Version is set at build time.
var Version = "dev"

// Config is kept next to the user's settings (0600). The token only lets
// this computer fetch and answer card-machine jobs of one store.
type Config struct {
	Server    string `json:"server"`
	Token     string `json:"token,omitempty"`
	BridgeID  string `json:"bridgeId,omitempty"`
	StoreID   string `json:"storeId,omitempty"`
	StoreName string `json:"storeName,omitempty"`
	Name      string `json:"name,omitempty"`
	Port      int    `json:"port,omitempty"`
}

func (c Config) paired() bool { return c.Token != "" && c.Server != "" }

type configStore struct {
	mu   sync.Mutex
	path string
	cfg  Config
}

func defaultConfigPath() string {
	if v := strings.TrimSpace(os.Getenv("STARTERP_CARDBRIDGE_CONFIG")); v != "" {
		return v
	}
	dir, err := os.UserConfigDir()
	if err != nil || dir == "" {
		dir = "."
	}
	return filepath.Join(dir, "StartERP Card Bridge", "config.json")
}

func openConfig(path string) (*configStore, error) {
	s := &configStore{path: path}
	b, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if err == nil {
		if err := json.Unmarshal(b, &s.cfg); err != nil {
			return nil, errors.New("the settings file " + path + " is damaged: " + err.Error())
		}
	}
	if s.cfg.Server == "" {
		s.cfg.Server = DefaultServer
	}
	if s.cfg.Port == 0 {
		s.cfg.Port = 17777
	}
	if s.cfg.Name == "" {
		h, _ := os.Hostname()
		s.cfg.Name = h
	}
	return s, nil
}

func (s *configStore) get() Config {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cfg
}

func (s *configStore) update(fn func(c *Config)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := s.cfg
	fn(&next)
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(next, "", "  ")
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, s.path); err != nil {
		return err
	}
	s.cfg = next
	return nil
}
