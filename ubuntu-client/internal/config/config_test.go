package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSaveLoadAndPermissions(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	cfg.ServerURL = "https://example.test"
	cfg.APIToken = "123456789012345678901234"
	if err := Save(cfg); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.DeviceID != cfg.DeviceID || loaded.Mode != ModeDownloadOnly {
		t.Fatalf("unexpected config: %+v", loaded)
	}
	path, _, _, _ := Paths()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("config permissions = %o", info.Mode().Perm())
	}
	if filepath.Base(path) != "config.json" {
		t.Fatalf("unexpected path %s", path)
	}
}

func TestRejectInsecureRemoteURL(t *testing.T) {
	cfg, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	cfg.ServerURL = "http://example.com"
	cfg.APIToken = "123456789012345678901234"
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected insecure remote URL to be rejected")
	}
	cfg.ServerURL = "http://127.0.0.1:8080"
	if err := cfg.Validate(); err != nil {
		t.Fatalf("localhost should be allowed: %v", err)
	}
}
