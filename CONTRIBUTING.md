# Contributing to iugum

iugum is a yoke: one static Go binary with opinionated defaults, and a small public contract so the yoke can hold other tools.

`CGO_ENABLED=0`. Do not add CGo. Do not use Go's `plugin.Open` (shared-object plugins). Those break a single static binary.

## Decisions

1. **The contract is the only API.** Implement `contract.Tracker`, `contract.Wiki`, or `contract.Observer` in package `github.com/srhopkins/iugum/contract`. If a change does not fit those interfaces, propose an interface change first. Do not special-case a new backend in `main.go`.

2. **Defaults are compiled in.** This binary ships:
   - tracker: **beads** (same CLI as `bd`)
   - wiki: **SilverBullet** (embedded binary)
   - observe: **memory** now; **sqlite + uPlot** is the planned default (`iugum-9n8`)

   A fork that wants a different compiled stack copies `defaults/defaults.go` and changes the blank imports.

3. **Registration is how you compile an alternative in.** In `init()`, call `plugin.RegisterTracker`, `RegisterWiki`, or `RegisterObserver`. Then `import _ "your.module/adapter"` from your `main` or `defaults` package. iugum looks adapters up by the name in the config file.

4. **Config is how you link something external.** File search order: `$IUGUM_CONFIG`, `./iugum.yaml`, `~/.config/iugum/config.yaml`. Set `tracker`, `wiki`, or `observe` to `exec` and fill `exec.<slot>` with a command. That process must uphold the same contract as the in-process adapter (see `iugum.example.yaml`). Config does not load arbitrary shared libraries.

5. **Casbin is the gate.** Package `policy` wraps [Casbin](https://casbin.org/) (an access-policy engine). `app.App` calls `Gate.Enforce` before every tracker, wiki, and observe action. The embedded model allows `* / * / *`. That is inert on purpose. When you add real rules, put a model and a policy file in config. Do not add a second permission check beside the gate. New features take a `sub, obj, act` and go through `App.Check`.

6. **Vendored trees stay upstream-shaped.** Edit `beads/cmd/bd` only to export `Execute`. Do not patch SilverBullet. Adapter wiring lives under `adapter/`, `app/`, and `main.go`.

## Add an in-process adapter

1. Create `adapter/<slot>/<name>/`.
2. Implement the interface from `contract`.
3. `init()` → `plugin.Register…("name", factory)`.
4. Blank-import it from `defaults/defaults.go` if it belongs in this binary.
5. Document the name in `iugum.example.yaml`.

## Add an external adapter

Ship a binary that speaks the exec contract for that slot. Users set `tracker: exec` (or wiki/observe) and `exec.tracker: ["your-bin"]`. No compile required. The contract still applies: same verbs, same JSON or CLI shape.

## Policy objects and actions (today)

| obj | act | when |
|-----|-----|------|
| `tracker` | `run` | `iugum beads …` |
| `wiki` | `serve` | `iugum wiki …` |
| `observe` | `ingest` | metric/log write |
| `observe` | `query` | metric/log read |

Add new rows here when you add commands. Use the same `obj`/`act` strings in tests.

## Build

```bash
CGO_ENABLED=0 go build -o iugum .
```

Run `iugum --help` and `iugum beads --help` before you open a pull request.
