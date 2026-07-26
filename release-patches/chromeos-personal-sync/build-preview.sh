#!/usr/bin/env bash
set -Eeuo pipefail

if [ "$#" -lt 2 ]; then
  echo "Usage: $0 <official-extension-root> <output-zip>" >&2
  exit 2
fi

SOURCE_DIR="$(cd "$1" && pwd)"
OUTPUT_ZIP="$(realpath -m "$2")"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ARCHIVE="$SCRIPT_DIR/runtime-patches.tar.gz"

if [ ! -f "$SOURCE_DIR/manifest.json" ] || [ ! -f "$SOURCE_DIR/background.js" ]; then
  echo "Source directory must contain manifest.json and background.js" >&2
  exit 1
fi
if [ ! -f "$ARCHIVE" ]; then
  echo "Missing runtime-patches.tar.gz" >&2
  exit 1
fi

TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT
ROOT="$TMP_DIR/fydeRhythm-ai-personal-sync-3.0.2.0"
PATCH_DIR="$TMP_DIR/runtime-patches"
mkdir -p "$PATCH_DIR"
tar -xzf "$ARCHIVE" -C "$PATCH_DIR"

for file in personal-sync.js personal-sync-options.js ai-reranker.js ai-options.js; do
  if [ ! -f "$PATCH_DIR/$file" ]; then
    echo "Runtime archive is missing $file" >&2
    exit 1
  fi
  node --check "$PATCH_DIR/$file"
done

cp -a "$SOURCE_DIR" "$ROOT"
if [ ! -f "$ROOT/background-original.js" ]; then
  mv "$ROOT/background.js" "$ROOT/background-original.js"
fi
cp "$PATCH_DIR/personal-sync.js" "$ROOT/personal-sync.js"
cp "$PATCH_DIR/ai-reranker.js" "$ROOT/ai-reranker.js"
cp "$PATCH_DIR/personal-sync-options.js" "$ROOT/personal-sync-options.js"
cp "$PATCH_DIR/ai-options.js" "$ROOT/ai-options.js"

cat > "$ROOT/background.js" <<'BOOTSTRAP'
/* FydeRhythm AI + Personal Sync bootstrap. */
importScripts('personal-sync.js');
importScripts('ai-reranker.js');
importScripts('background-original.js');
BOOTSTRAP

python3 - "$ROOT" <<'PY'
from pathlib import Path
import json
import sys

root = Path(sys.argv[1])
manifest_path = root / 'manifest.json'
manifest = json.loads(manifest_path.read_text())
manifest['version'] = '3.0.2.0'
manifest['version_name'] = '3.0.2 AI + Personal Sync Preview'
permissions = manifest.setdefault('permissions', [])
if 'alarms' not in permissions:
    permissions.append('alarms')
hosts = manifest.setdefault('host_permissions', [])
for host in ('https://api.deepseek.com/*', 'https://shulufa.555044.xyz/*'):
    if host not in hosts:
        hosts.append(host)
manifest['content_security_policy'] = {
    'extension_pages': "script-src 'self' 'wasm-unsafe-eval'; object-src 'self'; connect-src 'self' https://api.deepseek.com https://shulufa.555044.xyz;"
}
manifest_path.write_text(json.dumps(manifest, ensure_ascii=False, indent=2) + '\n')

options_path = root / 'options.html'
options = options_path.read_text()
for script in ('/ai-options.js', '/personal-sync-options.js'):
    tag = f'<script src="{script}"></script>'
    if tag not in options:
        options = options.replace('</body>', f'  {tag}\n</body>')
options_path.write_text(options)
PY

cat > "$ROOT/安装说明.txt" <<'INSTRUCTIONS'
FydeRhythm 3.0.2 AI + Personal Sync Preview

1. 打开 chrome://extensions 并开启开发者模式。
2. 选择“加载已解压的扩展程序”，选择本目录。
3. 在 ChromeOS 输入法设置中启用 FydeRhythm。
4. 打开扩展选项，在“个人词库与输入习惯同步”中填写 API Token。
5. 服务器保持 https://shulufa.555044.xyz，测试成功后立即同步。

个人词以“★”显示在普通候选末尾，Tab 接受；AI 候选建议用 Alt+Enter 接受。
INSTRUCTIONS

python3 -m json.tool "$ROOT/manifest.json" >/dev/null
mkdir -p "$(dirname "$OUTPUT_ZIP")"
rm -f "$OUTPUT_ZIP"
(
  cd "$TMP_DIR"
  zip -qr "$OUTPUT_ZIP" "$(basename "$ROOT")"
)
sha256sum "$OUTPUT_ZIP"
