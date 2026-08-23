# iugum north stars

iugum is a tool set for agents first.
A person interface is a translation layer.
An agent must know how to operate each slot.

These three rules filter each product decision.
The sequence is the priority.

## 1. One Go program

New work is Go.
Or the new work becomes part of this one static Go program (`CGO_ENABLED=0`).
One other language is permitted only if the result is one file.
One other procedure is permitted only if the result is one file.
The public contract must stay the same.

Do not add a program that the operator must install adjacent to iugum.

## 2. Agent-known syntax, not a local dialect

Agents select tools that the agents know.
iugum uses those languages.
Do not make a local dialect for a slot that has a known syntax.

Metrics search uses PromQL (Prometheus query language).
Example: `junction_c{gpu="mi50"}`.

Log search uses LogQL (Grafana Loki).
Example: `{stream="homelab"} |= "error"`.

The tracker uses the same CLI as `bd`.
The wiki uses the same flags as SilverBullet.

A **plugin** is a compiled Go adapter (`plugin.Register…`).
A **skill** is a folder with `SKILL.md` plus an optional command.
Do not invent a third word (recipe, playbook) for those two.

PromQL is for metrics only.
Log search uses LogQL, not PromQL.
LogQL uses the same label braces, then a line match.

The person layer translates.
The person layer does not replace these languages.

The memory observer filters by name, stream, words, and time.
PromQL and LogQL are the search syntax for agents.
Use that syntax when the observe slot expands.
See `iugum-9n8` — "observe: SQLite + uPlot metrics and logs in one binary".

## 3. Function first, then program size and speed

Keep the program small and light.
Then an operator can get the program, start the program, and get the same result.
Program size and speed are last.
Do not stop a function because the program becomes larger.
Do not stop a larger Go dependency because the program becomes larger.
If the function agrees with the first two stars, the function has priority.
The function must stay in this file.

---

## STE deviations

This document uses Simplified Technical English (ASD-STE100) as a style.
This is not an ASD approval.

We keep these deviations on purpose.

1. Product names and identifiers stay as written. Examples: iugum, Beads, SilverBullet, PromQL, LogQL, Go, Casbin, SQLite, uPlot, `bd`.
2. Code examples stay as code. Do not rewrite code to STE.
3. "Go" is the name of the programming language, not the verb.
4. The name line in `README.md` may keep the Latin word "yoke" and the industry word "harness". Do not add grammar tables.
5. The policy table in `CONTRIBUTING.md` stays as a table. Tables help a person scan. The checker reads a table as one long word group.
6. The Beads sections in `AGENTS.md` come from a tool. Do not rewrite those sections.
7. Technical nouns live in `glossaries/iugum.yaml`. This checker uses that list.
8. The name "Simplified Technical English" and file paths (`README.md`, `AGENTS.md`, `iugum.yaml`) stay as written. The checker flags those strings as unknown words.

Use the CLI command `ste100 check` with `glossaries/iugum.yaml`.
The package is `asd-ste100-checker` on GitHub (`sourdough-bread/asd-ste100-checker`).
