#!/bin/sh
set -eu

DATA_DIR=${DATA_DIR:-/opt/fyderhythm/sync-server/data}
BACKUP_DIR=${BACKUP_DIR:-/opt/fyderhythm/backups}
STAMP=$(date -u +%Y%m%dT%H%M%SZ)
mkdir -p "$BACKUP_DIR"

python3 - "$DATA_DIR/fyderhythm-sync.db" "$BACKUP_DIR/fyderhythm-sync-$STAMP.db" <<'PY'
import sqlite3
import sys

source, destination = sys.argv[1:]
with sqlite3.connect(source, timeout=30) as src:
    with sqlite3.connect(destination) as dst:
        src.backup(dst)
PY

chmod 600 "$BACKUP_DIR/fyderhythm-sync-$STAMP.db"
find "$BACKUP_DIR" -maxdepth 1 -type f -name 'fyderhythm-sync-*.db' -mtime +14 -delete
