---
name: prepare-pr
description: >-
  Write a reviewable markdown file and a script that publishes git work.
  Use when Steve says prepare a PR, first push, or prepare-pr.
---

# prepare-pr

Do not push. Do not run `gh pr create`. Write files, print paths, stop.

```bash
iugum skill run prepare-pr --repo DIR --title "…"
# same command:
iugum prepare-pr --repo DIR --title "…"
```

Body: stdin, `--body-file`, or `--body`.

| Remote | Files | Script does |
|--------|-------|-------------|
| No heads on `origin` | `push.md` + `push.sh` | `git push -u origin HEAD` |
| Base branch exists | `pr.md` + `create.sh` | `gh pr create --body-file pr.md` |

Files land in `<repo>/.iugum/prepare/<stamp>/` (gitignored).

Show Steve the markdown. Run the script only when he says go.
