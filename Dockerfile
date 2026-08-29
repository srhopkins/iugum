# syntax=docker/dockerfile:1.7
# ============================================================================
# iugum - one image, built from source, with the agent CLIs you choose.
#
# Build args:
#   WITH                comma list of: claude, codex, opencode, cursor,
#                       code-server, browser. "all" (default) installs every
#                       item including browser. "none" installs none.
#   CODE_SERVER         overlay on WITH: 1 adds code-server, 0 removes it,
#                       empty follows WITH.
#   BROWSER             overlay on WITH: 1 adds Chromium+KasmVNC, 0 removes
#                       it (opt-out from all), empty follows WITH.
#   KASMVNC_VERSION     KasmVNC deb when WITH has browser.
#   TTYD_VERSION        ttyd static binary (always installed; ~1.3 MB).
#   CGO_ENABLED         1 (default) links the embedded Dolt database (needs ICU).
#                       0 makes a static program with no C libraries.
#   GO_VERSION          Go toolchain tag for the builder stage.
#   CODE_SERVER_VERSION code-server release to install when CODE_SERVER=1 or WITH has it.
#   OPENCODE_VERSION    opencode-ai npm package version when WITH has opencode.
#   CLAUDE_VERSION      @anthropic-ai/claude-code npm version when WITH has claude.
#   CODEX_VERSION       @openai/codex npm version when WITH has codex.
#   CURSOR_AGENT_VERSION cursor-agent lab build (YYYY.MM.DD-hash) when WITH has cursor.
#
# Examples:
#   docker build -t iugum .
#   docker build --build-arg WITH=claude,code-server -t iugum:claude .
#   docker build --build-arg WITH=opencode --build-arg CODE_SERVER=1 --build-arg BROWSER=1 -t iugum .
#   docker build --build-arg WITH=all --build-arg BROWSER=0 -t iugum:nobrowser .
#   docker build --build-arg WITH=none --build-arg CGO_ENABLED=0 -t iugum:slim .
#
# Works with docker and podman. Full docs: docs/container.md
# ============================================================================

ARG GO_VERSION=1.26.5
ARG CODE_SERVER_VERSION=4.134.0
ARG OPENCODE_VERSION=1.18.23
ARG CLAUDE_VERSION=2.1.248
ARG CODEX_VERSION=0.150.1
ARG CURSOR_AGENT_VERSION=2026.08.25-3e8eec8
ARG SILVERBULLET_VERSION=2.10.0
ARG KASMVNC_VERSION=1.5.0
ARG TTYD_VERSION=1.7.7

