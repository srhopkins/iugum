# syntax=docker/dockerfile:1.7
# Build from the repository root: docker build -f agentbox/Dockerfile -t agentbox .
ARG GO_VERSION=1.26.5
# BASE=chromium (default): Chromium-only Selkies base. BASE=kde: full KDE desktop.
ARG BASE=chromium
ARG CHROME_IMAGE=lscr.io/linuxserver/chrome@sha256:75f9d59ff04952e1a9060dc2bd254f63776ce5181f7c4c21a9d392f3ddb7ef4e
ARG KDE_IMAGE=lscr.io/linuxserver/webtop:ubuntu-kde@sha256:22a6eb6e798abbbac3faed236dfb140b1451051b59fc58fd21eea097d1497ab9
FROM golang:${GO_VERSION} AS go-toolchain
FROM ${CHROME_IMAGE} AS base-chromium
FROM ${KDE_IMAGE} AS base-kde
FROM base-${BASE} AS base

# Build against the same libc/ICU as the desktop, without a host-built binary.
FROM base AS builder
USER root
COPY --from=go-toolchain /usr/local/go /usr/local/go
ENV PATH=/usr/local/go/bin:$PATH CGO_ENABLED=1 GOPATH=/go GOCACHE=/root/.cache/go-build
RUN apt-get update && apt-get install -y --no-install-recommends \
    libicu-dev g++ pkg-config git unzip ca-certificates curl \
    && rm -rf /var/lib/apt/lists/*
WORKDIR /src
COPY . .
ARG TARGETARCH
ARG SILVERBULLET_VERSION=2.10.0
RUN case "$TARGETARCH" in amd64) arch=x86_64;; arm64) arch=aarch64;; *) exit 1;; esac \
    && curl -fsSL "https://github.com/silverbulletmd/silverbullet/releases/download/${SILVERBULLET_VERSION}/silverbullet-server-linux-${arch}.zip" -o /tmp/wiki.zip \
    && unzip /tmp/wiki.zip -d /tmp/wiki \
    && install -m 755 /tmp/wiki/silverbullet wikiblob/silverbullet
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go build -buildvcs=false -trimpath -ldflags='-s -w -X main.buildCGO=on' -o /out/iugum .

FROM base AS desktop
USER root
ARG BASE
ARG TARGETARCH
ARG NODE_VERSION=22.22.2
ARG CODE_SERVER_VERSION=4.134.0
ARG CLAUDE_VERSION=2.1.248
ARG CODEX_VERSION=0.150.1
ARG OPENCODE_VERSION=1.18.23
ARG CURSOR_AGENT_VERSION=2026.08.25-3e8eec8
ARG PLAYWRITER_VERSION=0.7.0
ARG PLAYWRIGHT_VERSION=1.63.0

# The base's dockremap account can prevent abc from taking PUID=1000.
RUN if getent passwd dockremap >/dev/null; then usermod -u 300000 dockremap; fi \
    && if getent group dockremap >/dev/null; then groupmod -g 300000 dockremap; fi
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates curl gnupg git jq ripgrep sqlite3 gh python3 python3-venv \
    ffmpeg fonts-dejavu-core sox libsox-fmt-pulse pulseaudio-utils espeak-ng \
    xdotool xz-utils libicu-dev \
    && rm -rf /var/lib/apt/lists/*
RUN case "$TARGETARCH" in amd64) arch=x64;; arm64) arch=arm64;; *) exit 1;; esac \
    && curl -fsSL "https://nodejs.org/dist/v${NODE_VERSION}/node-v${NODE_VERSION}-linux-${arch}.tar.gz" \
       | tar -xz -C /usr/local --strip-components=1
RUN curl -fsSL https://code-server.dev/install.sh | sh -s -- --version "$CODE_SERVER_VERSION" \
    && rm -rf /var/lib/apt/lists/* /root/.cache
RUN --mount=type=cache,target=/config/.npm \
    npm install -g "@anthropic-ai/claude-code@${CLAUDE_VERSION}" \
      "@openai/codex@${CODEX_VERSION}" "opencode-ai@${OPENCODE_VERSION}" \
      "playwriter@${PLAYWRITER_VERSION}" "playwright@${PLAYWRIGHT_VERSION}" "@playwright/test@${PLAYWRIGHT_VERSION}"
RUN case "$TARGETARCH" in amd64) arch=x64;; arm64) arch=arm64;; *) exit 1;; esac \
    && mkdir -p /opt/cursor-agent \
    && curl -fsSL "https://downloads.cursor.com/lab/${CURSOR_AGENT_VERSION}/linux/${arch}/agent-cli-package.tar.gz" \
       | tar -xz --strip-components=1 -C /opt/cursor-agent \
    && ln -s /opt/cursor-agent/cursor-agent /usr/local/bin/cursor-agent \
    && ln -s /opt/cursor-agent/cursor-agent /usr/local/bin/agent
RUN curl -LsSf https://astral.sh/uv/install.sh \
    | env UV_INSTALL_DIR=/usr/local/bin UV_NO_MODIFY_PATH=1 sh
# Google Chrome is core: it supports signed-in profiles, Chromium does not.
# The chromium base already ships it; the KDE base does not. Google publishes
# Chrome for amd64 only, so arm64 builds keep the Playwright Chromium.
RUN if [ ! -x /usr/bin/google-chrome-stable ] && [ "$(dpkg --print-architecture)" = amd64 ]; then \
      curl -fsSL https://dl.google.com/linux/linux_signing_key.pub | gpg --dearmor -o /usr/share/keyrings/google-chrome.gpg \
      && echo "deb [arch=amd64 signed-by=/usr/share/keyrings/google-chrome.gpg] https://dl.google.com/linux/chrome/deb/ stable main" > /etc/apt/sources.list.d/google-chrome.list \
      && apt-get update && apt-get install -y --no-install-recommends google-chrome-stable \
      && rm -rf /var/lib/apt/lists/*; \
    fi
ENV PLAYWRIGHT_BROWSERS_PATH=/opt/ms-playwright
RUN npx playwright install --with-deps chromium \
    && rm -rf /var/lib/apt/lists/* /root/.cache /root/.npm /config/.cache /config/.npm

ENV TZ=UTC \
    PULSE_SERVER=unix:/defaults/native \
    SELKIES_MANUAL_WIDTH=1920 \
    SELKIES_MANUAL_HEIGHT=1080 \
    SELKIES_IS_MANUAL_RESOLUTION_MODE=true \
    SELKIES_SCALING_DPI=96 \
    SELKIES_USE_CSS_SCALING=false \
    SELKIES_USE_BROWSER_CURSORS=true \
    SELKIES_RECORDING_SOCKET=/config/.selkies-recording \
    PIXELFLUX_RECORDING_SOCKET=/config/.selkies-recording \
    CHROME_CLI="--remote-debugging-port=9222 --remote-debugging-address=127.0.0.1 --user-data-dir=/config/.chrome-profile --disable-dev-shm-usage --force-device-scale-factor=1.25 http://127.0.0.1:8080/?folder=/config/workspace"
COPY --from=builder /out/iugum /usr/local/bin/iugum
COPY --chmod=755 agentbox/runtime.sh /usr/local/lib/agentbox/runtime.sh
COPY agentbox/WORKFLOW.md /usr/local/share/agentbox/WORKFLOW.md
COPY --chmod=755 agentbox/50-install-d.sh /custom-cont-init.d/50-install-d.sh
COPY --chmod=755 agentbox/kde/ /usr/local/lib/agentbox/kde/
RUN mkdir -p /custom-cont-init.d /custom-services.d \
    && if [ "$BASE" = kde ]; then \
         ln -s /usr/local/lib/agentbox/kde/97-xwayland-input.sh /custom-cont-init.d/97-xwayland-input.sh \
         && ln -s /usr/local/lib/agentbox/kde/98-kde-browser.sh /custom-cont-init.d/98-kde-browser.sh; \
       fi \
    && printf '#!/bin/sh\nexec /usr/local/lib/agentbox/runtime.sh init\n' > /custom-cont-init.d/99-agentbox.sh \
    && printf '#!/bin/sh\nif [ -n "${PASSWORD:-}" ]; then set -- --auth password; else set --; fi\nexec s6-setuidgid abc env HOME=/config code-server --bind-addr 0.0.0.0:8080 "$@"\n' > /custom-services.d/code-server \
    && chmod 755 /custom-cont-init.d/99-agentbox.sh /custom-services.d/code-server \
    && for command in playwriter-mcp bd say agentbox-record agentbox-screenshot; do \
         ln -s /usr/local/lib/agentbox/runtime.sh /usr/local/bin/$command; \
       done
LABEL agentbox.base=$BASE
EXPOSE 3000 3001 8080
HEALTHCHECK --start-period=120s --interval=30s --timeout=5s \
    CMD curl -fsS http://127.0.0.1:8080/healthz >/dev/null && curl -fkSs https://127.0.0.1:3001/ >/dev/null
