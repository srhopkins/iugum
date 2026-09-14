# Local session retrieval

This package builds a derived SQLite FTS5 index in the iugum process. Native transcripts remain the evidence source. `Hit.Path` and `Hit.Line` point to JSONL evidence; OpenCode hits identify a database and part ID. Account labels come from configuration, never guessed from transcript paths.

```go
index, err := agentsessions.Open("/private/agent-home/session-index.sqlite")
// Handle err, and defer index.Close().
report, err := index.Sync(ctx, allowedSources, agentsessions.Limits{})
hits, err := index.Search(ctx, agentsessions.Query{
    Text: "ticket delivery", Scope: "ffai", Limit: 10,
    Authorize: authorizeCurrentPolicy,
})
```

The host must authorize each source before indexing. It must supply `Query.Authorize` to enforce current per-hit access rules, and recheck authorization before opening evidence. A denied result does not consume the requested result limit. Keep the index in the agent's private directory; it contains transcript text. Scope narrows results; the caller can explicitly retry a broader authorized scope when necessary. Exclusions and platform filters are supported. Search words are literal AND terms, not executable FTS syntax.

## Coverage and limits

- Claude: JSONL message content, session ID, project, timestamps. Configure additional Claude account roots explicitly.
- Codex: session metadata, response message text, user/agent event messages. Some native events repeat message text, so hits may duplicate.
- Cursor: canonical SQLite bubble rows including orphan/unlisted bubbles, message text, thinking, tool parameters/results, per-bubble timestamps, and workspace metadata. Exported JSONL remains supported as a fallback and can duplicate canonical hits.
- OpenCode: text parts from both native databases, with each part's creation timestamp.
- No embeddings, fuzzy matching, or semantic retrieval yet. This is the lexical baseline; test retrieval quality before claiming comprehensive recall.

Sync is incremental and atomic, with newest changed files first. It uses file size and nanosecond modification time (including SQLite WAL) to skip unchanged files. Changed files replace their owned records; deleted files and removed sources remove their records. On discovery failure, deletion pruning is deferred to avoid mistaking an inaccessible source for a deleted one. Defaults are 2,000 changed files and 100,000 new messages per refresh. JSONL streams regardless of total file size; the 32 MiB `MaxFileBytes` limit now applies to each line/bubble. Oversized lines are skipped with a warning and later lines continue. Reports expose truncation, source errors, unchanged-file counts and total searchable messages. A file exceeding the per-refresh message budget retains partial records and advances on subsequent refreshes when its watermark is unchanged. Completed prefixes are rescanned without reinsertion; additional indexed messages remain bounded per batch. A changing source resets that file’s partial snapshot safely. The transcript-lake README was inspected for its provenance and canonical Cursor-source design; no external CLI is required here.

## Claude execution

`Claude.Resume` supports a saved session through locally verified `claude --resume ID --print --output-format json`. It requires a project directory, explicit account label, account config directory, and prompt. The host must authorize dispatch first and verify the configured account actually maps to the intended credentials. The account label is descriptive, not authentication proof. Ambient provider credentials can still affect the Claude CLI's authentication selection.

The process accepts context cancellation. Native Claude permission checks remain enabled. No real work was dispatched while implementing this adapter. A fresh timestamp is prepended to the dispatched prompt; the adapter cannot guarantee a timestamp on every internal Claude tool-loop iteration.

Live steering is explicitly unsupported: local CLI help documents saved-session resume and background attach, but resuming an already-running background session can create a copy. `Steer` returns `ErrLiveSteeringUnsupported` rather than silently forking or claiming live control. Verified live control is still required to meet the full Chief MVP requirement.

## Drilldown and relaxed retrieval

`Get(ctx, hit, EvidenceOptions{Before: 2, After: 2, MaxBytes: 16384, Authorize: currentPolicy})` returns the full indexed message and nearby indexed JSONL lines. It does not open an arbitrary requested file: the reference must resolve to an indexed record. Authorization uses stored metadata, including for every nearby message. OpenCode evidence returns the indexed part without adjacent line context. Byte output is capped at 64 KiB and nearby line windows at 20 per direction. `Snapshot: true` explicitly means indexed evidence, not newly verified source contents.

Use `SearchFallback(ctx, query)` if a relaxed lexical fallback is desired. It first requires every word, then accepts any word only when no authorized results match. Display its `MatchMode` (`all_terms` or `any_terms`) so a loose hit is not presented as an exact answer. It retains all scope, exclusion, platform and policy restrictions. This is not semantic retrieval.

The local account-root discovery found only `~/.claude/projects`; no separate personal/contract configuration directory was established. Keep that source's account label empty rather than guessing. Additional roots can be configured as `Source{Platform: "claude", Root: "/path/to/account/projects", Account: "explicit-account-label"}` once their meaning is verified. Project scope can still prioritize FutureFit AI paths within co-mingled transcripts.

Canonical Cursor reads use a SQLite read transaction against the live database (including its WAL), so the scan is consistent without copying private data to a temporary file. All `bubbleId:` rows are scanned rather than trusting the incomplete composer index. Workspace paths resolve through Cursor's sibling `workspaceStorage/<id>/workspace.json`; unresolved identities are explicitly labeled `cursor-workspace:<id>` or `cursor-session:<id>`, not guessed. Native canonical provenance is `state.vscdb#cursorDiskKV/bubbleId:<session>:<bubble>`.

Incremental watermarks detect ordinary file changes, not malicious same-size/same-mtime replacement. The index is derived data and can be removed/rebuilt when stronger verification is required. Large changed databases are rescanned as a unit; row-level database watermarks are not implemented.

Disk indexes use SQLite WAL and a small read connection pool, so searches and evidence drilldown see the previous committed snapshot while a refresh is writing. Connections use a one-second busy timeout. In-memory test indexes retain a single connection to preserve their database identity.
