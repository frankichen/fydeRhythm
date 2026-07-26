package api

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/frankichen/fydeRhythm/sync-server/internal/store"
	"github.com/gin-gonic/gin"
)

func testRouter(t *testing.T) http.Handler {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := store.Open(t.TempDir() + "/api.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	router, err := New(Options{
		Store: db, APIToken: "01234567890123456789012345678901",
		CORSOrigins: []string{"chrome-extension://*"}, MaxBodyBytes: 1 << 20,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatal(err)
	}
	return router
}

func TestHealthAndAuthentication(t *testing.T) {
	router := testRouter(t)

	health := httptest.NewRecorder()
	router.ServeHTTP(health, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if health.Code != http.StatusOK {
		t.Fatalf("health status = %d, body = %s", health.Code, health.Body.String())
	}

	unauthorized := httptest.NewRecorder()
	router.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, "/api/v1/stats", nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized status = %d", unauthorized.Code)
	}
}

func TestCreateAndExportLexicon(t *testing.T) {
	router := testRouter(t)
	body, _ := json.Marshal(map[string]any{"phrase": "OpenCode", "shortcut": "opc", "weight": 100, "category": "tech"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/lexicon", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer 01234567890123456789012345678901")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Device-ID", "test-device")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)
	if response.Code != http.StatusCreated {
		t.Fatalf("create status = %d body = %s", response.Code, response.Body.String())
	}

	exportReq := httptest.NewRequest(http.MethodGet, "/api/v1/lexicon/export?format=csv", nil)
	exportReq.Header.Set("Authorization", "Bearer 01234567890123456789012345678901")
	exportResponse := httptest.NewRecorder()
	router.ServeHTTP(exportResponse, exportReq)
	if exportResponse.Code != http.StatusOK {
		t.Fatalf("export status = %d body = %s", exportResponse.Code, exportResponse.Body.String())
	}
	if !bytes.Contains(exportResponse.Body.Bytes(), []byte("OpenCode")) {
		t.Fatalf("export missing entry: %s", exportResponse.Body.String())
	}
}
