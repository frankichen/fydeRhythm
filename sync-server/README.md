# fydeRhythm Personal Sync Server

A single-user synchronization service for fydeRhythm personal dictionaries and typing habits.

It keeps the IME fast and local while synchronizing only compact learning events:

- custom lexicon entries and shortcuts;
- candidate selection counts;
- accepted correction pairs;
- AI suggestion acceptance/rejection counters;
- per-device settings;
- incremental changes for multiple devices.

The server does **not** require complete sentences or full input-box contents. The client should send only the selected word, its input code, correction pairs, and anonymous feedback counters.

## Architecture

- Go HTTP API
- SQLite in WAL mode
- static Bearer token authentication
- idempotent changes identified by `change_id`
- monotonic synchronization cursor (`sync_log.seq`)
- additive counters for usage events
- last-write-wins for lexicon/settings using `client_updated_at`, with `device_id` as a deterministic tie-breaker
- tombstones for deletions, preventing old devices from restoring deleted words

## Quick deployment with Docker

```bash
cd sync-server
cp .env.example .env
openssl rand -hex 32
```

Put the generated token in `.env`, then start the service:

```bash
docker compose up -d --build
docker compose logs -f
```

The service binds only to localhost by default:

```text
http://127.0.0.1:18080
```

Health check:

```bash
curl http://127.0.0.1:18080/healthz
```

Authenticated request:

```bash
export API_TOKEN='your-token'
curl -H "Authorization: Bearer $API_TOKEN" \
  http://127.0.0.1:18080/api/v1/stats
```

For remote Chromebook access, place Caddy or Nginx in front of port `18080` and use HTTPS. Examples are included in `Caddyfile.example` and `nginx.conf.example`.

## Configuration

| Variable | Required | Default | Description |
|---|---:|---|---|
| `API_TOKEN` | yes | — | Bearer token, at least 24 characters |
| `LISTEN_ADDR` | no | `:8080` | HTTP listen address inside the container |
| `DATABASE_PATH` | no | `./data/fyderhythm-sync.db` | SQLite database path |
| `CORS_ORIGINS` | no | Chrome extension and localhost patterns | Comma-separated origin patterns |
| `MAX_BODY_BYTES` | no | `2097152` | Maximum request body size |

Generate a token:

```bash
make token
```

## Synchronization protocol

### Initial device bootstrap

1. Request `GET /api/v1/sync/snapshot`.
2. Store the returned `cursor` locally.
3. Apply the current lexicon, settings, usage counters, corrections and AI feedback.
4. Continue with incremental pulls from that cursor.

### Incremental push

`POST /api/v1/sync/push`

```json
{
  "device_id": "chromebook-redrix",
  "device": {
    "name": "Chromebook",
    "platform": "ChromeOS",
    "app_version": "3.0.1.3"
  },
  "changes": [
    {
      "change_id": "b5a56c9b-0ef0-4e59-bdb2-8c450ee74d91",
      "entity": "candidate_usage",
      "operation": "increment",
      "client_updated_at": 1785060000000,
      "payload": {
        "code": "abcd",
        "candidate": "测试",
        "delta": 1,
        "last_used_at": 1785060000000
      }
    }
  ]
}
```

A retry with the same `change_id` is ignored, so network retries are safe.

### Incremental pull

```bash
curl -H "Authorization: Bearer $API_TOKEN" \
  'https://sync.example.com/api/v1/sync/pull?since=123&limit=500'
```

Persist the returned cursor only after all returned changes have been applied locally. If `has_more` is true, pull again using the returned cursor.

## Change entities

### `lexicon`

Operations: `upsert`, `delete`

```json
{
  "id": "stable-client-generated-id",
  "phrase": "太阳新城",
  "code": "",
  "shortcut": "tyxc",
  "weight": 100,
  "category": "address",
  "notes": ""
}
```

### `candidate_usage`

Operation: `increment`

```json
{
  "code": "abcd",
  "candidate": "测试",
  "delta": 1,
  "last_used_at": 1785060000000
}
```

### `correction`

Operation: `increment`

```json
{
  "original": "在确认",
  "corrected": "再确认",
  "delta": 1,
  "last_used_at": 1785060000000
}
```

### `ai_feedback`

Operation: `increment`

Only a local hash should be sent in `context_hash`; do not send the original sentence.

```json
{
  "feature": "prediction",
  "context_hash": "sha256-prefix-or-empty",
  "accepted_delta": 1,
  "rejected_delta": 0,
  "last_used_at": 1785060000000
}
```

### `setting`

Operations: `upsert`, `delete`

```json
{
  "key": "prediction.delay_ms",
  "value": 1000
}
```

## Personal lexicon management

List:

```bash
curl -H "Authorization: Bearer $API_TOKEN" \
  'http://127.0.0.1:18080/api/v1/lexicon?q=OpenCode'
```

Create:

```bash
curl -X POST \
  -H "Authorization: Bearer $API_TOKEN" \
  -H "X-Device-ID: admin" \
  -H "Content-Type: application/json" \
  -d '{"phrase":"OpenCode","shortcut":"opc","weight":100,"category":"tech"}' \
  http://127.0.0.1:18080/api/v1/lexicon
```

Export:

```bash
curl -H "Authorization: Bearer $API_TOKEN" \
  'http://127.0.0.1:18080/api/v1/lexicon/export?format=csv' \
  -o fyderhythm-lexicon.csv
```

Import:

```bash
curl -X POST \
  -H "Authorization: Bearer $API_TOKEN" \
  -H "X-Device-ID: admin" \
  -H "Content-Type: text/csv" \
  --data-binary @fyderhythm-lexicon.csv \
  'http://127.0.0.1:18080/api/v1/lexicon/import?format=csv&mode=merge'
```

`mode=replace` creates tombstones for the current lexicon before importing the new file. It is intentionally limited to 500 total changes per request.

## Backup

The persistent data is a single SQLite file:

```text
sync-server/data/fyderhythm-sync.db
```

The minimal runtime image does not include the SQLite CLI. Stop the container briefly before copying the database, so the main file and WAL state are consistent:

```bash
docker compose stop
cp data/fyderhythm-sync.db data/fyderhythm-sync-$(date +%F).db
docker compose start
```

The lexicon can always be exported through the HTTP API without stopping the service.

## Development

```bash
go test ./...
go run ./cmd/server
```

Run the smoke test after deployment:

```bash
export API_TOKEN='your-token'
./scripts/smoke-test.sh
```

API definitions are documented in `openapi.yaml`.
