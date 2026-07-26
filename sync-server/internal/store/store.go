package store

import (
	"context"
	"database/sql"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schemaSQL string

var (
	ErrInvalidChange = errors.New("invalid change")
	ErrNotFound      = errors.New("not found")
)

type Store struct {
	db *sql.DB
}

func Open(path string) (*Store, error) {
	if path == "" {
		return nil, errors.New("database path is required")
	}
	if path != ":memory:" && !strings.HasPrefix(path, "file:") {
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			return nil, fmt.Errorf("create database directory: %w", err)
		}
	}

	dsn := path
	if path != ":memory:" && !strings.HasPrefix(path, "file:") {
		dsn = "file:" + path
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for _, pragma := range []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA synchronous=NORMAL",
		"PRAGMA foreign_keys=ON",
		"PRAGMA busy_timeout=5000",
	} {
		if _, err := db.ExecContext(ctx, pragma); err != nil {
			db.Close()
			return nil, fmt.Errorf("apply %s: %w", pragma, err)
		}
	}
	if _, err := db.ExecContext(ctx, schemaSQL); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate database: %w", err)
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) Ping(ctx context.Context) error { return s.db.PingContext(ctx) }

func (s *Store) Push(ctx context.Context, req PushRequest) (PushResult, error) {
	if strings.TrimSpace(req.DeviceID) == "" {
		return PushResult{}, fmt.Errorf("%w: device_id is required", ErrInvalidChange)
	}
	if len(req.Changes) > 500 {
		return PushResult{}, fmt.Errorf("%w: at most 500 changes per push", ErrInvalidChange)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return PushResult{}, err
	}
	defer tx.Rollback()

	now := time.Now().UnixMilli()
	if err := upsertDevice(ctx, tx, req.DeviceID, req.Device, now); err != nil {
		return PushResult{}, err
	}

	result := PushResult{Rejected: make([]RejectedChange, 0)}
	for _, change := range req.Changes {
		if strings.TrimSpace(change.ChangeID) == "" {
			result.Rejected = append(result.Rejected, RejectedChange{Reason: "change_id is required"})
			continue
		}
		duplicate, err := changeAlreadyApplied(ctx, tx, change.ChangeID)
		if err != nil {
			return PushResult{}, err
		}
		if duplicate {
			result.Duplicates++
			continue
		}

		applied, conflict, normalized, entityKey, err := applyChange(ctx, tx, req.DeviceID, change, now)
		if err != nil {
			result.Rejected = append(result.Rejected, RejectedChange{ChangeID: change.ChangeID, Reason: err.Error()})
			continue
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO applied_changes(change_id, device_id, applied_at) VALUES(?, ?, ?)`,
			change.ChangeID, req.DeviceID, now,
		); err != nil {
			return PushResult{}, err
		}
		if conflict {
			result.Conflicts++
		}
		if !applied {
			continue
		}
		payloadJSON, err := json.Marshal(normalized)
		if err != nil {
			return PushResult{}, err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO sync_log(change_id, entity, operation, entity_key, payload_json, source_device_id, created_at)
			VALUES(?, ?, ?, ?, ?, ?, ?)`,
			change.ChangeID, change.Entity, change.Operation, entityKey, string(payloadJSON), req.DeviceID, now,
		); err != nil {
			return PushResult{}, err
		}
		result.Applied++
	}

	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(seq), 0) FROM sync_log`).Scan(&result.Cursor); err != nil {
		return PushResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return PushResult{}, err
	}
	return result, nil
}

func upsertDevice(ctx context.Context, tx *sql.Tx, deviceID string, device *Device, now int64) error {
	name, platform, appVersion := "", "", ""
	if device != nil {
		name = strings.TrimSpace(device.Name)
		platform = strings.TrimSpace(device.Platform)
		appVersion = strings.TrimSpace(device.AppVersion)
	}
	_, err := tx.ExecContext(ctx, `
		INSERT INTO devices(device_id, name, platform, app_version, last_seen_at)
		VALUES(?, ?, ?, ?, ?)
		ON CONFLICT(device_id) DO UPDATE SET
		  name = CASE WHEN excluded.name <> '' THEN excluded.name ELSE devices.name END,
		  platform = CASE WHEN excluded.platform <> '' THEN excluded.platform ELSE devices.platform END,
		  app_version = CASE WHEN excluded.app_version <> '' THEN excluded.app_version ELSE devices.app_version END,
		  last_seen_at = excluded.last_seen_at`, deviceID, name, platform, appVersion, now)
	return err
}

func changeAlreadyApplied(ctx context.Context, tx *sql.Tx, changeID string) (bool, error) {
	var n int
	err := tx.QueryRowContext(ctx, `SELECT 1 FROM applied_changes WHERE change_id = ?`, changeID).Scan(&n)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

func applyChange(ctx context.Context, tx *sql.Tx, deviceID string, change Change, now int64) (applied bool, conflict bool, normalized any, entityKey string, err error) {
	change.Entity = strings.TrimSpace(change.Entity)
	change.Operation = strings.TrimSpace(change.Operation)
	if change.ClientUpdatedAt <= 0 {
		change.ClientUpdatedAt = now
	}

	switch change.Entity {
	case "lexicon":
		return applyLexicon(ctx, tx, deviceID, change, now)
	case "candidate_usage":
		return applyCandidateUsage(ctx, tx, change, now)
	case "correction":
		return applyCorrection(ctx, tx, change, now)
	case "ai_feedback":
		return applyAIFeedback(ctx, tx, change, now)
	case "setting":
		return applySetting(ctx, tx, deviceID, change, now)
	default:
		return false, false, nil, "", fmt.Errorf("%w: unsupported entity %q", ErrInvalidChange, change.Entity)
	}
}

func applyLexicon(ctx context.Context, tx *sql.Tx, deviceID string, change Change, now int64) (bool, bool, any, string, error) {
	var incoming LexiconEntry
	if err := json.Unmarshal(change.Payload, &incoming); err != nil {
		return false, false, nil, "", fmt.Errorf("invalid lexicon payload: %w", err)
	}
	incoming.ID = strings.TrimSpace(incoming.ID)
	if incoming.ID == "" {
		return false, false, nil, "", fmt.Errorf("%w: lexicon.id is required", ErrInvalidChange)
	}
	if change.Operation != "upsert" && change.Operation != "delete" {
		return false, false, nil, "", fmt.Errorf("%w: lexicon operation must be upsert or delete", ErrInvalidChange)
	}
	if change.Operation == "upsert" {
		incoming.Phrase = strings.TrimSpace(incoming.Phrase)
		if incoming.Phrase == "" {
			return false, false, nil, "", fmt.Errorf("%w: lexicon.phrase is required", ErrInvalidChange)
		}
	}
	incoming.Code = strings.TrimSpace(incoming.Code)
	incoming.Shortcut = strings.TrimSpace(incoming.Shortcut)
	incoming.Category = strings.TrimSpace(incoming.Category)
	incoming.Notes = strings.TrimSpace(incoming.Notes)
	incoming.ClientUpdatedAt = change.ClientUpdatedAt
	incoming.SourceDeviceID = deviceID
	incoming.ServerUpdatedAt = now
	incoming.Deleted = change.Operation == "delete"

	var current LexiconEntry
	var deleted int
	err := tx.QueryRowContext(ctx, `
		SELECT id, phrase, code, shortcut, weight, category, notes, deleted,
		       client_updated_at, source_device_id, server_updated_at, version
		FROM lexicon WHERE id = ?`, incoming.ID).Scan(
		&current.ID, &current.Phrase, &current.Code, &current.Shortcut, &current.Weight,
		&current.Category, &current.Notes, &deleted, &current.ClientUpdatedAt,
		&current.SourceDeviceID, &current.ServerUpdatedAt, &current.Version,
	)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return false, false, nil, "", err
	}
	if err == nil {
		current.Deleted = deleted != 0
		if !isNewer(change.ClientUpdatedAt, deviceID, current.ClientUpdatedAt, current.SourceDeviceID) {
			return false, true, current, incoming.ID, nil
		}
		if change.Operation == "delete" {
			incoming.Phrase = current.Phrase
			incoming.Code = current.Code
			incoming.Shortcut = current.Shortcut
			incoming.Weight = current.Weight
			incoming.Category = current.Category
			incoming.Notes = current.Notes
		}
		incoming.Version = current.Version + 1
	} else {
		incoming.Version = 1
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO lexicon(id, phrase, code, shortcut, weight, category, notes, deleted,
		                    client_updated_at, source_device_id, server_updated_at, version)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
		 phrase=excluded.phrase, code=excluded.code, shortcut=excluded.shortcut,
		 weight=excluded.weight, category=excluded.category, notes=excluded.notes,
		 deleted=excluded.deleted, client_updated_at=excluded.client_updated_at,
		 source_device_id=excluded.source_device_id, server_updated_at=excluded.server_updated_at,
		 version=excluded.version`,
		incoming.ID, incoming.Phrase, incoming.Code, incoming.Shortcut, incoming.Weight,
		incoming.Category, incoming.Notes, boolInt(incoming.Deleted), incoming.ClientUpdatedAt,
		incoming.SourceDeviceID, incoming.ServerUpdatedAt, incoming.Version,
	)
	if err != nil {
		return false, false, nil, "", err
	}
	return true, false, incoming, incoming.ID, nil
}

