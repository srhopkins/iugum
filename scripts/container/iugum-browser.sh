#!/bin/sh
# Chromium on KasmVNC (its own X server + web client).
# Chrome/Edge on the Mac get seamless clipboard. Safari does not.
set -eu

BIND="${IUGUM_BROWSER_BIND:-0.0.0.0}"
PORT="${IUGUM_BROWSER_PORT:-6080}"
DISPLAY_NUM="${IUGUM_BROWSER_DISPLAY:-1}"
export DISPLAY=":${DISPLAY_NUM}"

while [ $# -gt 0 ]; do
  case "$1" in
    --bind) BIND="$2"; shift 2 ;;
    --port) PORT="$2"; shift 2 ;;
    *) echo "iugum-browser: unknown arg $1" >&2; exit 2 ;;
  esac
done

if ! command -v vncserver >/dev/null 2>&1; then
  echo "iugum-browser: vncserver (KasmVNC) is not installed" >&2
  exit 1
fi
if ! command -v chromium >/dev/null 2>&1 && ! command -v chromium-browser >/dev/null 2>&1; then
  echo "iugum-browser: chromium is not installed" >&2
  exit 1
fi

HOME="${HOME:-/home/iugum}"
VNC_DIR="${HOME}/.vnc"
XSTARTUP="${IUGUM_BROWSER_XSTARTUP:-/usr/local/lib/iugum/browser-xstartup}"
mkdir -p "$VNC_DIR"
chmod +x "$XSTARTUP" 2>/dev/null || true

if command -v kasmvncpasswd >/dev/null 2>&1; then
  # Unused: we pass -disableBasicAuth. File must exist. Password min 6 chars.
  printf '%s\n' 'iugum1' 'iugum1' | kasmvncpasswd -u iugum -wo >/dev/null
fi

if [ ! -f "$VNC_DIR/self.pem" ]; then
  openssl req -x509 -nodes -newkey rsa:2048 \
    -keyout "$VNC_DIR/self.pem" -out "$VNC_DIR/self.pem" \
    -days 3650 -subj "/CN=localhost" >/dev/null 2>&1 || true
fi

cat > "$VNC_DIR/kasmvnc.yaml" <<EOF
desktop:
  resolution:
    width: 1920
    height: 1080
  allow_resize: true
  pixel_depth: 24
network:
  protocol: http
  interface: ${BIND}
  websocket_port: ${PORT}
  use_ipv4: true
  use_ipv6: false
  ssl:
    pem_certificate: ${VNC_DIR}/self.pem
    pem_key: ${VNC_DIR}/self.pem
    require_ssl: false
command_line:
  prompt: false
data_loss_prevention:
  clipboard:
    delay_between_operations: none
    server_to_client:
      enabled: true
      size: unlimited
    client_to_server:
      enabled: true
      size: unlimited
EOF

cleanup() {
  vncserver -kill "$DISPLAY" >/dev/null 2>&1 || true
  rm -f "/tmp/.X${DISPLAY_NUM}-lock" "/tmp/.X11-unix/X${DISPLAY_NUM}"
}
trap cleanup TERM INT EXIT

vncserver -kill "$DISPLAY" >/dev/null 2>&1 || true
rm -f "/tmp/.X${DISPLAY_NUM}-lock" "/tmp/.X11-unix/X${DISPLAY_NUM}"

echo "iugum-browser: KasmVNC http://${BIND}:${PORT}/"
vncserver "$DISPLAY" \
  -geometry 1920x1080 \
  -depth 24 \
  -websocketPort "$PORT" \
  -disableBasicAuth \
  -fg \
  -xstartup "$XSTARTUP"
