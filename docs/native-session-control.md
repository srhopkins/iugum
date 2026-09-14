# Saved Claude session control

Inspect implemented interfaces without sending a prompt:

```sh
iugum agent session capabilities
```

Resume a saved Claude conversation with an explicitly authorized instruction:

```sh
iugum agent session resume --session SESSION_ID \
  --project /absolute/project/path --account personal \
  --config-dir /absolute/claude/config \
  --prompt 'Continue the agreed task and report the result.'
```

This invokes the installed Claude CLI in print mode and returns JSON. It can perform work and consume the selected account's allowance. The current iugum actor must pass policy checks for `transcript:claude:personal` / `resume`, `project:/absolute/project/path` / `execute`, and `credentials:claude:/absolute/claude/config` / `use`. Claude retains its native permission checks. Account labels are operator-provided metadata, not proof of authentication: verify the CLI profile before delegation. Ambient API credentials can affect Claude's actual account selection.

The command handles cancellation through its context. A fresh timestamp is included at dispatch; timestamps inside the external CLI's own tool loop are not controlled by iugum.

Running-session steering is not implemented. The capability response says so explicitly. Resuming an already-running background session can fork it, so use this command for a saved inactive session. No automated live-session detection is claimed. Cursor, OpenCode and Codex session adapters remain unimplemented extension points in this command.
