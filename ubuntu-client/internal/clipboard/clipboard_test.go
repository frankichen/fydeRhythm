package clipboard

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

func TestSnapshotAllowsEmptyClipboard(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "xclip")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	t.Setenv("WAYLAND_DISPLAY", "")
	t.Setenv("DISPLAY", ":0")
	text, err := Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if text != "" {
		t.Fatalf("expected empty snapshot, got %q", text)
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

func TestWaitForChangeRejectsStaleClipboardAndReturnsNewSelection(t *testing.T) {
	dir := t.TempDir()
	state := filepath.Join(dir, "state")
	script := filepath.Join(dir, "xclip")
	body := "#!/bin/sh\ncount=0\n[ -f '" + state + "' ] && read count < '" + state + "'\ncount=$((count + 1))\nprintf '%s' \"$count\" > '" + state + "'\nif [ \"$count\" -lt 3 ]; then printf '旧剪贴板'; else printf '又到了商业胡吹时间了'; fi\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	t.Setenv("WAYLAND_DISPLAY", "")
	t.Setenv("DISPLAY", ":0")
	text, err := WaitForChange(context.Background(), "旧剪贴板", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if text != "又到了商业胡吹时间了" {
		t.Fatalf("unexpected changed clipboard text %q", text)
	}
}