func applyCandidateUsage(ctx context.Context, tx *sql.Tx, change Change, now int64) (bool, bool, any, string, error) {
	if change.Operation != "increment" {
		return false, false, nil, "", fmt.Errorf("%w: candidate_usage operation must be increment", ErrInvalidChange)
	}
	var payload struct {
		Code       string `json:"code"`
		Candidate  string `json:"candidate"`
		Delta      int64  `json:"delta"`
		LastUsedAt int64  `json:"last_used_at"`
	}
	if err := json.Unmarshal(change.Payload, &payload); err != nil {
		return false, false, nil, "", fmt.Errorf("invalid candidate_usage payload: %w", err)
	}
	payload.Code = strings.TrimSpace(payload.Code)
	payload.Candidate = strings.TrimSpace(payload.Candidate)
	if payload.Code == "" || payload.Candidate == "" || payload.Delta <= 0 || payload.Delta > 100000 {
		return false, false, nil, "", fmt.Errorf("%w: code, candidate and positive delta are required", ErrInvalidChange)
	}
	if payload.LastUsedAt <= 0 {
		payload.LastUsedAt = now
	}
	_, err := tx.ExecContext(ctx, `
		INSERT INTO candidate_usage(code, candidate, count, last_used_at) VALUES(?, ?, ?, ?)
		ON CONFLICT(code, candidate) DO UPDATE SET
		 count = candidate_usage.count + excluded.count,
		 last_used_at = MAX(candidate_usage.last_used_at, excluded.last_used_at)`,
		payload.Code, payload.Candidate, payload.Delta, payload.LastUsedAt)
	if err != nil {
		return false, false, nil, "", err
	}
	return true, false, payload, payload.Code + "\x00" + payload.Candidate, nil
}

