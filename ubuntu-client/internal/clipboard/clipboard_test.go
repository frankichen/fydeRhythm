package clipboard

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadTextUsesWaylandClipboard(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "wl-paste")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nprintf '微信兼容测试'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	t.Setenv("WAYLAND_DISPLAY", "wayland-0")
	t.Setenv("DISPLAY", "")
	text, err := ReadText(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if text != "微信兼容测试" {
		t.Fatalf("unexpected clipboard text %q", text)
	}
}

func TestReadTextRejectsEmptyClipboard(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "xclip")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	t.Setenv("WAYLAND_DISPLAY", "")
	t.Setenv("DISPLAY", ":0")
	_, err := ReadText(context.Background())
	if err == nil || !strings.Contains(err.Error(), "剪贴板") {
		t.Fatalf("expected clipboard error, got %v", err)
	}
}
