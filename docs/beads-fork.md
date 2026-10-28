# Beads fork

iugum uses a private fork of Beads. The fork replaces the old vendored `beads/` folder.

| Item | Value |
|---|---|
| Repo | `github.com/srhopkins/beads-iugum` (private) |
| Branch | `iugum` |
| Base commit | upstream `b2b153b7b834` (Beads `HEAD-b2b153b`, 2026-07-19) |
| Schema version | 55 (`0055_move_leases_to_table`) |
| Hook in iugum | `adapter/tracker/beadsadapt` |

Root `go.mod` points at the fork:
`replace github.com/steveyegge/beads => github.com/srhopkins/beads-iugum <pseudo-version>`.
Import paths stay `github.com/steveyegge/beads/...`.

## What the fork changes

- `cmd/bd` is `package bdcmd`, with a thin `cmd/bdmain` as the real `main`.
- `Execute`, `RunWeb`, `MemoryHook` and `SetMemoryHook` are exported.
- `remember`, `recall`, `forget`, `memories` and `prime` use the memory hook. iugum sets it to its SQLite store (`adapter/memory/sqlitemem`).
- The Dolt "start a server" hint is not printed for an external server.
- `bd web` serves the viewer (`beadview/` Go package, React UI in `beadview/web/`). `iugum beadview` runs it. Flags are unchanged.

## Schema rule

The fork base must match the `bd` binary that writes the `.beads` database. A newer `bd` migrates the schema. An older base then fails on missing columns. Keep the fork on schema 55 until every `bd` that touches the trackers is upgraded.

## Take upstream fixes

1. In the fork clone, `git fetch upstream` and merge the wanted upstream commit into `iugum`. Check the schema version first (the rule above).
2. Run `make test`, then push `iugum`.
3. In iugum, run `GOPRIVATE=github.com/srhopkins/* go get github.com/srhopkins/beads-iugum@iugum`, copy the new pseudo-version into the `replace` line, and run `go mod tidy`.
4. `scripts/build.sh --cgo`, then check `iugum beads list` and `iugum beadview`.

## Private module access

Set `GOPRIVATE=github.com/srhopkins/*` so Go skips the public proxy and checksum database. Git needs SSH access to the repo. A Docker build or CI job needs the same access. For local work you can use `replace github.com/steveyegge/beads => ../beads` (the fork clone) instead.

The viewer accepts `iugum beadview --viewer-config FILE` for optional shared YAML lane defaults (`lanes:`, with `status`, optional `label`, and `hidden`). List order sets column order. Status catalogs come from each project's `bd statuses --json`; browser Board lanes preferences override the YAML and persist in localStorage. Hidden and unknown tickets remain accessible in Other, which cannot be a drag destination for status writes. See the fork's `beadview/README.md` for a full example. Building this change requires the active local Beads fork replacement; install and restart the viewer only when requested.

The dependency-readiness fix in the local fork reads `bd blocked --json`
alongside the cached issue list. The board, status filters and counts classify
dependency-blocked open work as Blocked while preserving its stored status.
Closing a blocker returns its dependent work to Ready on refresh. This includes
inherited blockers and uses Beads' own rules. The source change requires a fork
version update or a local module replacement before it is included in an iugum
build; it does not update a running viewer.
