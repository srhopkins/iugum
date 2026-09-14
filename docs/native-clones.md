# Named native agent clones

`iugum agent clone --config /private/chief/agent.yaml --name chief-next --output /private/chief-next` creates a private configuration experiment. The destination must be new. Instructions and exact policy actor tokens are copied; secret references stay references. Unknown YAML fields survive.

Clones use their own data and commitment directories. Their `usage_path` points to the parent's ledger, so all Chief experiments count toward the same provider pools. The shared ledger locks concurrent writes. An embedded wiki is not started on the parent's occupied port.

In Chief chat, `clone_create` also snapshots the configured `code_repo` into a detached Git worktree and copies authorized typed memories into the clone's namespace. Source snapshots include uncommitted changes without committing them. The candidate directory must be outside its parent repository. Memory is a real independent snapshot; copy-on-write remains a future extension.

`clone_read` and `clone_write` operate on candidate source files. `clone_evaluate` records Go test results bound to the exact source content. Paths cannot escape the candidate or follow symlinks. Tests execute code with the host's development permissions; a worktree is not an operating-system sandbox.

`clone_request_promotion` creates a review in Chief's approval panel. Promotion checks the evaluation, candidate content, and unchanged parent baseline again. Backups are written before applying files; failed application attempts restore previous content. No commit, push, deployment, or automatic restart occurs. An edited parent requires a fresh candidate or explicit reconciliation.

The CLI command alone creates the configuration experiment. The chat tool adds source and memory snapshots. Advanced automatic evaluation-based promotion remains intentionally unavailable: a human must approve.
