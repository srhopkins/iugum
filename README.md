# iugum

**iugum** /ˈjuː.ɡum/ · *YOO-gum*  
*noun* · neuter · 2nd declension · genitive **iugī**

**Latin** *iugum, iugī* — from *iungō*, “to join, to yoke.”  
Cognate with Greek *ζυγόν* (zugón) and English *yoke*.

1. A **yoke**: the bar that joins a pair so they pull as one.
2. A **pair** or **team** (oxen, horses).
3. A **ridge**: the line where two slopes meet.
4. *Transferred.* One static Go binary that yokes **Beads** (the issue tracker CLI, same as `bd`) and **SilverBullet** (the markdown wiki) so the user holds one file.

---

One static Go binary that bundles **Beads** (issue tracking CLI) and **SilverBullet** (markdown wiki server).

## Build and install

```bash
CGO_ENABLED=0 go build -o /Users/steve/bin/iugum .
```

`CGO_ENABLED=0` produces a fully static binary with no C dependencies.

## Usage

```bash
iugum --help
```

### Beads

```bash
iugum beads …
```

Runs Beads in-process — same commands and behavior as the standalone `bd` CLI. Arguments after `beads` pass through unchanged.

### Wiki

```bash
iugum wiki [space-dir]
```

Starts an embedded SilverBullet server. Default space directory is `./wiki` (current directory's `wiki` folder).

| Flag | Default | Meaning |
|------|---------|---------|
| `-p`, `--port` | `3000` | HTTP listen port |
| `-L`, `--hostname` | (SilverBullet default) | Bind hostname |

There is no `serve` subcommand — `wiki` starts the server directly.

If port `3000` is already taken, pass `--port`.

## Adapters

Built-in slots: beads, SilverBullet, in-memory observe. Config file can point a slot at `exec` (an external binary that upholds the same contract). Casbin (policy engine) runs before every slot. Default policy allows all.

See [CONTRIBUTING.md](CONTRIBUTING.md) and [iugum.example.yaml](iugum.example.yaml).

## Repo status

- Install path: `/Users/steve/bin/iugum`
- Vendored upstream trees: `beads/`, `silverbullet/`. The only beads edit allowed is exporting `Execute` from `beads/cmd/bd`.
- Feature beads live in this repo (prefix `iugum`).
