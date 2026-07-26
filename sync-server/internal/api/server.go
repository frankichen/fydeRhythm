package api

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/frankichen/fydeRhythm/sync-server/internal/security"
	"github.com/frankichen/fydeRhythm/sync-server/internal/store"
	"github.com/gin-gonic/gin"
)

type Server struct {
	store        *store.Store
	maxBodyBytes int64
	logger       *slog.Logger
}

type Options struct {
	Store        *store.Store
	APIToken     string
	CORSOrigins  []string
	MaxBodyBytes int64
	Logger       *slog.Logger
}

func New(opts Options) (*gin.Engine, error) {
	if opts.Store == nil {
		return nil, errors.New("store is required")
	}
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	if opts.MaxBodyBytes <= 0 {
		opts.MaxBodyBytes = 2 << 20
	}

	s := &Server{store: opts.Store, maxBodyBytes: opts.MaxBodyBytes, logger: opts.Logger}
	r := gin.New()
	r.Use(gin.Recovery(), requestLogger(opts.Logger), security.CORS(opts.CORSOrigins))
	r.GET("/healthz", s.health)

	api := r.Group("/api/v1")
	api.Use(security.BearerAuth(opts.APIToken), security.APIHeaders(), s.limitBody())
	{
		api.POST("/sync/push", s.push)
		api.GET("/sync/pull", s.pull)
		api.GET("/sync/snapshot", s.snapshot)

		api.GET("/lexicon", s.listLexicon)
		api.POST("/lexicon", s.createLexicon)
		api.PUT("/lexicon/:id", s.updateLexicon)
		api.DELETE("/lexicon/:id", s.deleteLexicon)
		api.GET("/lexicon/export", s.exportLexicon)
		api.POST("/lexicon/import", s.importLexicon)

		api.GET("/settings", s.listSettings)
		api.PUT("/settings/:key", s.putSetting)
		api.DELETE("/settings/:key", s.deleteSetting)

		api.POST("/events", s.events)
		api.GET("/stats", s.stats)
	}
	return r, nil
}

func requestLogger(logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		started := time.Now()
		c.Next()
		logger.Info("http_request",
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"status", c.Writer.Status(),
			"duration_ms", time.Since(started).Milliseconds(),
			"client_ip", c.ClientIP(),
		)
	}
}

func (s *Server) limitBody() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Body != nil {
			c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, s.maxBodyBytes)
		}
		c.Next()
	}
}

