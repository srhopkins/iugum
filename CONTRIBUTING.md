# Contributing to iugum

iugum is a harness.
iugum is one static Go program with default adapters.
iugum has a small public contract so the harness can hold other tools.

Read [NORTHSTARS.md](NORTHSTARS.md) before you add a dependency or a query language.

1. **Go or one file.** New work is Go, or the new work becomes part of this one program. Do not add a sidecar stack.
2. **Agent-known syntax.** Metrics: PromQL. Logs: LogQL. Tracker: `bd`. Wiki: SilverBullet. Do not make a local dialect.
3. **Function first.** Keep the file small when you can. Do not stop a function to save bytes.

After clone, run `scripts/install-hooks.sh`. Git does not install hooks for you.

The pre-commit hook runs `scripts/public-audit.sh --staged`:

- **Block:** [gitleaks](https://github.com/gitleaks/gitleaks) (keys, tokens, PEM, `.pw` files). Config: `.gitleaks.toml`.
- **Warn:** `scripts/public-warn.patterns` (home paths, private email, LAN IPs). Fix the hit when it applies. Do not block the commit.
- **Agent:** residual pass only after a WARN, or when `IUGUM_AUDIT_AGENT=1`. The agent cannot waive a gitleaks block.

Install gitleaks with `brew install gitleaks`. Agents: `skills/public-audit/SKILL.md`.

The default build is CGO for the tracker's embedded Dolt store. Do not add new CGo.
CGo is permitted in two places only: the tracker's embedded Dolt store (`beads/`, needs ICU) and the sqlite-vec path.
`CGO_ENABLED=0` stays a secondary static build (beads then needs Dolt server mode). See NORTHSTARS.md star 1.
Do not use Go `plugin.Open` (shared-object plugins).
Those plugins break a single static program.

## Decisions

1. **The contract is the only API.**
   Implement `contract.Tracker`, `contract.Wiki`, or `contract.Observer` in package `github.com/srhopkins/iugum/contract`.
   If a change does not match those interfaces, propose an interface change first.
   Do not special-case a new backend in `main.go`.

2. **Defaults are compiled in.**
   This program includes three defaults.
   The tracker is **beads**. The CLI is the same as `bd`.
   The wiki is **SilverBullet**. SilverBullet is an embedded program.
   The observe slot in config is **memory** (in-process).
   `iugum observe` uses **sqlite + uPlot** (`observe: sqlite` selects that store for App).

   A fork that wants a different compiled stack copies `defaults/defaults.go`.
   Then edit the blank imports.

3. **Registration compiles an other adapter in.**
   In `init()`, call `plugin.RegisterTracker`, `RegisterWiki`, `RegisterObserver`, or `RegisterMemory`.
   Then `import _ "your.module/adapter"` from your `main` or `defaults` package.
   iugum finds adapters by the name in the config file.

4. **Config links an external program.**
   File search order: `$IUGUM_CONFIG`, `./iugum.yaml`, `~/.config/iugum/config.yaml`.
   Put `tracker`, `wiki`, or `observe` to `exec`.
   Fill `exec.<slot>` with a command.
   That procedure must use the same contract as the in-process adapter.
   See `iugum.example.yaml`.
   Config does not load arbitrary shared libraries.

5. **Casbin is the gate.**
   Package `policy` wraps [Casbin](https://casbin.org/) (an access-policy engine).
   `app.App` calls `Gate.Enforce` before each tracker, wiki, observe, and memory action.
   The embedded model permits `* / * / *`.
   That model is inert on purpose.
   When you add real rules, put a model and a policy file in config.
   Do not add a second permission check adjacent to the gate.
   New functions take a `sub, obj, act` and go through the App gate.

6. **Vendored trees stay in the upstream form.**
   Edit `beads/cmd/bd` only to export `Execute` and to keep the memory hook. The patch set is in `scripts/vendor/beads-patches/`; re-vendor with `scripts/vendor-beads.sh <version>` (see `docs/beads-vendor.md`).
   Do not edit SilverBullet.
   Adapter wiring lives under `adapter/`, `app/`, and `main.go`.

## Add an in-process adapter

1. Create `adapter/<slot>/<name>/`.
2. Implement the interface from `contract`.
3. In `init()`, call `plugin.Register…("name", factory)`.
4. Add a blank import in `defaults/defaults.go` if the adapter belongs in this program.
5. Document the name in `iugum.example.yaml`.

## Add an external adapter

Ship a program that uses the exec contract for that slot.
Operators put `tracker: exec` (or wiki or observe) and `exec.tracker: ["your-bin"]`.
No compile is required.
The contract continues to apply: same verbs, same JSON or CLI form.

## Policy objects and actions (today)

| obj | act | when |
|-----|-----|------|
| `tracker` | `run` | `iugum beads …` |
| `wiki` | `serve` | `iugum wiki …` |
| `observe` | `ingest` | metric/log write |
| `observe` | `query` | metric/log read |
| `observe` | `serve` | `iugum observe …` |
| `ship` | `prepare` | `iugum prepare-pr` / `iugum skill run prepare-pr` |
| `mem/{type}/ns/{path}` | `read` | recall, search, walk |
| `mem/{type}/ns/{path}` | `write` | remember, ingest |
| `mem/{type}/ns/{path}` | `attach` | attach a namespace to a row |
| `mem/{type}/ns/{path}` | `slice` | copy bindings that match a filter |
| `mem/{type}/ns/{path}` | `detach` | drop a binding |
| `mem/{type}/ns/{path}` | `grant` | reserved for later policy |
| `hook` | `fire` | local hook |
| `hook` | `http` | POST /hooks/{name} when hook_http is set |
| `schedule` | `run` | cron or adhoc job |
| `service` | `serve` | `iugum up` host mode start |
| `container` | `run` | `iugum up --container` |
| `container` | `build` | `iugum container build` |
| `container` | `stop` | `iugum container stop` |
| `agent` | `run` | `iugum agent up` |
| `agent` | `stop` | `iugum agent down` |
| `agent` | `status` | `iugum agent status` |
| `agent` | `ls` | `iugum agent ls` |
| `agent` | `tui` | `iugum agent tui` |
| `agent` | `acp` | `iugum agent acp` |
| `agent` | `checkpoint` | `iugum agent checkpoint` |
| `net` | `plan` | `iugum net plan` (render rules, no change) |
| `net` | `apply` | `iugum net apply [--dry-run]`, `iugum up` at start |
| `net` | `show` | `iugum net show` (live ruleset) |

The default Casbin model allows all. Object uses `keyMatch`, so `mem/*/ns/steve/*` is a later deny/allow rule.
Graph split words live in `glossaries/memory-graph.yaml` (same shape as the STE glossary).

Add new rows here when you add commands.
Use the same `obj`/`act` strings in tests.

## Build

```bash
scripts/build.sh --cgo      # default: CGO_ENABLED=1, embedded Dolt works (needs ICU)
scripts/build.sh --static   # CGO_ENABLED=0, static program, beads needs server mode
```

ICU: macOS `brew install icu4c`; Debian/Ubuntu `apt-get install libicu-dev g++ pkg-config`.
`CGO_ENABLED=0 go build -o iugum .` must keep compiling.

Run `iugum --help` and `iugum beads --help` before you open a pull request.
