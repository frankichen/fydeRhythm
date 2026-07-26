package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	ListenAddr        string
	DatabasePath      string
	APIToken          string
	CORSOrigins       []string
	MaxBodyBytes      int64
	ReadTimeout       time.Duration
	ReadHeaderTimeout time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
	ShutdownGrace     time.Duration
}

func Load() (Config, error) {
	cfg := Config{
		ListenAddr:        env("LISTEN_ADDR", ":8080"),
		DatabasePath:      env("DATABASE_PATH", "./data/fyderhythm-sync.db"),
		APIToken:          strings.TrimSpace(os.Getenv("API_TOKEN")),
		CORSOrigins:       splitCSV(env("CORS_ORIGINS", "chrome-extension://*,http://localhost:*,http://127.0.0.1:*")),
		MaxBodyBytes:      2 << 20,
		ReadTimeout:       15 * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
		ShutdownGrace:     10 * time.Second,
	}

	var err error
	if raw := strings.TrimSpace(os.Getenv("MAX_BODY_BYTES")); raw != "" {
		cfg.MaxBodyBytes, err = strconv.ParseInt(raw, 10, 64)
		if err != nil || cfg.MaxBodyBytes < 1024 {
			return Config{}, fmt.Errorf("invalid MAX_BODY_BYTES: %q", raw)
		}
	}
	if cfg.APIToken == "" {
		return Config{}, errors.New("API_TOKEN is required")
	}
	if len(cfg.APIToken) < 24 {
		return Config{}, errors.New("API_TOKEN must contain at least 24 characters")
	}
	return cfg, nil
}

func env(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func splitCSV(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}
