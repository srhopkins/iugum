# Agent homes

`iugum agent init <name>` creates one directory that owns an agent's container inventory:

```text
<name>/
├── agent.yaml
├── home/
│   ├── .iugum-probe
│   └── policy.csv
└── data/
    └── .iugum-probe
```

The command refuses to replace an existing `agent.yaml`.
Inside a git worktree, it warns when `home/` or `data/` is not ignored.
These directories can contain private credentials and runtime state.

## Lifecycle

```text
iugum agent up scout
iugum agent status scout
iugum agent ls
iugum agent down scout
iugum agent rm scout
```

`up` creates the agent's `iugum-agent-scout` network (labeled `iugum.managed=true` and `iugum.agent=scout`) and starts a detached container, with restart policy `unless-stopped`.
The container runs as no fixed user by default; set `user` in `agent.yaml`, or pass `--user`, to add `--user`. This is a behavior change: earlier versions always ran the container as `1000:1000`. Images that must start as root (for example linuxserver.io images that drop privilege themselves via s6-init) need `user` left empty.
`startup.command` is optional extra argv after the image. Use `[run]` for jobs only (no wiki). Use `[up, --hostname, 0.0.0.0]` when the image has code-server and you publish port 8080. Empty keeps the image default (`up`).
Running `up` again leaves a running container unchanged or starts a stopped container.
Use `--dry-run` on `up`, `down`, or `rm` to print the Docker or Podman commands.

### `up` with no `agent.yaml`

`up` works in an empty directory: no `./NAME/agent.yaml` is required as long as `--kind` or `--image` is given.

```text
iugum agent up chrome1 --kind selkies
```

`--kind selkies` fills in a built-in preset: image, volumes, env, memory/CPU limits, and the reverse-proxy labels for the active Docker context (Traefik on the homelab tower, Caddy on Docker Desktop for Mac). The Docker context comes from the `DOCKER_CONTEXT` environment variable if set, else `<engine> context show` (podman always uses the homelab preset).

`up` flags: `--image`, `--kind`, `--label KEY=VALUE` (repeatable), `--volume SPEC` (repeatable), `--network NAME` (join an existing external network), `--shm-size SIZE`, `--env KEY=VALUE` (repeatable), `--user USER`, `--mem SIZE`, `--cpus N`, `--port SPEC` (repeatable), plus `--engine` and `--dry-run`.

Precedence, low to high: the `--kind` preset, then `./NAME/agent.yaml` if it exists, then flags. `--label`/`--volume`/`--env`/`--port` add to whatever the preset and `agent.yaml` already set; every other flag replaces the merged value. With no `agent.yaml`, no `--kind`, and no `--image`, `up` fails and names `--image` or `--kind` in its error.

`down` stops and removes the container. It keeps any named volumes.
It removes the network when no container still uses it, unless `network.external` is set (see below).
`rm` stops and removes the container, its network (unless external), and every volume labeled `iugum.agent=NAME` — including the `NAME-config` convention volume a `--kind` preset uses. It asks you to type the agent's name again on stdin, unless `--yes` is given.
`status` reports `running` or `not-running` and succeeds for a missing container.
`ls` lists directories with an `agent.yaml` under the current directory, merged with any running or stopped container found by the `iugum.managed=true` label — so a container started with `--kind` and no directory still shows up.

`status`, `down`, `tui`, `acp`, and `rm` all work on a container with no `agent.yaml` on disk: they look it up by the `iugum.agent=NAME` container label.

Network mode `locked` is reserved for future network enforcement.
Lifecycle commands reject it until that enforcement exists.

## OpenCode access

```text
iugum agent tui scout
iugum agent acp scout
```

`tui` runs `opencode` in the agent container with an interactive terminal.
`acp` runs `opencode acp` as an Agent Client Protocol (ACP) JSON-RPC bridge.
The ACP bridge uses stdin and stdout directly and does not allocate a terminal.
Use `--dry-run` to print either container command without running it.

Both commands require the agent container to be running.
They use Docker or Podman from the current environment.

## Memory checkpoint

`iugum agent checkpoint scout` checkpoints `scout/home/memory.db` with host-side `sqlite3`.
It stages only that database and commits it in the enclosing agent-homes git repo.
It never stages the SQLite `-wal` or `-shm` files.
The command succeeds without a commit when the database is missing or unchanged.

## agent.yaml

`agent.yaml` is the single source of truth for one agent.
Keep it sparse.
Add a field only when the agent needs it.

```yaml
name: scout
image: iugum:latest
mounts:
  - source: ./home
    target: /home/iugum
  - source: ./data
    target: /data
network:
  name: scout
  mode: open
startup:
  restart: unless-stopped
```

