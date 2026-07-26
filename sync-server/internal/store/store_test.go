package store

import (
	"context"
	"encoding/json"
	"testing"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func raw(t *testing.T, value any) json.RawMessage {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestPushPullLexiconAndIdempotency(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	change := Change{
		ChangeID:        "change-1",
		Entity:          "lexicon",
		Operation:       "upsert",
		ClientUpdatedAt: 1000,
		Payload: raw(t, LexiconEntry{
			ID: "entry-1", Phrase: "太阳新城", Code: "test", Weight: 100, Category: "address",
		}),
	}

	first, err := s.Push(ctx, PushRequest{DeviceID: "chromebook", Changes: []Change{change}})
	if err != nil {
		t.Fatalf("Push() error = %v", err)
	}
	if first.Applied != 1 || first.Duplicates != 0 || first.Cursor != 1 {
		t.Fatalf("unexpected first result: %+v", first)
	}

	second, err := s.Push(ctx, PushRequest{DeviceID: "chromebook", Changes: []Change{change}})
	if err != nil {
		t.Fatalf("second Push() error = %v", err)
	}
	if second.Applied != 0 || second.Duplicates != 1 || second.Cursor != 1 {
		t.Fatalf("unexpected duplicate result: %+v", second)
	}

	pulled, err := s.Pull(ctx, 0, 100)
	if err != nil {
		t.Fatalf("Pull() error = %v", err)
	}
	if len(pulled.Changes) != 1 || pulled.Cursor != 1 || pulled.HasMore {
		t.Fatalf("unexpected pull result: %+v", pulled)
	}

	items, err := s.ListLexicon(ctx, "太阳", "", false, 20, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Phrase != "太阳新城" || items[0].Weight != 100 {
		t.Fatalf("unexpected lexicon: %+v", items)
	}
}

func TestLexiconLastWriterWins(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	initial := Change{ChangeID: "c1", Entity: "lexicon", Operation: "upsert", ClientUpdatedAt: 2000, Payload: raw(t, LexiconEntry{ID: "e1", Phrase: "新版"})}
	if _, err := s.Push(ctx, PushRequest{DeviceID: "device-b", Changes: []Change{initial}}); err != nil {
		t.Fatal(err)
	}
	older := Change{ChangeID: "c2", Entity: "lexicon", Operation: "upsert", ClientUpdatedAt: 1000, Payload: raw(t, LexiconEntry{ID: "e1", Phrase: "旧版"})}
	result, err := s.Push(ctx, PushRequest{DeviceID: "device-a", Changes: []Change{older}})
	if err != nil {
		t.Fatal(err)
	}
	if result.Conflicts != 1 || result.Applied != 0 {
		t.Fatalf("unexpected conflict result: %+v", result)
	}
	items, err := s.ListLexicon(ctx, "", "", false, 20, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Phrase != "新版" {
		t.Fatalf("older write replaced current entry: %+v", items)
	}
}

func TestUsageDeltasAreAdditiveAndIdempotent(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	makeChange := func(id string, delta int64) Change {
		return Change{ChangeID: id, Entity: "candidate_usage", Operation: "increment", ClientUpdatedAt: 1000, Payload: raw(t, map[string]any{
			"code": "abcd", "candidate": "测试", "delta": delta, "last_used_at": int64(1000),
		})}
	}
	if _, err := s.Push(ctx, PushRequest{DeviceID: "d1", Changes: []Change{makeChange("u1", 2), makeChange("u2", 3)}}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Push(ctx, PushRequest{DeviceID: "d1", Changes: []Change{makeChange("u2", 3)}}); err != nil {
		t.Fatal(err)
	}
	stats, err := s.Stats(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(stats.TopCandidates) != 1 || stats.TopCandidates[0].Count != 5 {
		t.Fatalf("unexpected usage aggregate: %+v", stats.TopCandidates)
	}
}

func TestSnapshotCursorDoesNotReplayIncludedData(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	changes := []Change{
		{ChangeID: "snapshot-lexicon", Entity: "lexicon", Operation: "upsert", ClientUpdatedAt: 1000, Payload: raw(t, LexiconEntry{ID: "entry-1", Phrase: "个人词库"})},
		{ChangeID: "snapshot-usage", Entity: "candidate_usage", Operation: "increment", ClientUpdatedAt: 1000, Payload: raw(t, map[string]any{
			"code": "abcd", "candidate": "个人词库", "delta": int64(2), "last_used_at": int64(1000),
		})},
	}
	if _, err := s.Push(ctx, PushRequest{DeviceID: "snapshot-device", Changes: changes}); err != nil {
		t.Fatal(err)
	}

	snapshot, err := s.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Cursor != 2 || len(snapshot.Lexicon) != 1 || len(snapshot.CandidateUsage) != 1 {
		t.Fatalf("unexpected snapshot: %+v", snapshot)
	}
	if snapshot.CandidateUsage[0].Count != 2 {
		t.Fatalf("unexpected usage count: %+v", snapshot.CandidateUsage)
	}

	pulled, err := s.Pull(ctx, snapshot.Cursor, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(pulled.Changes) != 0 || pulled.Cursor != snapshot.Cursor {
		t.Fatalf("snapshot cursor replayed included data: %+v", pulled)
	}
}
