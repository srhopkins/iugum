---
name: public-audit
description: >-
  Audit staged or working-tree files before a public iugum commit. Run the
  deterministic tools first (gitleaks + personal regex). Then judge leftover
  personal comments. Use before commit or when Steve says public-audit.
---

# public-audit

iugum is a public repo. Do not put secrets or personal machine content in a commit.

## Run the tools first

From the repo root:

```bash
scripts/public-audit.sh --staged --no-agent
```

`--all` scans the working tree (pre-push and CI).

| Tool | What it catches | Result |
|------|-----------------|--------|
| [gitleaks](https://github.com/gitleaks/gitleaks) | Keys, tokens, PEM, `.pw` files | BLOCK |
| `scripts/public-warn.patterns` via `rg` | Home paths, private email, LAN IPs | WARN |

Do not rely on a prompt instead of these tools.

## Then judge leftovers

If the script prints WARN, fix or drop those lines.
Look for comments that name a person, a home path, or a private host.
Those are not secrets. They still do not belong in the public tree.

A secret that the tools missed is a BLOCK. Name the file. Do not print the secret.

## Hook

`scripts/install-hooks.sh` sets `core.hooksPath` to `.githooks`.
The pre-commit hook runs `public-audit.sh --staged`.
The residual agent pass runs only when there is a WARN, or when `IUGUM_AUDIT_AGENT=1`.
Set `IUGUM_AUDIT_AGENT=0` to skip the agent.
