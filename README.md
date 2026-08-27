# iugum

**iugum** /ˈjuː.ɡum/ · *YOO-gum* — Latin **yoke**: the bar that joins a pair so they pull as one. It is the joining piece of the **harness**.

This program is a harness. It is one static Go file. Slots are replaceable. Defaults are opinionated. A config file can point a slot at another adapter or at `exec`.

Current defaults:

- **Tracker — Beads.** Issue graph and CLI. Same commands as `bd`.
- **Wiki — SilverBullet.** Markdown wiki server, embedded in this file.
- **Metrics — observe.** `iugum observe` serves sqlite + uPlot. PromQL is the query language. Config `observe: memory` stays for tests.
- **Logs — observe.** Separate sqlite file (`observe-logs.db`). LogQL is the query language. FTS5 word search.
- **Policy — Casbin.** Every command hits the gate first. The default model allows all.
- **Memory — SQLite.** Facts, FTS5 word search, optional embeddings, namespaces, and a glossary graph. Tickets stay on Dolt.
- **Jobs — go-cron.** Schedule, adhoc `@triggered`, or hook. File watch uses fsnotify. HTTP `POST /hooks/{name}` listens when `hook_http` is set. HMAC uses `IUGUM_HOOK_SECRET`.
- **Ship — prepare-pr.** Writes a review markdown file and a script. Does not push. First push or `gh pr create`.

More slots can join the same file. The contract stays the same.

## North stars

iugum is for agents first.
The product is one static Go program.
The product uses languages that agents know.
Function has priority. Program size is last.

New work is Go, or the new work becomes part of this one file (`CGO_ENABLED=0`).
Metrics search uses **PromQL** (Prometheus query language).
Log search uses **LogQL** (Grafana Loki).
The tracker is `bd`. The wiki is SilverBullet. Do not make a local dialect.
Keep the program small enough to get and start in each location.
Do not stop a function to save bytes.

Full text and STE deviations: [NORTHSTARS.md](NORTHSTARS.md).

## Build and install

```bash
CGO_ENABLED=0 go build -o iugum .
scripts/install.sh
```

`CGO_ENABLED=0` makes a static program with no C libraries.

Container image (`docker build -t iugum .`, with `WITH=` to pick agent CLIs): [docs/container.md](docs/container.md).

Install puts the binary at `~/.local/bin/iugum` and `~/bin/iugum`.
`~/.local/bin/bd` runs `iugum beads`. Every `bd` command is iugum.

Config: `~/.config/iugum/config.yaml` (or `./iugum.yaml`, or `IUGUM_CONFIG`).
Data: `~/Library/Application Support/iugum` (`memory.db`, `observe-metrics.db`, `observe-logs.db`). Override with `data_dir` or `IUGUM_DATA`.

Optional nearest-neighbor search (`embeddings.vec: true`) uses **sqlite-vec** (`modernc.org/sqlite/vec`) on linux, darwin, freebsd, netbsd, openbsd, and windows (`vec.go`). Other platforms compile `vec_stub.go` and keep cosine search in Go. Vec stays off unless embeddings are on and the vec0 probe succeeds.

## Usage

Show help.

```bash
iugum --help
```

### Beads

This starts Beads.

```bash
iugum beads …
```

This command starts Beads.
The procedure is the same as the standalone `bd` CLI.
Arguments after `beads` pass through with no change.

### Wiki

This starts the wiki.

```bash
iugum wiki [space-dir]
```

This command starts the embedded SilverBullet server.
The default space directory is `./wiki`.

`-p` / `--port` default is `3000` (HTTP listen port).
`-L` / `--hostname` uses the SilverBullet default.

There is no `serve` subcommand.
The `wiki` command starts the server.

If port `3000` is in use, pass `--port`.

### Observe

This starts the metrics and logs server (sqlite file + embedded graphs).

```bash
iugum observe [--port N] [--hostname ADDR]
```

Default listen is `127.0.0.1:3848`. This port is not SilverBullet (`:3737`).
Temperatures graph in °C. Mark lines sit at 50 / 100 / 105 °C.
Ctrl+C or SIGTERM stops the server.

## Adapters

How to swap a default: [CONTRIBUTING.md](CONTRIBUTING.md) and [iugum.example.yaml](iugum.example.yaml).

After clone, run `scripts/install-hooks.sh`. The hook blocks secrets ([gitleaks](https://github.com/gitleaks/gitleaks)) and warns on personal paths.

## Repo status

Install: copy `iugum` onto your `PATH`.
Vendored upstream trees: `beads/`, `silverbullet/`.
You can export `Execute` from `beads/cmd/bd` only.
Feature beads live in this repo (prefix `iugum`).
