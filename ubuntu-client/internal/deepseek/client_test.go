package deepseek

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/frankichen/fydeRhythm/ubuntu-client/internal/config"
)

func TestCleanResult(t *testing.T) {
	cases := map[string]string{
		"  连接成功  ":             "连接成功",
		"\"修正后的文字\"":           "修正后的文字",
		"“修正后的文字”":             "修正后的文字",
		"```text\n修正后的文字\n```": "修正后的文字",
	}
	for input, want := range cases {
		if got := cleanResult(input); got != want {
			t.Fatalf("cleanResult(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestCorrectUsesConfiguredModelAndAuthorization(t *testing.T) {
	t.Setenv("HTTP_PROXY", "")
	t.Setenv("HTTPS_PROXY", "")
	t.Setenv("ALL_PROXY", "")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer ds-test-secret-key" {
			t.Fatalf("authorization = %q", got)
		}
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		var request chatRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		if request.Model != "deepseek-v4-flash" || request.Thinking.Type != "disabled" {
			t.Fatalf("unexpected request: %+v", request)
		}
		fmt.Fprint(w, `{"choices":[{"message":{"content":"这个目录不存在"}}]}`)
	}))
	defer server.Close()

	client, err := New(config.AIConfig{
		Enabled:        true,
		BaseURL:        server.URL,
		APIKey:         "ds-test-secret-key",
		Model:          "deepseek-v4-flash",
		TimeoutSeconds: 10,
		MaxInputChars:  2000,
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := client.Correct(context.Background(), "这个目录不存再")
	if err != nil {
		t.Fatal(err)
	}
	if got != "这个目录不存在" {
		t.Fatalf("result = %q", got)
	}
}