func applyCorrection(ctx context.Context, tx *sql.Tx, change Change, now int64) (bool, bool, any, string, error) {
	if change.Operation != "increment" {
		return false, false, nil, "", fmt.Errorf("%w: correction operation must be increment", ErrInvalidChange)
	}
	var payload struct {
		Original   string `json:"original"`
		Corrected  string `json:"corrected"`
		Delta      int64  `json:"delta"`
		LastUsedAt int64  `json:"last_used_at"`
	}
	if err := json.Unmarshal(change.Payload, &payload); err != nil {
		return false, false, nil, "", fmt.Errorf("invalid correction payload: %w", err)
	}
	payload.Original = strings.TrimSpace(payload.Original)
	payload.Corrected = strings.TrimSpace(payload.Corrected)
	if payload.Original == "" || payload.Corrected == "" || payload.Original == payload.Corrected || payload.Delta <= 0 {
		return false, false, nil, "", fmt.Errorf("%w: distinct original/corrected and positive delta are required", ErrInvalidChange)
	}
	if payload.LastUsedAt <= 0 {
		payload.LastUsedAt = now
	}
	_, err := tx.ExecContext(ctx, `
		INSERT INTO corrections(original, corrected, count, last_used_at) VALUES(?, ?, ?, ?)
		ON CONFLICT(original, corrected) DO UPDATE SET
		 count = corrections.count + excluded.count,
		 last_used_at = MAX(corrections.last_used_at, excluded.last_used_at)`,
		payload.Original, payload.Corrected, payload.Delta, payload.LastUsedAt)
	if err != nil {
		return false, false, nil, "", err
	}
	return true, false, payload, payload.Original + "\x00" + payload.Corrected, nil
}

