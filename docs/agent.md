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
```

`up` creates the agent's `iugum-agent-scout` network and starts a detached container.
The container runs as user `1000:1000` with restart policy `unless-stopped`.
`startup.command` is optional extra argv after the image. Use `[run]` for jobs only (no wiki). Use `[up, --hostname, 0.0.0.0]` when the image has code-server and you publish port 8080. Empty keeps the image default (`up`).
Running `up` again leaves a running container unchanged or starts a stopped container.
Use `--dry-run` on `up` or `down` to print the Docker or Podman commands.

`down` stops and removes the container.
It removes the network when no container still uses it.
`status` reports `running` or `not-running` and succeeds for a missing container.
`ls` finds directories with an `agent.yaml` under the current directory.

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
- `mounts` is a list of bind mounts or tmpfs masks. A bind mount has `source` and `target`. Set `ro: true` for read-only access. A tmpfs mask has `target` and `tmpfs: true`, with no `source`.
- `ports` is an optional list of Docker-style port mappings, such as `127.0.0.1:8080:8080`.
- `network.name` is the agent's Docker network.
- `network.mode` defaults to `open`. `locked` is reserved for network enforcement.
- `privileges.cap_add` is an optional list of Linux capabilities. The container still runs as a non-root user.
- `startup.restart` defaults to `unless-stopped`.
- `startup.env` is an optional list of host environment variable names to pass through. Do not put secret values in this file.
- If `home/.env` exists, `up` passes it to Docker as `--env-file`. That file is gitignored with the rest of `home/`. Put long-lived tokens there (`HASS_TOKEN=...`).
- `jobs` points to a cron jobs file (default `jobs.yaml`). `up` mounts it at `/workspace/jobs.yaml` and sets `IUGUM_JOBS`.
- `shm_size` is Docker `/dev/shm` size (example `1g`). Chromium needs this.
- `extra_hosts` is a list of Docker `--add-host` entries. Empty defaults to `host.docker.internal:host-gateway` so Linux containers can reach services on the host.

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
