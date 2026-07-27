#!/usr/bin/env bash
set -Eeuo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
RIME_DIR="${XDG_DATA_HOME:-$HOME/.local/share}/fcitx5/rime"
BIN_DIR="${HOME}/.local/bin"
SYSTEMD_DIR="${XDG_CONFIG_HOME:-$HOME/.config}/systemd/user"

sudo apt-get update
sudo apt-get install -y \
  fcitx5 fcitx5-rime fcitx5-config-qt fcitx5-frontend-gtk3 \
  fcitx5-frontend-gtk4 fcitx5-frontend-qt5 im-config librime-bin git rsync

mkdir -p "$BIN_DIR" "$RIME_DIR" "$SYSTEMD_DIR"
CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o "$BIN_DIR/fyderhythm-sync" "$ROOT_DIR/cmd/fyderhythm-sync"
install -m 0644 "$ROOT_DIR/packaging/systemd/fyderhythm-sync.service" "$SYSTEMD_DIR/fyderhythm-sync.service"
install -m 0644 "$ROOT_DIR/packaging/systemd/fyderhythm-sync.timer" "$SYSTEMD_DIR/fyderhythm-sync.timer"
sed -i "s#/usr/bin/fyderhythm-sync#$BIN_DIR/fyderhythm-sync#" "$SYSTEMD_DIR/fyderhythm-sync.service"

TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT
git clone --depth 1 https://github.com/frankichen/rime-wubi86-jidian.git "$TMP_DIR/rime-wubi86-jidian"
rsync -a --exclude='.git' "$TMP_DIR/rime-wubi86-jidian/" "$RIME_DIR/"

if command -v im-config >/dev/null 2>&1; then
  im-config -n fcitx5 || true
fi

systemctl --user daemon-reload
systemctl --user enable fyderhythm-sync.timer

echo
echo "安装完成。下一步："
echo "  1. 注销并重新登录 Ubuntu"
echo "  2. 运行 fcitx5-configtool，把 Rime 添加到输入法列表"
echo "  3. 运行 $BIN_DIR/fyderhythm-sync init --mode download-only"
echo "  4. 运行 $BIN_DIR/fyderhythm-sync test"
echo "  5. 运行 $BIN_DIR/fyderhythm-sync sync --force"
echo "  6. systemctl --user start fyderhythm-sync.timer"