func applyAIFeedback(ctx context.Context, tx *sql.Tx, change Change, now int64) (bool, bool, any, string, error) {
	if change.Operation != "increment" {
		return false, false, nil, "", fmt.Errorf("%w: ai_feedback operation must be increment", ErrInvalidChange)
	}
	var payload struct {
		Feature       string `json:"feature"`
		ContextHash   string `json:"context_hash"`
		AcceptedDelta int64  `json:"accepted_delta"`
		RejectedDelta int64  `json:"rejected_delta"`
		LastUsedAt    int64  `json:"last_used_at"`
	}
	if err := json.Unmarshal(change.Payload, &payload); err != nil {
		return false, false, nil, "", fmt.Errorf("invalid ai_feedback payload: %w", err)
	}
	payload.Feature = strings.TrimSpace(payload.Feature)
	payload.ContextHash = strings.TrimSpace(payload.ContextHash)
	if payload.Feature == "" || payload.AcceptedDelta < 0 || payload.RejectedDelta < 0 || payload.AcceptedDelta+payload.RejectedDelta <= 0 {
		return false, false, nil, "", fmt.Errorf("%w: feature and a positive feedback delta are required", ErrInvalidChange)
	}
	if len(payload.ContextHash) > 128 {
		return false, false, nil, "", fmt.Errorf("%w: context_hash is too long", ErrInvalidChange)
	}
	if payload.LastUsedAt <= 0 {
		payload.LastUsedAt = now
	}
	_, err := tx.ExecContext(ctx, `
		INSERT INTO ai_feedback(feature, context_hash, accepted_count, rejected_count, last_used_at)
		VALUES(?, ?, ?, ?, ?)
		ON CONFLICT(feature, context_hash) DO UPDATE SET
		 accepted_count = ai_feedback.accepted_count + excluded.accepted_count,
		 rejected_count = ai_feedback.rejected_count + excluded.rejected_count,
		 last_used_at = MAX(ai_feedback.last_used_at, excluded.last_used_at)`,
		payload.Feature, payload.ContextHash, payload.AcceptedDelta, payload.RejectedDelta, payload.LastUsedAt)
	if err != nil {
		return false, false, nil, "", err
	}
	return true, false, payload, payload.Feature + "\x00" + payload.ContextHash, nil
}