# --- silverbullet -----------------------------------------------------------
# main.go embeds silverbullet/silverbullet (the SilverBullet wiki server).
# That file is a build artifact and is not in git. Fetch the upstream static
# musl release for the target CPU. The vendored silverbullet/ tree is the
# same version (see silverbullet/package.json).
FROM debian:13-slim AS silverbullet
ARG SILVERBULLET_VERSION
ARG TARGETARCH
RUN set -eux; \
    apt-get update; \
    apt-get install -y --no-install-recommends ca-certificates curl unzip; \
    rm -rf /var/lib/apt/lists/*; \
    case "$TARGETARCH" in \
      amd64) sbarch=x86_64 ;; \
      arm64) sbarch=aarch64 ;; \
      arm)   sbarch=armv7 ;; \
      *) echo "ERROR: no SilverBullet release for TARGETARCH=$TARGETARCH" >&2; exit 1 ;; \
    esac; \
    curl -fsSL -o /tmp/sb.zip \
      "https://github.com/silverbulletmd/silverbullet/releases/download/${SILVERBULLET_VERSION}/silverbullet-server-linux-${sbarch}.zip"; \
    unzip -q /tmp/sb.zip -d /out; \
    chmod +x /out/silverbullet; \
    /out/silverbullet --version

# --- builder ----------------------------------------------------------------
FROM golang:${GO_VERSION}-trixie AS builder
ARG CGO_ENABLED=1
ENV CGO_ENABLED=${CGO_ENABLED}

# ICU headers + a C++ compiler: github.com/dolthub/go-icu-regex (a dependency
# of the embedded Dolt) needs unicode/regex.h when CGO_ENABLED=1.
RUN set -eux; \
    apt-get update; \
    apt-get install -y --no-install-recommends libicu-dev g++ pkg-config; \
    rm -rf /var/lib/apt/lists/*

# go.mod has local replace directives (./beads, ...), so the whole tree is
# needed before go mod download. Module and build caches keep rebuilds fast.
WORKDIR /src
COPY . .
COPY --from=silverbullet /out/silverbullet silverbullet/silverbullet
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go build -trimpath -ldflags="-s -w" -o /out/iugum . \
    && /out/iugum --help >/dev/null

# --- runtime ----------------------------------------------------------------
FROM debian:13-slim
ARG WITH=all
ARG CODE_SERVER=
ARG BROWSER=
ARG CGO_ENABLED=1
ARG CODE_SERVER_VERSION
ARG OPENCODE_VERSION
ARG CLAUDE_VERSION
ARG CODEX_VERSION
ARG CURSOR_AGENT_VERSION
ARG KASMVNC_VERSION
ARG TTYD_VERSION
ARG TARGETARCH
ENV DEBIAN_FRONTEND=noninteractive

# Base packages. libicu76 is the ICU runtime that a CGO build links against.
# It is small and always installed, so one runtime stage serves both builds.
# sqlite3 is needed for agent checkpoint WAL work inside the container.
RUN set -eux; \
    apt-get update; \
    apt-get install -y --no-install-recommends sqlite3 \
        ca-certificates curl git jq less libicu76 procps ripgrep tzdata; \
    apt-get clean; rm -rf /var/lib/apt/lists/*

# ttyd: one-tab web shell (PTY over WebSocket). ~1.3 MB static binary.
RUN set -eux; \
    case "$TARGETARCH" in \
      amd64) tarch=x86_64 ;; \
      arm64) tarch=aarch64 ;; \
      *) echo "ERROR: no ttyd binary for TARGETARCH=$TARGETARCH" >&2; exit 1 ;; \
    esac; \
    curl -fsSL -o /usr/local/bin/ttyd \
      "https://github.com/tsl0922/ttyd/releases/download/${TTYD_VERSION}/ttyd.${tarch}"; \
    chmod 0755 /usr/local/bin/ttyd; \
    ttyd --version

# Resolve WITH into /etc/iugum-with (one item per line). Unknown item = fail.
RUN set -eu; \
    known="claude codex opencode cursor code-server browser"; \
    case "$WITH" in \
      all)     items="$known" ;; \
      none|"") items="" ;; \
      *)       items="$(printf '%s' "$WITH" | tr ',' ' ')" ;; \
    esac; \
    out=""; \
    for i in $items; do \
      ok=0; for k in $known; do [ "$i" = "$k" ] && ok=1; done; \
      [ "$ok" = 1 ] || { echo "ERROR: unknown WITH item '$i'. Known: $known, all, none" >&2; exit 1; }; \
      out="$out $i"; \
    done; \
    : > /etc/iugum-with; \
    for i in $out; do echo "$i" >> /etc/iugum-with; done; \
    apply_overlay() { \
      item="$1"; val="$2"; name="$3"; \
      case "${val:-}" in \
        "" ) ;; \
        1|true|on|yes) grep -qx "$item" /etc/iugum-with || echo "$item" >> /etc/iugum-with ;; \
        0|false|off|no) grep -vx "$item" /etc/iugum-with > /tmp/iugum-with || true; mv /tmp/iugum-with /etc/iugum-with ;; \
        *) echo "ERROR: $name must be 1, 0, or empty (got '$val')" >&2; exit 1 ;; \
      esac; \
    }; \
    apply_overlay code-server "${CODE_SERVER:-}" CODE_SERVER; \
    apply_overlay browser "${BROWSER:-}" BROWSER; \
    echo "WITH resolved to: $(tr '\n' ' ' < /etc/iugum-with | sed 's/[[:space:]]*$//')"

# Node 22 - only when an npm-installed CLI is selected.
RUN set -eu; \
    if grep -qxE 'claude|codex|opencode' /etc/iugum-with; then \
      curl -fsSL https://deb.nodesource.com/setup_22.x | bash -; \
      apt-get install -y --no-install-recommends nodejs; \
      apt-get clean; rm -rf /var/lib/apt/lists/*; \
      npm config set update-notifier false --global; \
    fi

# Non-root user. uid 1000, writable /workspace and /data.
RUN set -eux; \
    useradd -m -u 1000 -s /bin/bash iugum; \
    mkdir -p /workspace /data; \
    chown 1000:1000 /workspace /data

# npm CLIs. Each package is pinned (npm semver, no v prefix).
RUN set -eu; \
    if grep -qx claude   /etc/iugum-with; then npm i -g "@anthropic-ai/claude-code@${CLAUDE_VERSION}"; fi; \
    if grep -qx codex    /etc/iugum-with; then npm i -g "@openai/codex@${CODEX_VERSION}"; fi; \
    if grep -qx opencode /etc/iugum-with; then npm i -g "opencode-ai@${OPENCODE_VERSION}"; fi; \
    if grep -qxE 'claude|codex|opencode' /etc/iugum-with; then \
      npm cache clean --force; rm -rf /root/.npm; \
    fi

# cursor-agent (also linked as `agent`): pinned lab tarball, not the floating
# cursor.com/install script. Run as uid 1000 so files are owned by iugum.
RUN set -eu; \
    if grep -qx cursor /etc/iugum-with; then \
      case "$TARGETARCH" in \
        amd64) carch=x64 ;; \
        arm64) carch=arm64 ;; \
        *) echo "ERROR: no cursor-agent package for TARGETARCH=$TARGETARCH" >&2; exit 1 ;; \
      esac; \
      dest="/home/iugum/.local/share/cursor-agent/versions/${CURSOR_AGENT_VERSION}"; \
      install -d -o 1000 -g 1000 "$dest" /home/iugum/.local/bin /home/iugum/.local/share/cursor-agent/versions; \
      curl -fsSL "https://downloads.cursor.com/lab/${CURSOR_AGENT_VERSION}/linux/${carch}/agent-cli-package.tar.gz" \
        | tar --strip-components=1 -xzf - -C "$dest"; \
      chown -R 1000:1000 /home/iugum/.local; \
      ln -sf "$dest/cursor-agent" /home/iugum/.local/bin/cursor-agent; \
      ln -sf "$dest/cursor-agent" /home/iugum/.local/bin/agent; \
      ln -sf /home/iugum/.local/bin/cursor-agent /usr/local/bin/cursor-agent; \
      ln -sf /home/iugum/.local/bin/agent /usr/local/bin/agent; \
      cursor-agent --version; \
    fi

# code-server: official installer, pinned release. Extensions dir is shared
# and owned by uid 1000. Binds 0.0.0.0:8080 by default.
RUN set -eu; \
    if grep -qx code-server /etc/iugum-with; then \
      curl -fsSL https://code-server.dev/install.sh | sh -s -- --version "$CODE_SERVER_VERSION"; \
      mkdir -p /opt/code-server-extensions /home/iugum/.config/code-server /home/iugum/.local/share/code-server; \
      printf 'bind-addr: 0.0.0.0:8080\nauth: none\ncert: false\n' > /home/iugum/.config/code-server/config.yaml; \
      chown -R 1000:1000 /opt/code-server-extensions /home/iugum/.config /home/iugum/.local; \
      rm -rf /var/lib/apt/lists/* /var/cache/apt/archives/*.deb; \
      code-server --version; \
    fi

# browser: Chromium on KasmVNC (X server + web client, seamless clipboard
# in Chrome/Edge). Opt out with BROWSER=0. Scripts copy after the package
# install so a script-only change does not re-download the KasmVNC deb.
RUN set -eu; \
    install -d /etc/kasmvnc /usr/local/lib/iugum; \
    if grep -qx browser /etc/iugum-with; then \
      apt-get update; \
      apt-get install -y --no-install-recommends \
        chromium openssl fonts-liberation fonts-unifont dbus-x11; \
      curl -fsSL -o /tmp/kasmvnc.deb \
        "https://github.com/kasmtech/KasmVNC/releases/download/v${KASMVNC_VERSION}/kasmvncserver_trixie_${KASMVNC_VERSION}_${TARGETARCH}.deb"; \
      apt-get install -y /tmp/kasmvnc.deb; \
      rm -f /tmp/kasmvnc.deb; \
      apt-get clean; rm -rf /var/lib/apt/lists/*; \
      install -d -o 1000 -g 1000 /home/iugum/.chromium; \
      chromium --version; \
    fi
# xdpyinfo: Chromium can match the desktop after KasmVNC resizes to the tab.
RUN set -eu; \
    if grep -qx browser /etc/iugum-with; then \
      apt-get update; \
      apt-get install -y --no-install-recommends x11-utils openbox; \
      apt-get clean; rm -rf /var/lib/apt/lists/*; \
    fi
COPY scripts/container/iugum-browser.sh /usr/local/bin/iugum-browser
COPY scripts/container/browser-xstartup /usr/local/lib/iugum/browser-xstartup
COPY scripts/container/openbox-rc.xml /usr/local/lib/iugum/openbox-rc.xml
COPY scripts/container/kasmvnc.yaml /etc/kasmvnc/kasmvnc.yaml
RUN chmod 0755 /usr/local/bin/iugum-browser /usr/local/lib/iugum/browser-xstartup

COPY --from=builder /out/iugum /usr/local/bin/iugum

# iugum.with records the WITH build arg. The resolved item list (one per
# line) is at /etc/iugum-with inside the image.
LABEL org.opencontainers.image.source="https://github.com/srhopkins/iugum" \
      org.opencontainers.image.title="iugum" \
      org.opencontainers.image.description="iugum with agent CLIs selected by the WITH build arg" \
      iugum.with="${WITH}" \
      iugum.cgo="${CGO_ENABLED}" \
      iugum.code_server_version="${CODE_SERVER_VERSION}" \
      iugum.opencode_version="${OPENCODE_VERSION}" \
      iugum.claude_version="${CLAUDE_VERSION}" \
      iugum.codex_version="${CODEX_VERSION}" \
      iugum.cursor_agent_version="${CURSOR_AGENT_VERSION}"

ENV IUGUM_DATA=/data \
    HOME=/home/iugum \
    PATH=/home/iugum/.local/bin:/usr/local/bin:/usr/bin:/bin

USER 1000
WORKDIR /workspace
EXPOSE 8080 6080 7681
ENTRYPOINT ["iugum"]
CMD ["up"]
