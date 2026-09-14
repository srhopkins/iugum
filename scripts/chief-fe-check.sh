#!/usr/bin/env bash
# Fresh isolated native wiki; no personal config, model calls, or saved messages.
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"
TASK_DIR="$(mktemp -d "${TMPDIR:-/tmp}/iugum-chief-test.XXXXXX")"
SERVER_PID=""
cleanup() {
  if [ -n "$SERVER_PID" ]; then kill "$SERVER_PID" 2>/dev/null || true; wait "$SERVER_PID" 2>/dev/null || true; fi
  rm -rf "$TASK_DIR"
}
trap cleanup EXIT
PORT="${CHIEF_TEST_PORT:-3852}"
WIKI_PORT="${CHIEF_TEST_WIKI_PORT:-3052}"
mkdir -p "$TASK_DIR/wiki" "$TASK_DIR/empty"
cp plugs/editor-width/EditorWidth.md "$TASK_DIR/wiki/EditorWidth.md"
cp plugs/theme-menu/ThemeMenu.md "$TASK_DIR/wiki/ThemeMenu.md"
printf 'Keyboard regression fixture.\n' > "$TASK_DIR/wiki/index.md"
printf 'Search navigation fixture.\n' > "$TASK_DIR/wiki/SearchTarget.md"
cat > "$TASK_DIR/agent.yaml" <<CONFIG
name: chief-regression-test
runtime: native
listen: 127.0.0.1:$PORT
data_dir: data
wiki_dir: wiki
wiki_port: $WIKI_PORT
sources:
  - platform: claude
    root: $TASK_DIR/empty
CONFIG
CGO_ENABLED=0 go build -o "$TASK_DIR/iugum" .
"$TASK_DIR/iugum" agent run --config "$TASK_DIR/agent.yaml" > "$TASK_DIR/server.log" 2>&1 &
SERVER_PID=$!
export CHIEF_UI_TEST_URL="http://127.0.0.1:$PORT"
if ! node --input-type=module - <<'JS'
const start=Date.now();
while(Date.now()-start<60000){
 try{const r=await fetch(process.env.CHIEF_UI_TEST_URL+'/api/status');const s=await r.json();if(s.name==='chief-regression-test')process.exit(0);}catch{}
 await new Promise(r=>setTimeout(r,250));
}
process.exit(1);
JS
then cat "$TASK_DIR/server.log"; exit 1; fi
node --test plugs/chief/chief.test.mjs
node --test plugs/chief/styles.test.mjs
node plugs/chief/keyboard.e2e.mjs
node plugs/chief/search.e2e.mjs
node plugs/chief/shared-ui.e2e.mjs
node plugs/chief/accessibility.e2e.mjs
node plugs/chief/sticky.e2e.mjs
node plugs/chief/width.e2e.mjs
node plugs/chief/theme.e2e.mjs
node plugs/chief/agents.e2e.mjs
node plugs/chief/pending.e2e.mjs
node plugs/chief/conversations.e2e.mjs
if [ "${IUGUM_UI_UPDATE_BASELINES:-0}" = "1" ]; then
  silverbullet/node_modules/.bin/playwright test -c plugs/chief/visual.config.mjs --update-snapshots=all
else
  silverbullet/node_modules/.bin/playwright test -c plugs/chief/visual.config.mjs --update-snapshots=none
fi
if [ "${1:-}" = "--prove-regression" ]; then
  if CHIEF_KEYBOARD_NEGATIVE_CONTROL=1 node plugs/chief/keyboard.e2e.mjs > "$TASK_DIR/negative.log" 2>&1; then
    echo 'ERROR: removing the fix did not fail the regression test.' >&2
    exit 1
  fi
  if ! rg -q '4 !== 3' "$TASK_DIR/negative.log"; then
    cat "$TASK_DIR/negative.log"
    echo 'ERROR: negative control failed for an unexpected reason.' >&2
    exit 1
  fi
  echo 'Regression proven: without the fix, ArrowLeft leaves the caret at 4 instead of 3.'
fi

# Verify the independent wiki command with search only and no agent home.
kill "$SERVER_PID"; wait "$SERVER_PID" || true
SERVER_PID=""
printf 'search: true\nchat: false\ndata_dir: addon-data\n' > "$TASK_DIR/addons.yaml"
"$TASK_DIR/iugum" wiki --port "$PORT" --addons "$TASK_DIR/addons.yaml" "$TASK_DIR/wiki" > "$TASK_DIR/addons.log" 2>&1 &
SERVER_PID=$!
node --input-type=module - <<'JS'
const start=Date.now();
while(Date.now()-start<60000){
 try{const r=await fetch(process.env.CHIEF_UI_TEST_URL+'/api/status');const s=await r.json();if(s.name==='Wiki')process.exit(0);}catch{}
 await new Promise(r=>setTimeout(r,250));
}
process.exit(1);
JS
node plugs/chief/addons.e2e.mjs
kill "$SERVER_PID"; wait "$SERVER_PID" || true
SERVER_PID=""
printf 'search: false\nchat: true\ndata_dir: addon-data\n' > "$TASK_DIR/addons.yaml"
"$TASK_DIR/iugum" wiki --port "$PORT" --addons "$TASK_DIR/addons.yaml" "$TASK_DIR/wiki" > "$TASK_DIR/addons.log" 2>&1 &
SERVER_PID=$!
node --input-type=module - <<'JS'
const start=Date.now();
while(Date.now()-start<60000){
 try{const r=await fetch(process.env.CHIEF_UI_TEST_URL+'/api/agents');if(r.ok)process.exit(0);}catch{}
 await new Promise(r=>setTimeout(r,250));
}
process.exit(1);
JS
node plugs/chief/chat-only.e2e.mjs