func (s *Server) health(c *gin.Context) {
	if err := s.store.Ping(c.Request.Context()); err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"status": "unhealthy"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (s *Server) push(c *gin.Context) {
	var req store.PushRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, err)
		return
	}
	result, err := s.store.Push(c.Request.Context(), req)
	if err != nil {
		handleStoreError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

func (s *Server) pull(c *gin.Context) {
	since, err := int64Query(c, "since", 0)
	if err != nil {
		badRequest(c, err)
		return
	}
	limit64, err := int64Query(c, "limit", 500)
	if err != nil {
		badRequest(c, err)
		return
	}
	result, err := s.store.Pull(c.Request.Context(), since, int(limit64))
	if err != nil {
		internalError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

func (s *Server) snapshot(c *gin.Context) {
	result, err := s.store.Snapshot(c.Request.Context())
	if err != nil {
		internalError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

func (s *Server) listLexicon(c *gin.Context) {
	limit64, err := int64Query(c, "limit", 200)
	if err != nil {
		badRequest(c, err)
		return
	}
	offset64, err := int64Query(c, "offset", 0)
	if err != nil {
		badRequest(c, err)
		return
	}
	entries, err := s.store.ListLexicon(
		c.Request.Context(),
		c.Query("q"),
		c.Query("category"),
		boolQuery(c, "include_deleted"),
		int(limit64),
		int(offset64),
	)
	if err != nil {
		internalError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": entries, "limit": limit64, "offset": offset64})
}

type lexiconInput struct {
	ID        string `json:"id"`
	Phrase    string `json:"phrase" binding:"required"`
	Code      string `json:"code"`
	Shortcut  string `json:"shortcut"`
	Weight    int    `json:"weight"`
	Category  string `json:"category"`
	Notes     string `json:"notes"`
	UpdatedAt int64  `json:"updated_at"`
}

func (s *Server) createLexicon(c *gin.Context) {
	var input lexiconInput
	if err := c.ShouldBindJSON(&input); err != nil {
		badRequest(c, err)
		return
	}
	if strings.TrimSpace(input.ID) == "" {
		input.ID = security.NewID()
	}
	s.writeLexicon(c, input, "upsert")
}

func (s *Server) updateLexicon(c *gin.Context) {
	var input lexiconInput
	if err := c.ShouldBindJSON(&input); err != nil {
		badRequest(c, err)
		return
	}
	input.ID = c.Param("id")
	s.writeLexicon(c, input, "upsert")
}

func (s *Server) deleteLexicon(c *gin.Context) {
	input := lexiconInput{ID: c.Param("id"), UpdatedAt: time.Now().UnixMilli()}
	s.writeLexicon(c, input, "delete")
}

func (s *Server) writeLexicon(c *gin.Context, input lexiconInput, operation string) {
	deviceID, ok := requireDeviceID(c)
	if !ok {
		return
	}
	if input.UpdatedAt <= 0 {
		input.UpdatedAt = time.Now().UnixMilli()
	}
	payload, _ := json.Marshal(store.LexiconEntry{
		ID:       strings.TrimSpace(input.ID),
		Phrase:   strings.TrimSpace(input.Phrase),
		Code:     strings.TrimSpace(input.Code),
		Shortcut: strings.TrimSpace(input.Shortcut),
		Weight:   input.Weight,
		Category: strings.TrimSpace(input.Category),
		Notes:    strings.TrimSpace(input.Notes),
	})
	result, err := s.store.Push(c.Request.Context(), store.PushRequest{
		DeviceID: deviceID,
		Changes: []store.Change{{
			ChangeID:        security.NewID(),
			Entity:          "lexicon",
			Operation:       operation,
			ClientUpdatedAt: input.UpdatedAt,
			Payload:         payload,
		}},
	})
	if err != nil {
		handleStoreError(c, err)
		return
	}
	if len(result.Rejected) > 0 {
		badRequest(c, errors.New(result.Rejected[0].Reason))
		return
	}
	status := http.StatusOK
	if operation == "upsert" {
		status = http.StatusCreated
	}
	c.JSON(status, gin.H{"id": input.ID, "sync": result})
}

func (s *Server) exportLexicon(c *gin.Context) {
	entries, err := s.listAllLexicon(c.Request.Context(), boolQuery(c, "include_deleted"))
	if err != nil {
		internalError(c, err)
		return
	}
	format := strings.ToLower(strings.TrimSpace(c.DefaultQuery("format", "json")))
	switch format {
	case "json":
		c.Header("Content-Disposition", `attachment; filename="fyderhythm-lexicon.json"`)
		c.JSON(http.StatusOK, entries)
	case "csv":
		c.Header("Content-Type", "text/csv; charset=utf-8")
		c.Header("Content-Disposition", `attachment; filename="fyderhythm-lexicon.csv"`)
		writer := csv.NewWriter(c.Writer)
		_ = writer.Write([]string{"id", "phrase", "code", "shortcut", "weight", "category", "notes", "deleted", "client_updated_at"})
		for _, entry := range entries {
			_ = writer.Write([]string{
				entry.ID, entry.Phrase, entry.Code, entry.Shortcut, strconv.Itoa(entry.Weight),
				entry.Category, entry.Notes, strconv.FormatBool(entry.Deleted), strconv.FormatInt(entry.ClientUpdatedAt, 10),
			})
		}
		writer.Flush()
		if err := writer.Error(); err != nil {
			s.logger.Error("write csv", "error", err)
		}
	default:
		badRequest(c, errors.New("format must be json or csv"))
	}
}

func (s *Server) listAllLexicon(ctx context.Context, includeDeleted bool) ([]store.LexiconEntry, error) {
	const pageSize = 1000
	entries := make([]store.LexiconEntry, 0, pageSize)
	for offset := 0; ; offset += pageSize {
		page, err := s.store.ListLexicon(ctx, "", "", includeDeleted, pageSize, offset)
		if err != nil {
			return nil, err
		}
		entries = append(entries, page...)
		if len(page) < pageSize {
			return entries, nil
		}
		if len(entries) >= 100000 {
			return nil, errors.New("lexicon export exceeds 100000 entries")
		}
	}
}
func (s *Server) importLexicon(c *gin.Context) {
	deviceID, ok := requireDeviceID(c)
	if !ok {
		return
	}
	format := strings.ToLower(strings.TrimSpace(c.DefaultQuery("format", "json")))
	mode := strings.ToLower(strings.TrimSpace(c.DefaultQuery("mode", "merge")))
	if mode != "merge" && mode != "replace" {
		badRequest(c, errors.New("mode must be merge or replace"))
		return
	}

	var entries []lexiconInput
	var err error
	switch format {
	case "json":
		err = json.NewDecoder(c.Request.Body).Decode(&entries)
	case "csv":
		entries, err = readLexiconCSV(c.Request.Body)
	default:
		err = errors.New("format must be json or csv")
	}
	if err != nil {
		badRequest(c, err)
		return
	}
	if len(entries) == 0 {
		badRequest(c, errors.New("import contains no entries"))
		return
	}
	if len(entries) > 500 {
		badRequest(c, errors.New("at most 500 entries can be imported at once"))
		return
	}

	now := time.Now().UnixMilli()
	changes := make([]store.Change, 0, len(entries)+64)
	if mode == "replace" {
		current, err := s.store.ListLexicon(c.Request.Context(), "", "", false, 1000, 0)
		if err != nil {
			internalError(c, err)
			return
		}
		if len(current)+len(entries) > 500 {
			badRequest(c, errors.New("replace would exceed the 500-change transaction limit"))
			return
		}
		for _, item := range current {
			payload, _ := json.Marshal(store.LexiconEntry{ID: item.ID})
			changes = append(changes, store.Change{ChangeID: security.NewID(), Entity: "lexicon", Operation: "delete", ClientUpdatedAt: now, Payload: payload})
		}
	}
	for i, item := range entries {
		if strings.TrimSpace(item.ID) == "" {
			item.ID = security.NewID()
		}
		if strings.TrimSpace(item.Phrase) == "" {
			badRequest(c, fmt.Errorf("entry %d has an empty phrase", i+1))
			return
		}
		updatedAt := item.UpdatedAt
		if updatedAt <= 0 {
			updatedAt = now + int64(i)
		}
		payload, _ := json.Marshal(store.LexiconEntry{
			ID: item.ID, Phrase: item.Phrase, Code: item.Code, Shortcut: item.Shortcut,
			Weight: item.Weight, Category: item.Category, Notes: item.Notes,
		})
		changes = append(changes, store.Change{ChangeID: security.NewID(), Entity: "lexicon", Operation: "upsert", ClientUpdatedAt: updatedAt, Payload: payload})
	}
	result, err := s.store.Push(c.Request.Context(), store.PushRequest{DeviceID: deviceID, Changes: changes})
	if err != nil {
		handleStoreError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"imported": len(entries), "mode": mode, "sync": result})
}

func readLexiconCSV(reader io.Reader) ([]lexiconInput, error) {
	r := csv.NewReader(reader)
	r.FieldsPerRecord = -1
	records, err := r.ReadAll()
	if err != nil {
		return nil, err
	}
	if len(records) < 2 {
		return nil, errors.New("CSV must include a header and at least one row")
	}
	headers := make(map[string]int)
	for index, header := range records[0] {
		headers[strings.ToLower(strings.TrimSpace(header))] = index
	}
	value := func(row []string, key string) string {
		index, ok := headers[key]
		if !ok || index >= len(row) {
			return ""
		}
		return strings.TrimSpace(row[index])
	}
	entries := make([]lexiconInput, 0, len(records)-1)
	for rowIndex, row := range records[1:] {
		weight := 0
		if raw := value(row, "weight"); raw != "" {
			weight, err = strconv.Atoi(raw)
			if err != nil {
				return nil, fmt.Errorf("row %d has invalid weight", rowIndex+2)
			}
		}
		updatedAt := int64(0)
		if raw := value(row, "client_updated_at"); raw != "" {
			updatedAt, err = strconv.ParseInt(raw, 10, 64)
			if err != nil {
				return nil, fmt.Errorf("row %d has invalid client_updated_at", rowIndex+2)
			}
		}
		entries = append(entries, lexiconInput{
			ID: value(row, "id"), Phrase: value(row, "phrase"), Code: value(row, "code"),
			Shortcut: value(row, "shortcut"), Weight: weight, Category: value(row, "category"),
			Notes: value(row, "notes"), UpdatedAt: updatedAt,
		})
	}
	return entries, nil
}

func (s *Server) listSettings(c *gin.Context) {
	items, err := s.store.ListSettings(c.Request.Context(), boolQuery(c, "include_deleted"))
	if err != nil {
		internalError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

func (s *Server) putSetting(c *gin.Context) {
	deviceID, ok := requireDeviceID(c)
	if !ok {
		return
	}
	var input struct {
		Value     json.RawMessage `json:"value" binding:"required"`
		UpdatedAt int64           `json:"updated_at"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		badRequest(c, err)
		return
	}
	if input.UpdatedAt <= 0 {
		input.UpdatedAt = time.Now().UnixMilli()
	}
	payload, _ := json.Marshal(store.Setting{Key: c.Param("key"), Value: input.Value})
	result, err := s.store.Push(c.Request.Context(), store.PushRequest{DeviceID: deviceID, Changes: []store.Change{{
		ChangeID: security.NewID(), Entity: "setting", Operation: "upsert", ClientUpdatedAt: input.UpdatedAt, Payload: payload,
	}}})
	if err != nil {
		handleStoreError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

func (s *Server) deleteSetting(c *gin.Context) {
	deviceID, ok := requireDeviceID(c)
	if !ok {
		return
	}
	payload, _ := json.Marshal(store.Setting{Key: c.Param("key"), Value: json.RawMessage("null")})
	result, err := s.store.Push(c.Request.Context(), store.PushRequest{DeviceID: deviceID, Changes: []store.Change{{
		ChangeID: security.NewID(), Entity: "setting", Operation: "delete", ClientUpdatedAt: time.Now().UnixMilli(), Payload: payload,
	}}})
	if err != nil {
		handleStoreError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

func (s *Server) events(c *gin.Context) {
	deviceID, ok := requireDeviceID(c)
	if !ok {
		return
	}
	var input struct {
		Changes []store.Change `json:"changes" binding:"required"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		badRequest(c, err)
		return
	}
	for index := range input.Changes {
		if input.Changes[index].ChangeID == "" {
			input.Changes[index].ChangeID = security.NewID()
		}
		if input.Changes[index].ClientUpdatedAt <= 0 {
			input.Changes[index].ClientUpdatedAt = time.Now().UnixMilli()
		}
		if input.Changes[index].Entity != "candidate_usage" && input.Changes[index].Entity != "correction" && input.Changes[index].Entity != "ai_feedback" {
			badRequest(c, fmt.Errorf("event %d has unsupported entity", index+1))
			return
		}
	}
	result, err := s.store.Push(c.Request.Context(), store.PushRequest{DeviceID: deviceID, Changes: input.Changes})
	if err != nil {
		handleStoreError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

func (s *Server) stats(c *gin.Context) {
	result, err := s.store.Stats(c.Request.Context())
	if err != nil {
		internalError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

func requireDeviceID(c *gin.Context) (string, bool) {
	deviceID := strings.TrimSpace(c.GetHeader("X-Device-ID"))
	if deviceID == "" || len(deviceID) > 128 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "X-Device-ID header is required and must be at most 128 characters"})
		return "", false
	}
	return deviceID, true
}

func int64Query(c *gin.Context, key string, fallback int64) (int64, error) {
	raw := strings.TrimSpace(c.Query(key))
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer", key)
	}
	return value, nil
}

func boolQuery(c *gin.Context, key string) bool {
	value, _ := strconv.ParseBool(c.Query(key))
	return value
}

func badRequest(c *gin.Context, err error) {
	c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
}

func internalError(c *gin.Context, err error) {
	_ = c.Error(err)
	c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
}

func handleStoreError(c *gin.Context, err error) {
	if errors.Is(err, store.ErrInvalidChange) {
		badRequest(c, err)
		return
	}
	internalError(c, err)
}
