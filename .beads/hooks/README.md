# Global git hooks (factory)

Installed via `ayo factory install-hooks` → `git config --global core.hooksPath` points here (`~/projects/scripts/git-hooks`).

| Hook | Behavior |
|------|----------|
| `commit-msg` | Factory repos: bead DoD + namespaced audit |
| `pre-commit` | Path guards if the repo has `.guards.json` |
| `pre-push` | Guards + factory checks + declared test command |

Soft-skip repos without `.beads` / `.ao-factory`.

Canonical engine: [`../factory/`](../factory/).
