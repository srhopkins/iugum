# Disposable Beads hook audit

`iugum bead-hook-audit PRIVATE_LOG_DIR HOOK [ARGV...]` appends one input record.
The command reads stdin without interpreting it. It does not call Beads, launch
commands, initialize the app, or make network requests.

The pinned Beads fork is
`github.com/srhopkins/beads-iugum@v0.0.0-20261015101231-4c8f385341a9`.
Its `internal/hooks/hooks.go:eventToHook` accepts exactly these lifecycle events:

| Event | Executable name |
|---|---|
| `create` | `on_create` |
| `update` | `on_update` |
| `close` | `on_close` |

Beads passes the issue ID and event as arguments. Stdin contains a marshaled
issue snapshot. Git hooks, AI platform hooks, formula steps, and iugum's internal
tracker integrations use separate APIs. They do not add lifecycle executable
names to this mapping. This inventory describes the pinned fork, not upstream's
latest release or a different installed container build.

The logger stores timestamp, actual hook name, argument array, and complete stdin
in `events.jsonl`. `stdin_encoding` is `base64`; decode `stdin_base64` to recover
the exact received bytes, including whitespace and invalid UTF-8. Memory use does
not grow with the body. A temporary file holds each record before a locked append.

The directory must belong to the current user with mode `0700`. The log must be
a regular, singly linked file owned by that user with mode `0600`. The logger
rejects a symlink at the directory or log path. Its caller must supply a trusted
directory ancestry. No path comes from the body or Beads arguments. Interrupted
or failed disk writes can leave a partial final record; this is a diagnostic log,
not a transaction journal. The integration test treats malformed records as errors.

## Test scope

No lifecycle hook is installed in a live project by this work. The opt-in test
creates all three wrappers only in an explicitly supplied disposable fixture.
It refuses to replace any existing lifecycle executable. Existing Git hooks are
untouched. Do not point this test at a real tracker.

Build a test binary with `CGO_ENABLED=0 go build -o /tmp/iugum-hook-audit .`.
Create a new directory with `mktemp -d`. In that directory, initialize the fixture
with a CGO Beads binary using `bd init --prefix hooktest --skip-hooks --skip-agents
--non-interactive`. The static logger does not open a database.

From the iugum source directory, run:

```sh
IUGUM_AUDIT_FIXTURE=/absolute/disposable/directory \
IUGUM_AUDIT_BINARY=/tmp/iugum-hook-audit \
go test ./hookaudit -run TestRealCLIEvents -v
```

The test writes `matrix.json` in the fixture. Each operation inherits its own
private log directory under `.beads/hook-audit/`. This keeps late asynchronous
deliveries associated with their originating operation. The matrix records
observed counts after bounded waits; it does not promise reliable delivery.

In this pinned embedded backend, claim-only updates bypass the hook decorator.
A claim with other fields fires `on_update`. `update --status closed` fires
`on_update`, while `close` fires `on_close`. Initial labels and dependencies can
produce multiple update snapshots. Snapshot arrival order is not guaranteed.
The runner launches a goroutine without waiting during CLI exit. The proxied
backend has separate synchronous call sites and needs separate validation.