func applySetting(ctx context.Context, tx *sql.Tx, deviceID string, change Change, now int64) (bool, bool, any, string, error) {
	if change.Operation != "upsert" && change.Operation != "delete" {
		return false, false, nil, "", fmt.Errorf("%w: setting operation must be upsert or delete", ErrInvalidChange)
	}
	var incoming Setting
	if err := json.Unmarshal(change.Payload, &incoming); err != nil {
		return false, false, nil, "", fmt.Errorf("invalid setting payload: %w", err)
	}
	incoming.Key = strings.TrimSpace(incoming.Key)
	if incoming.Key == "" || len(incoming.Key) > 128 {
		return false, false, nil, "", fmt.Errorf("%w: valid setting.key is required", ErrInvalidChange)
	}
	if len(incoming.Value) == 0 {
		incoming.Value = json.RawMessage("null")
	}
	if !json.Valid(incoming.Value) {
		return false, false, nil, "", fmt.Errorf("%w: setting.value must be valid JSON", ErrInvalidChange)
	}
	incoming.ClientUpdatedAt = change.ClientUpdatedAt
	incoming.SourceDeviceID = deviceID
	incoming.ServerUpdatedAt = now
	incoming.Deleted = change.Operation == "delete"

	var current Setting
	var currentValue string
	var deleted int
	err := tx.QueryRowContext(ctx, `
		SELECT key, value_json, deleted, client_updated_at, source_device_id, server_updated_at, version
		FROM settings WHERE key = ?`, incoming.Key).Scan(
		&current.Key, &currentValue, &deleted, &current.ClientUpdatedAt,
		&current.SourceDeviceID, &current.ServerUpdatedAt, &current.Version,
	)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return false, false, nil, "", err
	}
	if err == nil {
		current.Value = json.RawMessage(currentValue)
		current.Deleted = deleted != 0
		if !isNewer(change.ClientUpdatedAt, deviceID, current.ClientUpdatedAt, current.SourceDeviceID) {
			return false, true, current, incoming.Key, nil
		}
		incoming.Version = current.Version + 1
	} else {
		incoming.Version = 1
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO settings(key, value_json, deleted, client_updated_at, source_device_id, server_updated_at, version)
		VALUES(?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET
		 value_json=excluded.value_json, deleted=excluded.deleted,
		 client_updated_at=excluded.client_updated_at, source_device_id=excluded.source_device_id,
		 server_updated_at=excluded.server_updated_at, version=excluded.version`,
		incoming.Key, string(incoming.Value), boolInt(incoming.Deleted), incoming.ClientUpdatedAt,
		incoming.SourceDeviceID, incoming.ServerUpdatedAt, incoming.Version)
	if err != nil {
		return false, false, nil, "", err
	}
	return true, false, incoming, incoming.Key, nil
}

func isNewer(incomingTime int64, incomingDevice string, currentTime int64, currentDevice string) bool {
	if incomingTime != currentTime {
		return incomingTime > currentTime
	}
	return incomingDevice > currentDevice
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func (s *Store) Pull(ctx context.Context, since int64, limit int) (PullResult, error) {
	if since < 0 {
		since = 0
	}
	if limit <= 0 || limit > 1000 {
		limit = 500
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT seq, change_id, entity, operation, entity_key, payload_json, source_device_id, created_at
		FROM sync_log WHERE seq > ? ORDER BY seq ASC LIMIT ?`, since, limit+1)
	if err != nil {
		return PullResult{}, err
	}
	defer rows.Close()

	result := PullResult{Cursor: since, Changes: make([]LogChange, 0, limit)}
	for rows.Next() {
		var item LogChange
		var payload string
		if err := rows.Scan(&item.Seq, &item.ChangeID, &item.Entity, &item.Operation, &item.EntityKey, &payload, &item.SourceDeviceID, &item.CreatedAt); err != nil {
			return PullResult{}, err
		}
		if len(result.Changes) == limit {
			result.HasMore = true
			break
		}
		item.Payload = json.RawMessage(payload)
		result.Changes = append(result.Changes, item)
		result.Cursor = item.Seq
	}
	return result, rows.Err()
}

