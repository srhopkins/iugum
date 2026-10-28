# iugum agentbox

A shared desktop for an agent and a person: Chrome streamed with Selkies,
VS Code through code-server, Claude Code, Codex, OpenCode, Cursor Agent,
Playwriter, Playwright, iugum/Beads, recording, screenshots, and speech.

## Start from a clone

From the repository root, with Docker and Compose installed:

```sh
docker compose up --build -d
```

Open **https://localhost:3011** for the shared Chrome desktop. Accept the
container's self-signed certificate. Open **http://localhost:8090** for the
editor directly. Chrome inside the desktop already opens that editor.
Enable audio in the Selkies client to hear narration.

Open an editor terminal, clone your project into `/config/workspace`, and run
`claude`, `codex`, `opencode`, or `cursor-agent`. Sign into your own provider.
Playwriter connections are seeded for all four agents and code-server.
Ask the agent to make a change, walk you through it in the visible editor,
or demonstrate the running application. The container includes the
[workflow guide](WORKFLOW.md) for browser control, recordings, and speech.

The build compiles iugum with embedded Dolt support. It downloads the upstream
SilverBullet release for the embedded wiki, as the existing root image does;
it does not compile the repository's patched SilverBullet source.
No prebuilt binary, private host path, GPU, speech server, proxy, or local DNS
is needed. First builds download several gigabytes of tooling.

## Files and ownership

The canonical build is `agentbox/Dockerfile`; its build context is the entire
iugum repository. The root `compose.yaml` supplies portable runtime defaults.
`runtime.sh` contains the desktop initialization and media command shims.
This container integration is the single-file shell exception to the Go rule.
The existing root `Dockerfile` continues to build the separate iugum service
image; use this Dockerfile for Selkies.

The named Compose volume at `/config` contains project files, credentials,
browser profiles, settings, and recordings. `docker compose down` keeps it.
Each desktop needs its own volume. Initialization preserves existing settings
and adds missing Playwriter entries in JSON configs. An existing Codex TOML
config is preserved; add `[mcp_servers.playwriter]` with
`command = "/usr/local/bin/playwriter-mcp"` yourself if that entry is missing.

## Use the iugum launcher

Build the image once on the Docker engine that will run it:

```sh
docker build -f agentbox/Dockerfile -t agentbox:latest .
iugum agent up demo --kind selkies
iugum agent shell demo
```

The same localhost URLs apply. iugum creates its own network and a named
`demo-config` volume. It uses an existing image; `agent up` does not build it.
Compose and iugum are alternative launchers, with separate volumes. Stop one
before starting the other on the default ports.

For a second desktop, replace the host bindings:

```sh
iugum agent up second --kind selkies \
  --port 127.0.0.1:4010:3000 \
  --port 127.0.0.1:4011:3001 \
  --port 127.0.0.1:9090:8080
```

A `--port` entry replaces the preset binding for that container port/protocol.
New container ports append. The same rule applies to `agent.yaml` ports.

## Deployment overrides

Ports bind to the Docker host's loopback address. A remote Docker context
requires an SSH tunnel or your own reverse proxy; its ports do not appear on
the client machine automatically. For example:

```sh
ssh -N -L 3011:127.0.0.1:3011 -L 8090:127.0.0.1:8090 user@docker-host
```

Compose uses its own network. iugum uses a per-agent network. To join an
existing proxy network with iugum, pass `--network NAME` and your own `--label`
values, or save those fields in `agent.yaml`. Context names do not choose
networks, domains, or proxy labels. Existing running containers are unchanged;
save their deployment-specific routing before recreating them.

For Compose, put host-specific mounts, networks, labels, and authentication in
the ignored root `compose.override.yaml`. For example, an optional project mount:

```yaml
services:
  agentbox:
    volumes:
      - /absolute/path/to/project:/config/workspace/project
```

Match `PUID`/`PGID` to the owner of a Linux bind mount. Do not share a `/config`
volume between desktops. No credentials or host SSH directories are mounted
by default. Default services have no authentication; keep loopback bindings
unless you configure access controls. `CUSTOM_USER` and `PASSWORD` enable
the base image's login; `PASSWORD` also enables code-server authentication.

The ignored root `.env` can set `AGENTBOX_HTTP_PORT`, `AGENTBOX_HTTPS_PORT`,
`AGENTBOX_EDITOR_PORT`, `AGENTBOX_IMAGE`, `AGENTBOX_MEMORY`, `AGENTBOX_CPUS`,
`AGENTBOX_TITLE`, `PUID`, `PGID`, and `TZ`. Compose supports multiple copies
with separate project names and different host ports.

## Media

Inside the desktop terminal:

```sh
say "Here is the change."
agentbox-screenshot /config/recordings/app.png localhost
agentbox-record /config/recordings/demo.mkv
# Stop with Ctrl-C; the recording includes desktop audio.
ffprobe /config/recordings/demo.mkv
```

Local eSpeak NG supplies speech without a service. For an optional speech API,
set `TTS_URL` to its base URL, plus `TTS_MODEL` and `TTS_VOICE`. The helper calls
`/v1/audio/speech`. No homelab address is assumed.
The recorder needs an active Selkies stream and its recording socket. Save
as Matroska (`.mkv`); convert after recording if an MP4 is needed:

```sh
ffmpeg -i /config/recordings/demo.mkv -c copy /config/recordings/demo.mp4
docker compose cp agentbox:/config/recordings/demo.mp4 ./demo.mp4
```

## Updating

Build args in the Dockerfile select CLI versions and the Chrome base image.
The Chrome base is pinned by digest; Playwriter and Playwright use explicit
versions. Override those args deliberately when updating the ecosystem.
Rebuild with `docker compose build --pull`, then `docker compose up -d`.
Recreating the container keeps its named volume.

## Verify a build

```sh
AGENTBOX_TEST_IMAGE=agentbox:latest go test ./agentbox -v -timeout 5m
```

This opt-in test uses Docker's selected context. It creates and removes its own
container and volume, checks startup and saved settings across recreation, and
records the shared stream with local narration. It checks the recording's
duration, decoded video frame, and non-silent audio. It needs no provider login.

## Optional T3 server

Build with `docker build -f agentbox/Dockerfile --build-arg T3=1 -t agentbox:t3 .`.
`T3_VERSION` pins the official release (default `0.1.0`). The download is checked
against the release checksum file. T3 runs as `abc`, under s6, on port 3773.
Its pairing, projects, and conversation state stay in `/config/.t3`.
Publish `127.0.0.1:3773:3773` or configure your HTTPS reverse proxy.
No Electron app, systemd, or Selkies connection is needed for the web interface.
See [existing-agent installation](../docs/t3-agent-servers.md) for a live install
that preserves running agent sessions.
