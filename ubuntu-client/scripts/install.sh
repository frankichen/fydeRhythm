#!/usr/bin/env bash
set -Eeuo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
RIME_DIR="${XDG_DATA_HOME:-$HOME/.local/share}/fcitx5/rime"
BIN_DIR="${HOME}/.local/bin"
SYSTEMD_DIR="${XDG_CONFIG_HOME:-$HOME/.config}/systemd/user"

sudo apt-get update
sudo apt-get install -y \
  fcitx5 fcitx5-rime fcitx5-config-qt fcitx5-frontend-gtk3 \
  fcitx5-frontend-gtk4 fcitx5-frontend-qt5 im-config librime-bin \
  git rsync zenity cmake g++ extra-cmake-modules libfcitx5core-dev

mkdir -p "$BIN_DIR" "$RIME_DIR" "$SYSTEMD_DIR"
CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o "$BIN_DIR/fyderhythm-sync" "$ROOT_DIR/cmd/fyderhythm-sync"
cmake -S "$ROOT_DIR/fcitx-addon" -B "$ROOT_DIR/dist/fcitx-addon-build" \
  -DCMAKE_BUILD_TYPE=Release -DCMAKE_INSTALL_PREFIX=/usr
cmake --build "$ROOT_DIR/dist/fcitx-addon-build" --parallel
sudo cmake --install "$ROOT_DIR/dist/fcitx-addon-build"
install -m 0755 "$ROOT_DIR/packaging/bin/fyderhythm-ai-settings" "$BIN_DIR/fyderhythm-ai-settings"
install -m 0644 "$ROOT_DIR/packaging/systemd/fyderhythm-sync.service" "$SYSTEMD_DIR/fyderhythm-sync.service"
install -m 0644 "$ROOT_DIR/packaging/systemd/fyderhythm-sync.timer" "$SYSTEMD_DIR/fyderhythm-sync.timer"
install -m 0644 "$ROOT_DIR/packaging/systemd/fyderhythm-ai.service" "$SYSTEMD_DIR/fyderhythm-ai.service"
sed -i "s#/usr/bin/fyderhythm-sync#$BIN_DIR/fyderhythm-sync#" "$SYSTEMD_DIR/fyderhythm-sync.service" "$SYSTEMD_DIR/fyderhythm-ai.service"
mkdir -p "$HOME/.local/share/applications"
install -m 0644 "$ROOT_DIR/packaging/applications/fyderhythm-ai-settings.desktop" "$HOME/.local/share/applications/fyderhythm-ai-settings.desktop"
sed -i "s#Exec=fyderhythm-ai-settings#Exec=$BIN_DIR/fyderhythm-ai-settings#" "$HOME/.local/share/applications/fyderhythm-ai-settings.desktop"

TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT
git clone --depth 1 https://github.com/frankichen/rime-wubi86-jidian.git "$TMP_DIR/rime-wubi86-jidian"
rsync -a --exclude='.git' "$TMP_DIR/rime-wubi86-jidian/" "$RIME_DIR/"

if command -v im-config >/dev/null 2>&1; then
  im-config -n fcitx5 || true
fi
systemctl --user daemon-reload
systemctl --user enable fyderhythm-sync.timer

cat <<'MESSAGE'
安装完成。

1. 注销并重新登录 Ubuntu。
2. 在 fcitx5-configtool 中添加 Rime。
3. 先配置词库同步：fyderhythm-sync init --mode download-only
4. 从应用菜单打开“fydeRhythm AI 设置”，输入 DeepSeek Key 和代理。
5. Alt+R 纠错；Alt+Enter 续写。
MESSAGE
