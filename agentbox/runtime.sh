#!/usr/bin/env bash
# Desktop integration, migrated from homelab/agentbox. One runtime file supplies
# the init hook and command shims; all user state lives in /config.
set -euo pipefail
command_name="$(basename "$0")"
export PULSE_SERVER="${PULSE_SERVER:-unix:/defaults/native}"
# Reuse this integration file for the optional image installer and service.
if [ "${1:-}" = install-t3 ]; then command_name=agentbox-t3-install; shift; fi
if [ "${1:-}" = serve-t3 ]; then command_name=agentbox-t3-server; shift; fi
case "$command_name" in
  69-t3.sh)
    # Existing desktops opt in without recreation; the marker lives in their volume.
    [ -f /config/.t3/enabled ] || exit 0
    # Refresh the older Claude bundled in existing images, without downgrading newer installs.
    if claude --version 2>/dev/null | grep -q '^2.1.248 '; then
      npm install -g @anthropic-ai/claude-code@2.1.302
    fi
    version="${T3_VERSION:-0.1.0}"
    if ! command -v t3 >/dev/null || [ "$(t3 --version)" != "t3 v$version" ]; then
      T3_VERSION="$version" bash "$0" install-t3
    fi
    mkdir -p /custom-services.d /usr/local/lib/agentbox
    install -m 755 "$0" /usr/local/lib/agentbox/t3-runtime.sh
    printf '#!/bin/sh\nexec s6-setuidgid abc /usr/local/lib/agentbox/t3-runtime.sh serve-t3\n' > /custom-services.d/t3-server
    chmod 755 /custom-services.d/t3-server
    # s6 registers custom services before init hooks run. On a first live install
    # or recreation, use tmux; a subsequent restart picks up the saved service.
    if [ ! -d /run/service/custom-svc-t3-server ]; then
      if ! command -v tmux >/dev/null; then
        apt-get update && apt-get install -y --no-install-recommends tmux
      fi
      if ! su abc -s /bin/bash -c 'tmux has-session -t t3-server 2>/dev/null'; then
        su abc -s /bin/bash -c 'tmux new-session -d -s t3-server /usr/local/lib/agentbox/t3-runtime.sh serve-t3'
      fi
    fi
    ;;
  agentbox-t3-install)
    version="${T3_VERSION:-0.1.0}"
    case "$(uname -m)" in x86_64) arch=x64;; aarch64|arm64) arch=arm64;; *) echo 'Unsupported T3 architecture' >&2; exit 1;; esac
    archive="t3-${version}-linux-${arch}.tar.gz"
    base="https://github.com/pingdotgg/t3code/releases/download/v${version}"
    stage="$(mktemp -d)"
    trap 'rm -rf "$stage"' EXIT
    curl -fsSL --retry 3 "$base/$archive" -o "$stage/$archive"
    curl -fsSL --retry 3 "$base/SHA256SUMS" -o "$stage/SHA256SUMS"
    (cd "$stage"; grep "  $archive$" SHA256SUMS > checksum; test -s checksum; sha256sum -c checksum)
    tar -xzf "$stage/$archive" -C "$stage"
    install -d /opt/t3 /usr/local/bin
    # Keep the native terminal modules and other bundled assets beside the binary.
    release="$(mktemp -d /opt/t3/release-${version}.XXXXXX)"
    cp -a "$stage/t3-${version}-linux-${arch}/." "$release/"
    chmod 755 "$release" "$release/t3"
    ln -sfn "$release/t3" /usr/local/bin/t3
    t3 --version
    ;;
  agentbox-t3-server)
    # Pairing remains enabled. Logs contain one-time tokens, so keep them private.
    umask 077
    export HOME=/config T3CODE_HOME=/config/.t3
    mkdir -p "$T3CODE_HOME"
    cd /config
    exec /usr/local/bin/t3 serve --host 0.0.0.0 --port "${T3_PORT:-3773}" >> "$T3CODE_HOME/server.log" 2>&1
    ;;
  bd) exec /usr/local/bin/iugum beads "$@" ;;
  playwriter-mcp)
    export PLAYWRITER_DIRECT="${PLAYWRITER_DIRECT:-1}"
    exec /usr/local/bin/playwriter "$@"
    ;;
  say)
    # Local speech works offline. TTS_URL opts into a compatible speech API.
    [ "$#" -gt 0 ] && text="$*" || text="$(cat)"
    [ -n "$text" ] || { echo 'usage: say TEXT (or text on stdin)' >&2; exit 2; }
    speech_file="$(mktemp --suffix=.wav)"
    trap 'rm -f "$speech_file"' EXIT
    if [ -n "${TTS_URL:-}" ]; then
      body="$(jq -cn --arg input "$text" --arg voice "${TTS_VOICE:-af_nova}" \
        --arg model "${TTS_MODEL:-kokoro}" --argjson speed "${TTS_SPEED:-1}" \
        '{input:$input,voice:$voice,model:$model,speed:$speed,response_format:"wav"}')"
      curl -fsS --max-time 120 "${TTS_URL%/}/v1/audio/speech" \
        -H 'Content-Type: application/json' --data "$body" -o "$speech_file"
    else
      espeak-ng -v "${SPEECH_VOICE:-en-us}" -s "${SPEECH_RATE:-175}" -w "$speech_file" -- "$text"
    fi
    paplay "$speech_file"
    ;;
  agentbox-record)
    [ "$#" -eq 1 ] || { echo 'usage: agentbox-record OUTPUT.mkv (Ctrl-C stops cleanly)' >&2; exit 2; }
    socket="${PIXELFLUX_RECORDING_SOCKET:-${SELKIES_RECORDING_SOCKET:-/config/.selkies-recording}}"
    [ -S "$socket" ] || { echo "Recording socket absent: $socket. Open the Selkies stream first." >&2; exit 1; }
    mkdir -p "$(dirname "$1")"
    sink="$(pactl get-default-sink)"
    # The Selkies socket is raw H.264; copy video and record desktop audio,
    # including say/TTS. Matroska keeps recordings usable after interruption.
    exec ffmpeg -n -thread_queue_size 512 -use_wallclock_as_timestamps 1 \
      -analyzeduration 1000000 -probesize 1048576 -f h264 -i "unix:$socket" \
      -thread_queue_size 512 -f pulse -i "$sink.monitor" \
      -c:v copy -c:a aac "$1"
    ;;
  agentbox-screenshot)
    [ "$#" -ge 1 ] && [ "$#" -le 2 ] || { echo 'usage: agentbox-screenshot OUTPUT.png [URL_SUBSTRING]' >&2; exit 2; }
    # Capture the visible Chrome page, not a second headless browser.
    NODE_PATH="$(npm root -g)" node - "$@" <<'JS'