- `name` is the agent and container name.
- `image` is the container image.
- `kind` is a free-text label for the agent's flavor (example `selkies`). It shows up as the `iugum.kind` container label; empty defaults to `custom`.
- `user` sets Docker `--user` (example `1000:1000`). Empty omits the flag, so the image's own entrypoint decides. Images that must start as root belong here with `user` left unset.
- `labels` is a list of `KEY=VALUE` strings, each added with its own `--label`. Use this for reverse-proxy routing labels (Traefik, Caddy).
- `mounts` is a list of bind mounts or tmpfs masks. A bind mount has `source` and `target`. Set `ro: true` for read-only access. A tmpfs mask has `target` and `tmpfs: true`, with no `source`.
- `volumes` is a list of named-volume specs, `name:target[:ro]` (example `sel-config:/config`). Unlike `mounts`, these are Docker/Podman named volumes, not host paths. `up` creates a missing volume first (`<engine> volume create`, stamped with the same `iugum.*` labels as the container); `down` never removes it.
- `ports` is an optional list of Docker-style port mappings, such as `127.0.0.1:8080:8080`.
- `network.name` is the agent's Docker network.
- `network.mode` defaults to `open`. `locked` is reserved for network enforcement.
- `network.external` joins an existing network by name instead of the agent's own private one (example: a shared Traefik/Caddy proxy network). `up` does not create it and `down` does not remove it.
- `privileges.cap_add` is an optional list of Linux capabilities. The container still runs as a non-root user unless `user` says otherwise.
- `startup.restart` defaults to `unless-stopped`.
- `startup.env` is an optional list of host environment variable names to pass through. Do not put secret values in this file.
- `env` is an optional list of literal `KEY=VALUE` pairs added with `-e`, alongside the `startup.env` name passthrough.
- `mem` sets Docker `--memory` and `--memory-swap` to the same value (example `5g`).
- `cpus` sets Docker `--cpus` (example `"3"`).
- If `home/.env` exists, `up` passes it to Docker as `--env-file`. That file is gitignored with the rest of `home/`. Put long-lived tokens there (`HASS_TOKEN=...`).
- `jobs` points to a cron jobs file (default `jobs.yaml`). `up` mounts it at `/workspace/jobs.yaml` and sets `IUGUM_JOBS`.
- `shm_size` is Docker `/dev/shm` size (example `1g`). Chromium needs this.
- `extra_hosts` is a list of Docker `--add-host` entries. Empty defaults to `host.docker.internal:host-gateway` so Linux containers can reach services on the host.

Every container also gets `--label iugum.managed=true`, `--label iugum.agent=<name>`, `--label iugum.image=<image>`, and `--label iugum.kind=<kind or custom>`. A named volume `up` creates gets `iugum.managed=true` and `iugum.agent=<name>` too. The private network `up` creates is not labeled; `docker network create` takes the network name as its last, positional argument, so it cannot carry labels the same way.

Inside the container the agent adds work with `iugum job`:

```text
iugum job add hourly-checks --every 1h --prompt "Run the hourly-checks skill now."
```

That writes `jobs.yaml`. The running `iugum up` process loads the new job within a second. `kind: session` injects the prompt into the standing OpenCode session. Write the skill under `.opencode/skills/<name>/SKILL.md`.

A `kind: session` job also takes two optional stall-watchdog fields, both Go duration strings:

```yaml
jobs:
  - name: hourly-checks
    spec: "@every 1h"
    kind: session
    prompt: "Run the hourly-checks skill now."
    timeout: 4h        # hard ceiling on the whole job; default 4h
    idle_timeout: 10m  # kill if the job goes this long with no activity; default 10m
```

- `timeout` is the hard ceiling. The job is killed once it elapses, no matter how busy it is.
- `idle_timeout` fires on silence: no stdout/stderr byte, no ACP event (these ride the stdout stream, so it's one signal), and no file write under the job's working directory. A chatty job resets this clock on every event; a stuck one hits it even with time left on `timeout`.
- Either limit kills the whole process group (not just the direct child) and logs which one fired: `killed on timeout limit` or `killed on idle limit`.
- Both fields are optional. A `jobs.yaml` written before they existed loads unchanged and gets the defaults above.

Cron is allowed by default. Lock it in `home/policy.csv`:

```text
p, *, *, *, allow
p, *, schedule, add, deny
p, *, schedule, remove, deny
```

`data/iugum.yaml` points Casbin at `/home/iugum/policy.csv`. The embedded model honors deny rows.

The starter `home/policy.csv` matches iugum's current Casbin allow-all policy.
Replace its allow row with narrower rules when an agent needs restrictions.

## Native agents

`iugum agent run --config examples/chief/agent.yaml` runs an agent's local chat workspace and native model/tool loop. It leaves the existing container commands intact. See [Native agent workspace](native-agent.md) for configuration, model pools, policy, and current limitations.

`iugum agent clone --config agent.yaml --name candidate --output ./candidate` creates a named configuration candidate with separate writable state. See [Named clones](native-clones.md).

`iugum agent session capabilities` describes verified coding-session control. See [Claude session control](native-session-control.md).

### Wiki model selection and settings

The chat model picker lists the active runtime's choices. Native agents expose configured profiles; selecting one uses the same policy, credentials and routing checks as the `model_select` tool. It does not switch billing transports. ACP agents expose model choices from session configuration options. Selection is saved locally, applied to resumed turns, and checked against current policy. Discovering ACP choices starts or resumes the provider session but sends no prompt.

Model selection applies to the selected agent's next turn. It preserves conversations and namespaces. Managed-agent connections proxy the same model and settings endpoints through the attachment gate. Providers without choices display an unavailable explanation.

The Agent settings drawer edits configured mission and instruction documents for native agents. Each read/write checks `agent/settings/mission` or `agent/settings/instructions`; API access also passes the normal agentdesk policy check. Saves replace only the named configured document and apply on the next turn. The drawer does not edit permissions, credentials, namespaces, or runtime commands. ACP providers without an instruction-document editor report that limitation. Unsaved text remains available after a rejected save.
