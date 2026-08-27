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
- `jobs` optionally points to a cron jobs file.

The starter `home/policy.csv` matches iugum's current Casbin allow-all policy.
Replace its allow row with narrower rules when an agent needs restrictions.
