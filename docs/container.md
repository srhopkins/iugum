# iugum container image

The root `Dockerfile` builds iugum from source and installs the agent CLIs you select.
It works with `docker build` and `podman build`. A clone of this repo is all you need.

## Build

```bash
# Default: every CLI and code-server (large image).
docker build -t iugum .

# Only Claude Code and code-server, with the embedded Dolt database.
docker build --build-arg WITH=claude,code-server -t iugum:claude .

# Smallest image: iugum only, static binary.
docker build --build-arg WITH=none --build-arg CGO_ENABLED=0 -t iugum:slim .
```

## Build args

| Arg | Default | Meaning |
|-----|---------|---------|
| `WITH` | `all` | Comma list of CLIs to install: `claude`, `codex`, `opencode`, `cursor`, `code-server`. `all` installs every item. `none` installs no item. An unknown item stops the build with an error. |
| `CGO_ENABLED` | `1` | `1` links the embedded Dolt database (the storage for `iugum beads`). `0` makes a static program with no C libraries. A static program cannot open the embedded Dolt database. |
| `GO_VERSION` | `1.26.5` | Go toolchain tag for the builder stage. |
| `CODE_SERVER_VERSION` | `4.134.0` | code-server release to install when `WITH` has `code-server`. |
| `SILVERBULLET_VERSION` | `2.10.0` | SilverBullet (the wiki server that iugum embeds) release to fetch. |

### WITH items

| Item | Command on PATH | Install method |
|------|-----------------|----------------|
| `claude` | `claude` | `npm i -g @anthropic-ai/claude-code` |
| `codex` | `codex` | `npm i -g @openai/codex` |
| `opencode` | `opencode` | `npm i -g opencode-ai` |
| `cursor` | `cursor-agent` | `curl https://cursor.com/install \| bash`, run as the `iugum` user |
| `code-server` | `code-server` | Official install script, pinned to `CODE_SERVER_VERSION` |

Node 22 is installed only when `WITH` has `claude`, `codex`, or `opencode`.

### CGO and ICU

`iugum beads` uses an embedded Dolt database. Dolt depends on `github.com/dolthub/go-icu-regex`, and that package needs the ICU C library.
The builder stage installs `libicu-dev`, `g++`, and `pkg-config`.
The runtime stage installs `libicu76`, the ICU runtime for Debian 13.

With `CGO_ENABLED=0` the build skips ICU. The program is static. `iugum beads` then reports that embedded Dolt needs a CGO build.

## Image facts

| Item | Value |
|------|-------|
| Base | `debian:13-slim` |
| User | `iugum`, uid 1000 |
| Writable dirs | `/workspace` (working directory) and `/data` |
| `IUGUM_DATA` | `/data` (where iugum stores its databases) |
| Entrypoint | `iugum`, default command `up` |
| code-server | binds `0.0.0.0:8080`, no auth, extensions in `/opt/code-server-extensions` |
| Labels | `org.opencontainers.image.source`, `iugum.with` (the `WITH` value), `iugum.cgo`, `iugum.code_server_version` |

The resolved `WITH` list is also inside the image at `/etc/iugum-with`, one item per line.

## Run

```bash
# Show usage.
docker run --rm iugum --help

# Work on a project. Mount the project at /workspace and keep data in a volume.
docker run --rm -it -v "$PWD:/workspace" -v iugum-data:/data iugum beads list

# Run a CLI instead of iugum.
docker run --rm -it -v "$PWD:/workspace" --entrypoint claude iugum

# Start code-server on http://localhost:8080.
docker run --rm -p 8080:8080 -v "$PWD:/workspace" --entrypoint code-server iugum
```

Mounted files must be writable by uid 1000, or pass `--user "$(id -u):$(id -g)"`.

## Host or container

Both run the same program.

| | Host | Container |
|-|------|-----------|
| Install | `CGO_ENABLED=0 go build -o iugum .` then `scripts/install.sh` | `docker build -t iugum .` |
| Data dir | `~/Library/Application Support/iugum` or `IUGUM_DATA` | `/data` (`IUGUM_DATA`) |
| Agent CLIs | Whatever you install on the host | Only the `WITH` items |
| Embedded Dolt | Needs a CGO build on the host | `CGO_ENABLED=1` (default) |

Use the host build for daily work on this machine. Use the container to give an agent a clean, repeatable toolset, or to run iugum on a machine without Go.
