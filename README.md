# iugum

**iugum** /ˈjuː.ɡum/ · *YOO-gum* — Latin **yoke**: the bar that joins a pair so they pull as one. It is the joining piece of the **harness**.

This program is a harness. It is one static Go file. Slots are replaceable. Defaults are opinionated. A config file can point a slot at another adapter or at `exec`.

Current defaults:

- **Tracker — Beads.** Issue graph and CLI. Same commands as `bd`.
- **Wiki — SilverBullet.** Markdown wiki server, embedded in this file.
- **Metrics — observe.** `iugum observe` serves sqlite + uPlot. PromQL is the query language. Config `observe: memory` stays for tests.
- **Logs — observe.** Separate sqlite file (`observe-logs.db`). LogQL is the query language. FTS5 word search.
- **Policy — Casbin.** Every command hits the gate first. The default model allows all. A deny row locks one action (example: `schedule, add`).
- **Memory — SQLite.** Facts, FTS5 word search, optional embeddings, namespaces, and a glossary graph. Tickets stay on Dolt.
- **Jobs — go-cron.** Schedule, adhoc `@triggered`, or hook. `iugum job` adds jobs to `jobs.yaml`. File watch uses fsnotify. HTTP `POST /hooks/{name}` listens when `hook_http` is set. HMAC uses `IUGUM_HOOK_SECRET`. `kind: session` jobs take a per-job `timeout` (default 4h) and `idle_timeout` (default 10m) stall watchdog — see `docs/agent.md`.
- **Ship — prepare-pr.** Writes a review markdown file and a script. Does not push. First push or `gh pr create`.

More slots can join the same file. The contract stays the same.

## North stars

iugum is for agents first.
The same slots must stay readable to a person: a human must understand what the agent is doing, and must be able to operate the same commands.
The product is one Go program.
The product uses languages that agents know.
Function has priority. Program size is last.

New work is Go, or the new work becomes part of this one file.
The default build uses CGO so Beads can open the embedded Dolt store. A static (`CGO_ENABLED=0`) build still compiles; `iugum beads` then needs Dolt server mode.
Metrics search uses **PromQL** (Prometheus query language).
Log search uses **LogQL** (Grafana Loki).
The tracker is `bd`. The wiki is SilverBullet. Do not make a local dialect.
Keep the program small enough to get and start in each location.
Do not stop a function to save bytes.

Full text and STE deviations: [NORTHSTARS.md](NORTHSTARS.md).

## Build and install

```bash
scripts/build.sh --cgo      # default: CGO_ENABLED=1, embedded Dolt works
scripts/build.sh --static   # CGO_ENABLED=0, static program
scripts/install.sh          # builds (--cgo) and installs
```

The CGO build opens the Beads embedded Dolt database (`iugum beads list`).
It links ICU (Unicode C library). Install ICU first:

- macOS: `brew install icu4c` (the script finds the keg-only prefix).
- Debian/Ubuntu: `sudo apt-get install libicu-dev g++ pkg-config`.
- Fedora: `sudo dnf install libicu-devel gcc-c++`.

Manual form: `CGO_ENABLED=1 CGO_CPPFLAGS="-I$(brew --prefix icu4c)/include" CGO_LDFLAGS="-L$(brew --prefix icu4c)/lib" go build -o iugum .`

Bare `go vet ./...` and `go test ./...` compile the same C code.
One-time macOS setup so they find ICU: `go env -w CGO_CPPFLAGS="-I$(brew --prefix icu4c)/include" CGO_LDFLAGS="-L$(brew --prefix icu4c)/lib"`.

`--static` (`CGO_ENABLED=0`) makes a static program with no C libraries.
In that build `iugum beads` needs Dolt server mode (`bd init --server`).
CGO here is a deliberate, temporary deviation. See [NORTHSTARS.md](NORTHSTARS.md) star 1.

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

This is the tracker CLI. It is not a daemon.

```bash
iugum beads …
```

`iugum beads` is the same as the standalone `bd` CLI: one command, then exit.
Arguments after `beads` pass through with no change.
`~/.local/bin/bd` is a shim that runs `iugum beads`.

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

To run a build of the vendored SilverBullet source instead of the embedded
binary, set `IUGUM_WIKI_SB_SRC` (or `IUGUM_WIKI_SB_BIN` for a binary you already
have). See `docs/silverbullet-vendor.md`.

Every space also gets the atomdown assets the wiki binary carries: the two plug
bundles, and the library page with the header button and the card CSS. Nothing
is copied into the space folder. The assets are
compiled into the SilverBullet binary as a read-only layer under the space, so
they cannot be lost or deleted, and a page you write with the same name still
wins. See `docs/wiki-space-assets.md`.

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
`beads/` carries a small patch set (package rename, `Execute` export, memory hook). See `docs/beads-vendor.md`. Re-vendor with `scripts/vendor-beads.sh <version>`.
`silverbullet/` is a git subtree of `srhopkins/silverbullet`, pinned to the upstream `2.10.0` tag and unpatched. See `docs/silverbullet-vendor.md`.
Feature beads live in this repo (prefix `iugum`).
