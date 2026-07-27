package app

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/frankichen/fydeRhythm/ubuntu-client/internal/config"
	"github.com/frankichen/fydeRhythm/ubuntu-client/internal/state"
)

func TestInitialSnapshotSyncWritesRimeDictionary(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"status":"ok"}`)
	})
	mux.HandleFunc("/api/v1/sync/snapshot", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "" {
			http.Error(w, "missing auth", http.StatusUnauthorized)
			return
		}
		fmt.Fprint(w, `{"cursor":7,"generated_at":1,"lexicon":[{"id":"one","phrase":"云端测试词","code":"yunc","shortcut":"yc","weight":200}]}`)
	})
	mux.HandleFunc("/api/v1/sync/pull", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"cursor":7,"has_more":false,"changes":[]}`)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	rimeDir := t.TempDir()
	a := New()
	if err := a.Init(server.URL, "123456789012345678901234", config.ModeDownloadOnly, "test-ubuntu", "wubi86_jidian_pinyin_smart", rimeDir, true); err != nil {
		t.Fatal(err)
	}
	if err := a.Sync(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(rimeDir, "fyderhythm_personal.dict.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "云端测试词\tyc\t200") {
		t.Fatalf("unexpected dictionary:\n%s", data)
	}
	st, err := state.Load()
	if err != nil {
		t.Fatal(err)
	}
	if !st.Initialized || st.Cursor != 7 || len(st.Lexicon) != 1 {
		t.Fatalf("unexpected state: %+v", st)
	}
}
