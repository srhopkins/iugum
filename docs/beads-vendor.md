# Vendored Beads tree

`beads/` is a copy of the upstream Beads module (`github.com/steveyegge/beads`).
Root `go.mod` points the module at this copy with `replace github.com/steveyegge/beads => ./beads`.
`iugum beads ...` runs the Beads CLI in-process through this copy.

## Upstream commit

| Item | Value |
|---|---|
| Upstream commit | `b2b153b7b834` (Beads `HEAD-b2b153b`, 2026-07-19) |
| Go module version | `v1.1.1-0.20260719023420-b2b153b7b834` |
| Schema version | 55 (`0055_move_leases_to_table`) |
| Matches | Homebrew `bd` (`bd version` prints `HEAD-b2b153b`) |

The vendored commit must match the `bd` binary that writes the `.beads` Dolt database.
A newer `bd` migrates the database schema. An older vendored tree then fails on missing columns.
Example: schema 54 code reads `issues.lease_expires_at`; schema 55 moved that column to a table.

## Local patches

The vendored tree has five changes from upstream. Everything else is upstream form.

1. **Package rename.** Every `beads/cmd/bd/*.go` file says `package bdcmd` instead of `package main`.
   Reason: iugum imports the CLI as a library. Only the top-level `cmd/bd` directory changes. Sub-packages (`doctor`, `protocol`, `setup`) stay as they are.
2. **`Execute` export** (`scripts/vendor/beads-patches/main.patch`).
   `func main()` in `beads/cmd/bd/main.go` becomes `func Execute()`.
   `adapter/tracker/beadsadapt` calls it.
   The same patch adds `memories`, `recall`, `remember`, `forget` to the no-database command list.
   It also changes the subcommand check in the root `PersistentPreRunE`. Upstream compares the parent command name with the literal `"bd"`. iugum sets `BD_NAME=iugum`, which renames the root command, so every top-level command looked nested and the no-database list never applied (`bd init` failed with "no beads database found"). The patch checks `cmd.Parent().HasParent()` instead.
3. **Memory hook** (`scripts/vendor/beads-patches/memory.patch`).
   `beads/cmd/bd/memory.go` gets `MemoryHook`, `SetMemoryHook`, and `memoryGet` / `memorySet` / `memoryDelete` / `memoryList`.
   The `remember`, `recall`, `forget`, `memories` commands call these helpers.
   With a hook set, the commands skip Dolt and skip the direct-mode and proxied-server guards.
   iugum sets the hook to its SQLite memory store.
4. **Prime memory hook** (`scripts/vendor/beads-patches/prime.patch`).
   `formatMemoriesForPrime` in `beads/cmd/bd/prime.go` reads memories from the hook when one is set.

`beads/go.mod` is unchanged. Root `go.mod` carries the upstream version string on its `require` line.

## Re-vendor procedure

One command. Pass the module version that matches your `bd` binary:

```sh
scripts/vendor-beads.sh v1.1.1-0.20260719023420-b2b153b7b834
scripts/build.sh --cgo
./iugum beads list
```

The script does these steps:

1. `go mod download` the version (a bare commit hash also works).
2. Copy the module tree over `beads/` with write permission.
3. Rename `package main` to `package bdcmd` in `beads/cmd/bd/*.go`.
4. Apply `scripts/vendor/beads-patches/*.patch` with `patch -p1`.
5. Update the `require` line in root `go.mod`, run `go mod tidy`, `gofmt`, and a `CGO_ENABLED=0` build.

Then update the table above with the new commit and schema version.

### When a patch does not apply

Upstream moved the code the patch touches.

1. Open the file under `beads/cmd/bd/` and make the change by hand. Keep the same names (`Execute`, `MemoryHook`, `SetMemoryHook`, `memoryGet`, `memorySet`, `memoryDelete`, `memoryList`).
2. Regenerate the patch from the module cache copy:

```sh
UP=$(go env GOMODCACHE)/github.com/steveyegge/beads@<version>
f=memory   # or main, prime, where
diff -u <(sed '1s/^package main$/package bdcmd/' "$UP/cmd/bd/$f.go") "beads/cmd/bd/$f.go" \
  | sed "1s#.*#--- a/cmd/bd/$f.go#;2s#.*#+++ b/cmd/bd/$f.go#" > "scripts/vendor/beads-patches/$f.patch"
```

3. Run `scripts/vendor-beads.sh <version>` again. It must finish clean.

## Build notes

The Dolt store needs ICU (Unicode C library) through `github.com/dolthub/go-icu-regex`.
`#cgo` flags in the root package do not reach that dependency, so a root `cgo_icu_*.go` file cannot supply the include path.
Use `scripts/build.sh --cgo`. It sets `CGO_CPPFLAGS` / `CGO_LDFLAGS` from `brew --prefix icu4c` on macOS or `pkg-config` on Linux.
For bare `go vet ./...` and `go test` with CGO on macOS, run once: `go env -w CGO_CPPFLAGS="-I$(brew --prefix icu4c)/include" CGO_LDFLAGS="-L$(brew --prefix icu4c)/lib"`.
Linux packages: `libicu-dev g++ pkg-config` (Debian), `libicu-devel gcc-c++` (Fedora), `icu-dev g++ pkgconfig` (Alpine).
