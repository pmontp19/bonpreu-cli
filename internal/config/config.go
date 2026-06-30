package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const (
	DirName       = "bonpreu"
	SessionFile   = "cookies.json"
	ConfigFile    = "config.json"
	CacheFile     = "cache.json"
	DefaultMaxEUR = 0
	SourceWeb     = "web"
)

type Session struct {
	Cookies                  map[string]string `json:"cookies"`
	CSRFToken                string            `json:"csrf_token"`
	ClientRouteID            string            `json:"client_route_id"`
	PageViewID               string            `json:"page_view_id"`
	EcomRequestSource        string            `json:"ecom_request_source"`
	EcomRequestSourceVersion string            `json:"ecom_request_source_version"`
	RegionID                 string            `json:"region_id"`
	DeliveryDestinationID    string            `json:"delivery_destination_id"`
}

type Config struct {
	DefaultMaxEUR float64 `json:"default_max_eur,omitempty"`
}

type IDCache struct {
	RetailerToProduct map[string]string `json:"retailer_to_product"`
}

func Dir() (string, error) {
	if v := os.Getenv("BONPREU_HOME"); v != "" {
		return v, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "."+DirName), nil
}

func SessionPath() (string, error) {
	d, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, SessionFile), nil
}

func ConfigPath() (string, error) {
	d, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, ConfigFile), nil
}

func EnsureDir() (string, error) {
	d, err := Dir()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(d, 0o700); err != nil {
		return "", err
	}
	return d, nil
}

func LoadSession() (*Session, error) {
	p, err := SessionPath()
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return nil, fmt.Errorf("read session %s: %w", p, err)
	}
	var s Session
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, fmt.Errorf("parse session: %w", err)
	}
	if s.Cookies == nil {
		s.Cookies = map[string]string{}
	}
	if s.EcomRequestSource == "" {
		s.EcomRequestSource = SourceWeb
	}
	return &s, nil
}

func SaveSession(s *Session) error {
	if _, err := EnsureDir(); err != nil {
		return err
	}
	p, err := SessionPath()
	if err != nil {
		return err
	}
	return writeSecret(p, s)
}

func LoadConfig() (*Config, error) {
	p, err := ConfigPath()
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return &Config{}, nil
		}
		return nil, err
	}
	var c Config
	return &c, json.Unmarshal(b, &c)
}

func CachePath() (string, error) {
	d, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, CacheFile), nil
}

func LoadCache() (*IDCache, error) {
	p, err := CachePath()
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return &IDCache{RetailerToProduct: map[string]string{}}, nil
		}
		return nil, err
	}
	var c IDCache
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, err
	}
	if c.RetailerToProduct == nil {
		c.RetailerToProduct = map[string]string{}
	}
	return &c, nil
}

func SaveCache(c *IDCache) error {
	if _, err := EnsureDir(); err != nil {
		return err
	}
	p, err := CachePath()
	if err != nil {
		return err
	}
	return writeSecret(p, c)
}

func writeSecret(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, b, 0o600); err != nil {
		return err
	}
	return os.Chmod(path, 0o600)
}