const { chromium } = require('playwright');
const fs = require('node:fs');
const path = require('node:path');
(async () => {
  const browser = await chromium.connectOverCDP('http://127.0.0.1:9222');
  try {
    const pages = browser.contexts().flatMap(c => c.pages());
    const match = process.argv[3];
    const page = match ? pages.find(p => p.url().includes(match)) : pages.at(-1);
    if (!page) throw new Error('No matching Chrome tab');
    const output = process.argv[2];
    if (fs.existsSync(output)) throw new Error('Output already exists: ' + output);
    fs.mkdirSync(path.dirname(output), {recursive: true});
    await page.screenshot({path: output});
    console.log(output);
  } finally { await browser.close(); }
})().catch(e => { console.error(e.message); process.exitCode = 1; });
JS
    ;;
  *)
    [ "${1:-}" = init ] || { echo 'internal command: runtime.sh init' >&2; exit 2; }
    config_root=/config
    mkdir -p "$config_root"/{workspace,recordings,.local/bin,.config/code-server,.config/opencode,.codex,.cursor,.claude/skills,.agents/skills/playwriter,.local/share/code-server/User}
    # Shared profiles are unsupported: each container must have its own /config.
    rm -f "$config_root"/.chrome-profile/Singleton{Lock,Cookie,Socket}

    # Seed defaults once, preserving edits and account settings on recreation.
    seed() { if [ ! -e "$1" ]; then cat > "$1"; else cat >/dev/null; fi; }
    seed "$config_root/.config/code-server/config.yaml" <<'EOF'
