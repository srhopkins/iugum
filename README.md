# iugum

**iugum** /ˈjuː.ɡum/ · *YOO-gum*  
*noun* · neuter · 2nd declension · genitive **iugī**

**Latin** *iugum, iugī* — from *iungō*, “to join, to yoke.”  
Cognate with Greek *ζυγόν* (zugón) and English *yoke*.

1. A **yoke**: the bar that joins a pair so they pull as one.
2. A **pair** or **team** (oxen, horses).
3. A **ridge**: the line where two slopes meet.
4. *Transferred.* One static Go program that joins **Beads** (the issue tracker CLI, same as `bd`) and **SilverBullet** (the markdown wiki). The operator holds one file.

---

One static Go program that includes **Beads** (issue tracker CLI) and **SilverBullet** (markdown wiki server).

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
```

`CGO_ENABLED=0` makes a static program with no C libraries.

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

## Adapters

Built-in slots: beads, SilverBullet, in-memory observe.
A config file can put a slot to `exec` (an external program that uses the same contract).
Casbin (policy engine) runs before each slot.
The default policy permits all.

See [CONTRIBUTING.md](CONTRIBUTING.md) and [iugum.example.yaml](iugum.example.yaml).

After clone, run `scripts/install-hooks.sh`. The hook blocks secrets ([gitleaks](https://github.com/gitleaks/gitleaks)) and warns on personal paths.

## Repo status

Install: copy `iugum` onto your `PATH`.
Vendored upstream trees: `beads/`, `silverbullet/`.
You can export `Execute` from `beads/cmd/bd` only.
Feature beads live in this repo (prefix `iugum`).
