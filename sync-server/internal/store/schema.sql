PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS devices (
  device_id TEXT PRIMARY KEY,
  name TEXT NOT NULL DEFAULT '',
  platform TEXT NOT NULL DEFAULT '',
  app_version TEXT NOT NULL DEFAULT '',
  last_seen_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS lexicon (
  id TEXT PRIMARY KEY,
  phrase TEXT NOT NULL,
  code TEXT NOT NULL DEFAULT '',
  shortcut TEXT NOT NULL DEFAULT '',
  weight INTEGER NOT NULL DEFAULT 0,
  category TEXT NOT NULL DEFAULT '',
  notes TEXT NOT NULL DEFAULT '',
  deleted INTEGER NOT NULL DEFAULT 0,
  client_updated_at INTEGER NOT NULL,
  source_device_id TEXT NOT NULL,
  server_updated_at INTEGER NOT NULL,
  version INTEGER NOT NULL DEFAULT 1
);
CREATE INDEX IF NOT EXISTS idx_lexicon_phrase ON lexicon(phrase);
CREATE INDEX IF NOT EXISTS idx_lexicon_code ON lexicon(code);
CREATE INDEX IF NOT EXISTS idx_lexicon_category ON lexicon(category);

CREATE TABLE IF NOT EXISTS candidate_usage (
  code TEXT NOT NULL,
  candidate TEXT NOT NULL,
  count INTEGER NOT NULL DEFAULT 0,
  last_used_at INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY(code, candidate)
);

CREATE TABLE IF NOT EXISTS corrections (
  original TEXT NOT NULL,
  corrected TEXT NOT NULL,
  count INTEGER NOT NULL DEFAULT 0,
  last_used_at INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY(original, corrected)
);

CREATE TABLE IF NOT EXISTS ai_feedback (
  feature TEXT NOT NULL,
  context_hash TEXT NOT NULL DEFAULT '',
  accepted_count INTEGER NOT NULL DEFAULT 0,
  rejected_count INTEGER NOT NULL DEFAULT 0,
  last_used_at INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY(feature, context_hash)
);

CREATE TABLE IF NOT EXISTS settings (
  key TEXT PRIMARY KEY,
  value_json TEXT NOT NULL DEFAULT 'null',
  deleted INTEGER NOT NULL DEFAULT 0,
  client_updated_at INTEGER NOT NULL,
  source_device_id TEXT NOT NULL,
  server_updated_at INTEGER NOT NULL,
  version INTEGER NOT NULL DEFAULT 1
);

CREATE TABLE IF NOT EXISTS applied_changes (
  change_id TEXT PRIMARY KEY,
  device_id TEXT NOT NULL,
  applied_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS sync_log (
  seq INTEGER PRIMARY KEY AUTOINCREMENT,
  change_id TEXT NOT NULL,
  entity TEXT NOT NULL,
  operation TEXT NOT NULL,
  entity_key TEXT NOT NULL,
  payload_json TEXT NOT NULL,
  source_device_id TEXT NOT NULL,
  created_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_sync_log_seq ON sync_log(seq);
CREATE INDEX IF NOT EXISTS idx_sync_log_entity ON sync_log(entity, entity_key);