bind-addr: 0.0.0.0:8080
auth: none
cert: false
EOF
    seed "$config_root/.local/share/code-server/User/settings.json" <<'EOF'
{"editor.fontSize":16,"terminal.integrated.fontSize":16,"workbench.startupEditor":"none","terminal.integrated.defaultProfile.linux":"bash","window.autoDetectColorScheme":true}
EOF
    # Merge MCP entries only when absent; invalid JSON stops init rather than
    # replacing the user's settings. jq writes a temp file before rename.
    merge_json() {
      local file="$1" filter="$2" staged
      [ -e "$file" ] || printf '{}\n' > "$file"
      staged="$(mktemp "$file.XXXXXX")"
      if jq "$filter" "$file" > "$staged"; then mv "$staged" "$file"; else rm -f "$staged"; return 1; fi
    }
    merge_json "$config_root/.claude.json" '.mcpServers.playwriter //= {type:"stdio",command:"/usr/local/bin/playwriter-mcp",args:[]}'
    merge_json "$config_root/.cursor/mcp.json" '.mcpServers.playwriter //= {command:"/usr/local/bin/playwriter-mcp",args:[]}'
    merge_json "$config_root/.local/share/code-server/User/mcp.json" '.servers.playwriter //= {type:"stdio",command:"/usr/local/bin/playwriter-mcp",args:[]}'
    merge_json "$config_root/.config/opencode/opencode.json" '.mcp.playwriter //= {type:"local",command:["/usr/local/bin/playwriter-mcp"],enabled:true}'
    seed "$config_root/.codex/config.toml" <<'EOF'
[mcp_servers.playwriter]
command = "/usr/local/bin/playwriter-mcp"
EOF
    if [ ! -s "$config_root/.agents/skills/playwriter/SKILL.md" ]; then
      playwriter skill > "$config_root/.agents/skills/playwriter/SKILL.md"
    fi
    ln -sfn "$config_root/.agents/skills/playwriter" "$config_root/.claude/skills/playwriter"
    seed "$config_root/AGENTS.md" <<'EOF'
For browser control, code walkthroughs, screenshots, recordings, or spoken
narration, read /usr/local/share/agentbox/WORKFLOW.md before acting.
EOF
    seed "$config_root/CLAUDE.md" <<'EOF'
For browser control, code walkthroughs, screenshots, recordings, or spoken
narration, read /usr/local/share/agentbox/WORKFLOW.md before acting.
EOF
    seed "$config_root/.profile" <<'EOF'
if [ -n "${BASH_VERSION:-}" ] && [ -f "$HOME/.bashrc" ]; then
    . "$HOME/.bashrc"
fi
EOF
    touch "$config_root/.bashrc"
    if ! grep -q '# iugum agentbox' "$config_root/.bashrc"; then
      cat >> "$config_root/.bashrc" <<'EOF'
# iugum agentbox
export PATH="/usr/local/bin:/config/.local/bin:$PATH"
export PULSE_SERVER=unix:/defaults/native
EOF
    fi
    mkdir -p "$config_root/playwright/node_modules/@playwright"
    ln -sfn /usr/local/lib/node_modules/playwright "$config_root/playwright/node_modules/playwright"
    ln -sfn /usr/local/lib/node_modules/@playwright/test "$config_root/playwright/node_modules/@playwright/test"
    # Touch only config-owned files; avoid recursively chowning project mounts.
    find "$config_root" -maxdepth 1 -type f -exec chown abc:abc {} +
    for dir in .local .config .codex .cursor .claude .agents playwright; do
      chown -R abc:abc "$config_root/$dir"
    done
    chown abc:abc "$config_root" "$config_root/workspace" "$config_root/recordings"
    echo '[agentbox] editor, agent connections, recording and speech ready'
    ;;
esac
