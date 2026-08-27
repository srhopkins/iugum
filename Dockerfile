# syntax=docker/dockerfile:1.7
# ============================================================================
# iugum - one image, built from source, with the agent CLIs you choose.
#
# Build args:
#   WITH                comma list of: claude, codex, opencode, cursor, code-server
#                       "all" (default) installs every item. "none" installs none.
#   CGO_ENABLED         1 (default) links the embedded Dolt database (needs ICU).
#                       0 makes a static program with no C libraries.
#   GO_VERSION          Go toolchain tag for the builder stage.
#   CODE_SERVER_VERSION code-server release to install when WITH has code-server.
#
# Examples:
#   docker build -t iugum .
#   docker build --build-arg WITH=claude,code-server -t iugum:claude .
#   docker build --build-arg WITH=none --build-arg CGO_ENABLED=0 -t iugum:slim .
#
# Works with docker and podman. Full docs: docs/container.md
# ============================================================================

ARG GO_VERSION=1.26.5
ARG CODE_SERVER_VERSION=4.134.0
ARG SILVERBULLET_VERSION=2.10.0

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
ARG CGO_ENABLED=1
ARG CODE_SERVER_VERSION
ENV DEBIAN_FRONTEND=noninteractive

# Base packages. libicu76 is the ICU runtime that a CGO build links against.
# It is small and always installed, so one runtime stage serves both builds.
RUN set -eux; \
    apt-get update; \
    apt-get install -y --no-install-recommends \
        ca-certificates curl git jq less libicu76 procps ripgrep tzdata; \
    apt-get clean; rm -rf /var/lib/apt/lists/*

# Resolve WITH into /etc/iugum-with (one item per line). Unknown item = fail.
RUN set -eu; \
    known="claude codex opencode cursor code-server"; \
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
    echo "WITH resolved to: ${out:-none}"

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

# npm CLIs.
RUN set -eu; \
    if grep -qx claude   /etc/iugum-with; then npm i -g @anthropic-ai/claude-code; fi; \
    if grep -qx codex    /etc/iugum-with; then npm i -g @openai/codex; fi; \
    if grep -qx opencode /etc/iugum-with; then npm i -g opencode-ai; fi; \
    if grep -qxE 'claude|codex|opencode' /etc/iugum-with; then \
      npm cache clean --force; rm -rf /root/.npm; \
    fi

# cursor-agent: the installer writes to $HOME/.local/bin of the invoking user.
# Run it as uid 1000 so the files are owned by iugum, then link it onto PATH.
RUN set -eu; \
    if grep -qx cursor /etc/iugum-with; then \
      su iugum -c 'cd /home/iugum && curl https://cursor.com/install -fsS | bash'; \
      ln -s /home/iugum/.local/bin/cursor-agent /usr/local/bin/cursor-agent; \
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

COPY --from=builder /out/iugum /usr/local/bin/iugum

# iugum.with records the WITH build arg. The resolved item list (one per
# line) is at /etc/iugum-with inside the image.
LABEL org.opencontainers.image.source="https://github.com/srhopkins/iugum" \
      org.opencontainers.image.title="iugum" \
      org.opencontainers.image.description="iugum with agent CLIs selected by the WITH build arg" \
      iugum.with="${WITH}" \
      iugum.cgo="${CGO_ENABLED}" \
      iugum.code_server_version="${CODE_SERVER_VERSION}"

ENV IUGUM_DATA=/data \
    HOME=/home/iugum \
    PATH=/home/iugum/.local/bin:/usr/local/bin:/usr/bin:/bin

USER 1000
WORKDIR /workspace
EXPOSE 8080
ENTRYPOINT ["iugum"]
CMD ["up"]
