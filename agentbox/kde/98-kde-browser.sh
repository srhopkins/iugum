#!/bin/bash
# Plasma does not run autostart before the session is up. Wait for the Wayland
# socket, then start the browser as the desktop user.
s6-setuidgid abc env HOME=/config XDG_RUNTIME_DIR=/config/.XDG \
  WAYLAND_DISPLAY=wayland-0 DISPLAY=:1 bash -c '
    for _ in $(seq 1 90); do [ -S /config/.XDG/wayland-0 ] && break; sleep 1; done
    exec /usr/local/lib/agentbox/kde/agentbox-browser.sh
  ' >/config/.browser-launch.log 2>&1 &
