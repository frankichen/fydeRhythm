package rime

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/frankichen/fydeRhythm/ubuntu-client/internal/config"
	"github.com/frankichen/fydeRhythm/ubuntu-client/internal/model"
)

func TestRenderDictionaryAndPatch(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Config{RimeUserDir: dir, SchemaID: "wubi86_jidian_pinyin_smart"}
	entries := map[string]model.LexiconEntry{
		"1": {ID: "1", Phrase: "测试词", Code: "abcd", Shortcut: "cs", Weight: 321},
		"2": {ID: "2", Phrase: "已删除", Code: "zzzz", Deleted: true},
	}
	count, err := Render(cfg, entries)
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("count = %d", count)
	}
	dict, err := os.ReadFile(filepath.Join(dir, "fyderhythm_personal.dict.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(dict)
	for _, want := range []string{"测试词\tcs\t321", "测试词\tabcd\t321"} {
		if !strings.Contains(text, want) {
			t.Fatalf("dictionary missing %q:\n%s", want, text)
		}
	}
	patchPath := filepath.Join(dir, cfg.SchemaID+".custom.yaml")
	patch1, err := os.ReadFile(patchPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(patch1), "table_translator@fyderhythm_personal") {
		t.Fatalf("patch missing translator:\n%s", patch1)
	}
	for _, want := range []string{
		"accept: minus, send: Page_Up",
		"accept: equal, send: Page_Down",
		"ascii_composer/switch_key/Shift_L\": noop",
		"ascii_composer/switch_key/Shift_R\": commit_text",
		"switches/@2/reset\": 1",
	} {
		if !strings.Contains(string(patch1), want) {
			t.Fatalf("patch missing preference %q:\n%s", want, patch1)
		}
	}
	if _, err := Render(cfg, entries); err != nil {
		t.Fatal(err)
	}
	patch2, _ := os.ReadFile(patchPath)
	if string(patch1) != string(patch2) {
		t.Fatal("second render should be idempotent")
	}
}

func TestAppendToExistingPatch(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Config{RimeUserDir: dir, SchemaID: "demo"}
	path := filepath.Join(dir, "demo.custom.yaml")
	if err := os.WriteFile(path, []byte("patch:\n  menu/page_size: 9\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Render(cfg, map[string]model.LexiconEntry{}); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "menu/page_size") || !strings.Contains(string(data), markerBegin) {
		t.Fatalf("existing patch not preserved:\n%s", data)
	}
}
