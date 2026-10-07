#!/bin/bash
# KDE base only. The Chromium-only base starts its browser from CHROME_CLI.
# The KDE desktop has no such hook, so this starts Google Chrome (Playwright
# Chromium on arm64) with the same flags. Playwriter attaches on port 9222.
# The profile lives in /config/.chrome-profile, so it persists.
set -euo pipefail
for _ in $(seq 1 60); do
  bash -c 'echo >/dev/tcp/127.0.0.1/8080' 2>/dev/null && break
  sleep 1
done
rm -f /config/.chrome-profile/Singleton{Lock,Cookie,Socket}
bin=/usr/bin/google-chrome-stable
[ -x "$bin" ] || bin="$(find /opt/ms-playwright -type f -name chrome -path '*chrome-linux*' | head -1)"
[ -n "$bin" ] || { echo 'no browser found' >&2; exit 1; }
exec "$bin" --no-first-run --no-sandbox --password-store=basic \
  --ozone-platform=wayland --start-maximized --disable-dev-shm-usage \
  --remote-debugging-port=9222 --remote-debugging-address=127.0.0.1 \
  --user-data-dir=/config/.chrome-profile --force-device-scale-factor=1.25 \
  "http://127.0.0.1:8080/?folder=/config/workspace"