func (s *Store) ListLexicon(ctx context.Context, query, category string, includeDeleted bool, limit, offset int) ([]LexiconEntry, error) {
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	if offset < 0 {
		offset = 0
	}
	where := []string{"1=1"}
	args := make([]any, 0, 6)
	if !includeDeleted {
		where = append(where, "deleted = 0")
	}
	if query = strings.TrimSpace(query); query != "" {
		where = append(where, "(phrase LIKE ? OR code LIKE ? OR shortcut LIKE ?)")
		like := "%" + query + "%"
		args = append(args, like, like, like)
	}
	if category = strings.TrimSpace(category); category != "" {
		where = append(where, "category = ?")
		args = append(args, category)
	}
	args = append(args, limit, offset)
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, phrase, code, shortcut, weight, category, notes, deleted,
		       client_updated_at, source_device_id, server_updated_at, version
		FROM lexicon WHERE `+strings.Join(where, " AND ")+`
		ORDER BY weight DESC, phrase ASC LIMIT ? OFFSET ?`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	entries := make([]LexiconEntry, 0)
	for rows.Next() {
		var entry LexiconEntry
		var deleted int
		if err := rows.Scan(&entry.ID, &entry.Phrase, &entry.Code, &entry.Shortcut, &entry.Weight,
			&entry.Category, &entry.Notes, &deleted, &entry.ClientUpdatedAt,
			&entry.SourceDeviceID, &entry.ServerUpdatedAt, &entry.Version); err != nil {
			return nil, err
		}
		entry.Deleted = deleted != 0
		entries = append(entries, entry)
	}
	return entries, rows.Err()
}

func (s *Store) ListSettings(ctx context.Context, includeDeleted bool) ([]Setting, error) {
	query := `SELECT key, value_json, deleted, client_updated_at, source_device_id, server_updated_at, version FROM settings`
	if !includeDeleted {
		query += ` WHERE deleted = 0`
	}
	query += ` ORDER BY key ASC`
	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	settings := make([]Setting, 0)
	for rows.Next() {
		var item Setting
		var value string
		var deleted int
		if err := rows.Scan(&item.Key, &value, &deleted, &item.ClientUpdatedAt, &item.SourceDeviceID, &item.ServerUpdatedAt, &item.Version); err != nil {
			return nil, err
		}
		item.Value = json.RawMessage(value)
		item.Deleted = deleted != 0
		settings = append(settings, item)
	}
	return settings, rows.Err()
}

type Stats struct {
	LexiconCount    int64            `json:"lexicon_count"`
	DeletedCount    int64            `json:"deleted_count"`
	CandidatePairs  int64            `json:"candidate_pairs"`
	CorrectionPairs int64            `json:"correction_pairs"`
	Devices         int64            `json:"devices"`
	SyncCursor      int64            `json:"sync_cursor"`
	TopCandidates   []CandidateUsage `json:"top_candidates"`
	TopCorrections  []Correction     `json:"top_corrections"`
	Feedback        []AIFeedback     `json:"ai_feedback"`
}

func (s *Store) Stats(ctx context.Context) (Stats, error) {
	var out Stats
	queries := []struct {
		query string
		dest  *int64
	}{
		{`SELECT COUNT(*) FROM lexicon WHERE deleted = 0`, &out.LexiconCount},
		{`SELECT COUNT(*) FROM lexicon WHERE deleted = 1`, &out.DeletedCount},
		{`SELECT COUNT(*) FROM candidate_usage`, &out.CandidatePairs},
		{`SELECT COUNT(*) FROM corrections`, &out.CorrectionPairs},
		{`SELECT COUNT(*) FROM devices`, &out.Devices},
		{`SELECT COALESCE(MAX(seq), 0) FROM sync_log`, &out.SyncCursor},
	}
	for _, item := range queries {
		if err := s.db.QueryRowContext(ctx, item.query).Scan(item.dest); err != nil {
			return Stats{}, err
		}
	}

	candidateRows, err := s.db.QueryContext(ctx, `SELECT code, candidate, count, last_used_at FROM candidate_usage ORDER BY count DESC, last_used_at DESC LIMIT 20`)
	if err != nil {
		return Stats{}, err
	}
	for candidateRows.Next() {
		var item CandidateUsage
		if err := candidateRows.Scan(&item.Code, &item.Candidate, &item.Count, &item.LastUsedAt); err != nil {
			candidateRows.Close()
			return Stats{}, err
		}
		out.TopCandidates = append(out.TopCandidates, item)
	}
	candidateRows.Close()

	correctionRows, err := s.db.QueryContext(ctx, `SELECT original, corrected, count, last_used_at FROM corrections ORDER BY count DESC, last_used_at DESC LIMIT 20`)
	if err != nil {
		return Stats{}, err
	}
	for correctionRows.Next() {
		var item Correction
		if err := correctionRows.Scan(&item.Original, &item.Corrected, &item.Count, &item.LastUsedAt); err != nil {
			correctionRows.Close()
			return Stats{}, err
		}
		out.TopCorrections = append(out.TopCorrections, item)
	}
	correctionRows.Close()

	feedbackRows, err := s.db.QueryContext(ctx, `SELECT feature, context_hash, accepted_count, rejected_count, last_used_at FROM ai_feedback ORDER BY feature, accepted_count DESC`)
	if err != nil {
		return Stats{}, err
	}
	for feedbackRows.Next() {
		var item AIFeedback
		if err := feedbackRows.Scan(&item.Feature, &item.ContextHash, &item.AcceptedCount, &item.RejectedCount, &item.LastUsedAt); err != nil {
			feedbackRows.Close()
			return Stats{}, err
		}
		out.Feedback = append(out.Feedback, item)
	}
	feedbackRows.Close()
	return out, nil
}

func (s *Store) Snapshot(ctx context.Context) (Snapshot, error) {
	out := Snapshot{GeneratedAt: time.Now().UnixMilli()}
	if err := s.db.QueryRowContext(ctx, `SELECT COALESCE(MAX(seq), 0) FROM sync_log`).Scan(&out.Cursor); err != nil {
		return Snapshot{}, err
	}

	lexiconRows, err := s.db.QueryContext(ctx, `
		SELECT id, phrase, code, shortcut, weight, category, notes, deleted,
		       client_updated_at, source_device_id, server_updated_at, version
		FROM lexicon ORDER BY phrase ASC`)
	if err != nil {
		return Snapshot{}, err
	}
	for lexiconRows.Next() {
		var item LexiconEntry
		var deleted int
		if err := lexiconRows.Scan(&item.ID, &item.Phrase, &item.Code, &item.Shortcut, &item.Weight,
			&item.Category, &item.Notes, &deleted, &item.ClientUpdatedAt,
			&item.SourceDeviceID, &item.ServerUpdatedAt, &item.Version); err != nil {
			lexiconRows.Close()
			return Snapshot{}, err
		}
		item.Deleted = deleted != 0
		out.Lexicon = append(out.Lexicon, item)
	}
	if err := lexiconRows.Close(); err != nil {
		return Snapshot{}, err
	}

	usageRows, err := s.db.QueryContext(ctx, `SELECT code, candidate, count, last_used_at FROM candidate_usage ORDER BY code, candidate`)
	if err != nil {
		return Snapshot{}, err
	}
	for usageRows.Next() {
		var item CandidateUsage
		if err := usageRows.Scan(&item.Code, &item.Candidate, &item.Count, &item.LastUsedAt); err != nil {
			usageRows.Close()
			return Snapshot{}, err
		}
		out.CandidateUsage = append(out.CandidateUsage, item)
	}
	if err := usageRows.Close(); err != nil {
		return Snapshot{}, err
	}

	correctionRows, err := s.db.QueryContext(ctx, `SELECT original, corrected, count, last_used_at FROM corrections ORDER BY original, corrected`)
	if err != nil {
		return Snapshot{}, err
	}
	for correctionRows.Next() {
		var item Correction
		if err := correctionRows.Scan(&item.Original, &item.Corrected, &item.Count, &item.LastUsedAt); err != nil {
			correctionRows.Close()
			return Snapshot{}, err
		}
		out.Corrections = append(out.Corrections, item)
	}
	if err := correctionRows.Close(); err != nil {
		return Snapshot{}, err
	}

	feedbackRows, err := s.db.QueryContext(ctx, `SELECT feature, context_hash, accepted_count, rejected_count, last_used_at FROM ai_feedback ORDER BY feature, context_hash`)
	if err != nil {
		return Snapshot{}, err
	}
	for feedbackRows.Next() {
		var item AIFeedback
		if err := feedbackRows.Scan(&item.Feature, &item.ContextHash, &item.AcceptedCount, &item.RejectedCount, &item.LastUsedAt); err != nil {
			feedbackRows.Close()
			return Snapshot{}, err
		}
		out.AIFeedback = append(out.AIFeedback, item)
	}
	if err := feedbackRows.Close(); err != nil {
		return Snapshot{}, err
	}

	settings, err := s.ListSettings(ctx, true)
	if err != nil {
		return Snapshot{}, err
	}
	out.Settings = settings
	return out, nil
}
