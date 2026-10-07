#!/bin/bash
# Agents drive this desktop. KDE otherwise asks on every X11 pointer or key event.
mkdir -p /config/.config
command -v kwriteconfig6 >/dev/null 2>&1 && HOME=/config XDG_CONFIG_HOME=/config/.config \
  kwriteconfig6 --file kwinrc --group Xwayland --key XwaylandEisNoPrompt true
chown abc:abc /config/.config/kwinrc 2>/dev/null || true
