#!/usr/bin/env bash
set -Eeuo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
VERSION="$("$ROOT_DIR/dist/fyderhythm-sync" version)"
ARCH="$(dpkg --print-architecture 2>/dev/null || echo amd64)"
PKG_ROOT="$ROOT_DIR/dist/deb-root"
OUT="$ROOT_DIR/dist/fyderhythm-sync_${VERSION}_${ARCH}.deb"

rm -rf "$PKG_ROOT"
mkdir -p \
  "$PKG_ROOT/DEBIAN" \
  "$PKG_ROOT/usr/bin" \
  "$PKG_ROOT/usr/lib/systemd/user" \
  "$PKG_ROOT/usr/share/applications" \
  "$PKG_ROOT/usr/share/doc/fyderhythm-sync"

DESTDIR="$PKG_ROOT" cmake --install "$ROOT_DIR/dist/fcitx-addon-build"
install -m 0755 "$ROOT_DIR/dist/fyderhythm-sync" "$PKG_ROOT/usr/bin/fyderhythm-sync"
install -m 0755 "$ROOT_DIR/packaging/bin/fyderhythm-ai-settings" "$PKG_ROOT/usr/bin/fyderhythm-ai-settings"
install -m 0644 "$ROOT_DIR/packaging/systemd/fyderhythm-sync.service" "$PKG_ROOT/usr/lib/systemd/user/fyderhythm-sync.service"
install -m 0644 "$ROOT_DIR/packaging/systemd/fyderhythm-sync.timer" "$PKG_ROOT/usr/lib/systemd/user/fyderhythm-sync.timer"
install -m 0644 "$ROOT_DIR/packaging/systemd/fyderhythm-ai.service" "$PKG_ROOT/usr/lib/systemd/user/fyderhythm-ai.service"
install -m 0644 "$ROOT_DIR/packaging/applications/fyderhythm-ai-settings.desktop" "$PKG_ROOT/usr/share/applications/fyderhythm-ai-settings.desktop"
install -m 0644 "$ROOT_DIR/README.md" "$PKG_ROOT/usr/share/doc/fyderhythm-sync/README.md"

cat > "$PKG_ROOT/DEBIAN/control" <<CONTROL
Package: fyderhythm-sync
Version: $VERSION
Section: utils
Priority: optional
Architecture: $ARCH
Maintainer: frankichen
Depends: fcitx5, fcitx5-rime, librime-bin, zenity, wl-clipboard, xclip
Description: fydeRhythm sync and DeepSeek AI for Fcitx5/Rime
 Local-first personal lexicon synchronization plus an opt-in DeepSeek
 correction and next-sentence addon for Ubuntu Fcitx5/Rime.
CONTROL

cat > "$PKG_ROOT/DEBIAN/postinst" <<'POSTINST'
#!/bin/sh
set -e
if command -v systemctl >/dev/null 2>&1; then
  systemctl daemon-reload >/dev/null 2>&1 || true
fi
exit 0
POSTINST
chmod 0755 "$PKG_ROOT/DEBIAN/postinst"

dpkg-deb --build --root-owner-group "$PKG_ROOT" "$OUT"
echo "$OUT"
