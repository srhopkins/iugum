/**
 * Generate `fixture/running.md`.
 *
 * WHY A GENERATOR AND NOT A COPY. The suite was specified against Steve's real
 * page, `_silverbullet/Todo/running.md` — 291 lines, 82 atoms, 11 named
 * groups, a 10-row table, ordered lists, inline code and many links. That page
 * carries client ticket titles and colleague names, and this repo has a
 * public-repo gate (CONTRIBUTING.md), so the committed fixture reproduces the
 * page's SHAPE and none of its content. Every structural fact the six rules
 * assert against is preserved on purpose:
 *
 *   - 82 atoms and 11 named groups, so a sweep's expected-count assertion is
 *     the same number it was written for.
 *   - A `decisions` group whose lead atom is one ordered list of exactly six
 *     items — the ordered-list markers that rendered left of the card border.
 *   - A `resea` group holding one 10-row table whose ticket cells are links —
 *     the wide table that crossed the card border and the group border.
 *   - One row, keyed `FFAI-62019`, whose link label contains unescaped square
 *     brackets. That closes the label early, so it is NOT a link, and plain
 *     SilverBullet renders it raw too. Rule 5 encodes it as a known-good
 *     exception rather than a failure.
 *   - Nested lists, a blockquote, a fenced code block, inline code, and a link
 *     long enough to wrap, because each is a separate containment case.
 *
 * Run it when the shape needs to change:
 *
 *     node plugs/atomdown-e2e/fixture/make-fixture.mjs
 *
 * It writes plain markdown, then shells out to the real `atomdown` binary with
 * `materialize --digest -w` so every atom carries a genuine sha256 of
 * its own content. Rule 6 runs `atomdown lint` and `atomdown verify` against
 * this file, and neither passes on a hand-faked digest.
 */

import { execFileSync } from "node:child_process";
import { existsSync } from "node:fs";
import { readFileSync, writeFileSync } from "node:fs";
import { join } from "node:path";

const OUT = join(import.meta.dirname, "running.md");

/** Locate the atomdown binary the same way rule 6 does. */
function atomdownBin() {
  const candidates = [
    process.env.ATOMDOWN_BIN,
    join(process.env.HOME ?? "", "go", "bin", "atomdown"),
    "/usr/local/bin/atomdown",
    join(
      process.env.HOME ?? "",
      "projects/github/srhopkins/atomdown/atomdown",
    ),
  ].filter(Boolean);
  for (const c of candidates) if (existsSync(c)) return c;
  try {
    return execFileSync("which", ["atomdown"], { encoding: "utf8" }).trim();
  } catch {
    throw new Error(
      "No atomdown binary found. Set ATOMDOWN_BIN to one, or build it from " +
        "the atomdown checkout. Digests cannot be faked: rule 6 runs the real " +
        "`atomdown verify`.",
    );
  }
}

const blocks = [];
const push = (text) => blocks.push(text);

/**
 * Group ids, fixed rather than generated.
 *
 * `atomdown materialize` writes ids for atoms but not for groups, and a group
 * marker without an id is a lint error. Hardcoding them keeps a regeneration
 * byte-stable, so re-running this script produces no git diff unless the shape
 * actually changed — and it lets a test name a specific group.
 */
const GROUP_IDS = {
  decisions: "KATZ94NM",
  resea: "NS67J8K5",
  editor: "QP41ZR8T",
  parser: "VD07KHM2",
  "board-view": "WX53BCN9",
  "inline-view": "YT26FGK4",
  database: "ZR89PSD1",
  delivery: "BH14QWM7",
  agents: "CJ62MVX3",
  notes: "DK75NRT8",
  backlog: "EK38HYZ6",
};

/** Open and close a named group. `slug` becomes the group's readable name. */
function group(slug, body) {
  const id = GROUP_IDS[slug];
  if (!id) throw new Error(`no fixed id for group "${slug}"`);
  push(`<!-- <atom-group id="${id}" slug="${slug}"> -->`);
  body();
  push(`<!-- </atom-group> -->`);
}

/** Filler that varies by index so no two cards are byte-identical. */
function filler(i, topic) {
  const shapes = [
    () => `Note ${i}. ${topic} holds at the current step. Nothing is blocked.`,
    () =>
      `Check ${i}. Read \`${topic}/step-${i}.md\` before the next change. The path is relative to the space root.`,
    () =>
      `- ${topic} item ${i}a\n- ${topic} item ${i}b\n  - nested ${i}b1\n  - nested ${i}b2\n- ${topic} item ${i}c`,
    () =>
      `> A quoted line for ${topic}, step ${i}. It wraps far enough to make the blockquote bar measurable against the card border.`,
    () => `### ${topic} step ${i}`,
    () =>
      "```sh\n" +
      `# ${topic} step ${i}\n` +
      `iugum wiki --port 0 ./space-${i}\n` +
      "```",
    () =>
      `1. first for ${topic} ${i}\n2. second for ${topic} ${i}\n3. third for ${topic} ${i}`,
    () =>
      `A long reference for ${topic}: [a link label that is deliberately long enough to wrap inside a narrow card and reach the right border](https://example.invalid/atomdown/fixture/reference/${topic}/step-${i}?verbose=1&trace=1) and then some trailing prose.`,
  ];
  return shapes[i % shapes.length]();
}

// ---------------------------------------------------------------------------
// Document head: 2 ungrouped atoms
// ---------------------------------------------------------------------------

push("# Running todo");
push(
  "The master list. Daily pages are filled from here, grouped by subject. " +
    "This is a generated fixture: the shape of a real page, none of its content.",
);

// ---------------------------------------------------------------------------
// Group 1: decisions — the six-item ordered list rule 5 asserts on
// ---------------------------------------------------------------------------

group("decisions", () => {
  push("## Decisions waiting on me");
  push(
    "Read this group first. Nothing below is blocked on work. It is all " +
      "blocked on an answer.",
  );
  // Rule 5: exactly one <ol> with exactly six <li>.
  push(
    [
      "1. **History.** One commit is still local and later commits reverse it. Drop it, squash the pair, or push the contradiction.",
      "2. **Stale ticket.** A closed ticket carries a title that now states the opposite of the rule it closed.",
      "3. **Rewritten policy.** An agent rewrote a conformance note to fit a change. Defensible, but read that paragraph.",
      "4. **Editor core.** Approve a two-line change to the vendored editor, and the upstream pull request.",
      "5. **One file, not two.** This page exists twice. Collapse to one, probably a symlink.",
      "6. **Five calls.** Listed in the next group. Those five gate the tickets.",
    ].join("\n"),
  );
  push("Answer the six above in order. Do not start below them.");
});

// ---------------------------------------------------------------------------
// Group 2: resea — the 10-row table rule 5 asserts on
// ---------------------------------------------------------------------------

const ticket = (id, label) =>
  `[${id} "${label}"](https://example.invalid/browse/${id})`;

group("resea", () => {
  push("## RESEA tickets - due tonight");
  push(
    "The work is not writing tickets. It is posting a review call into the " +
      "tracker. Nothing has been posted yet.",
  );
  push(
    "The feature reads a state case system through `GET /resea-status` and " +
      "shows the steps in a carousel. Epic: " +
      ticket("FFAI-62016", "Home: carousel and action plan status") +
      ".",
  );
  // Rule 5: one <table>, 10 body rows, an <a href> in every ticket cell
  // EXCEPT the FFAI-62019 row, whose unescaped brackets close the link label
  // early so the construct is not a link at all.
  push(
    [
      "| Ticket | State | Tonight |",
      "|---|---|---|",
      `| ${ticket("FFAI-72357", "Productionize the programs service")} | On Hold | Add cache expiry, single flight, rate limit, error code, unique key |`,
      `| ${ticket("FFAI-62020", "Participant API integration")} | Triage | Add the dedupe key. Land the connector first |`,
      `| ${ticket("FFAI-62017", "Carousel and status modal")} | Triage | Add the per-tenant switch and the fall-back decision |`,
      `| ${ticket("FFAI-72606", "Confirm the API contract points")} | Triage | Add the five missing questions. Do not close |`,
      `| ${ticket("FFAI-72356", "Participant identity: verify and roll out")} | Triage | **Top blocker.** Move out of Triage, name a backfill owner |`,
      `| ${ticket("FFAI-72629", "Spike: steps data model and tenant config")} | In progress | Comment two reversals: row filtering dropped, config moved |`,
      `| ${ticket("FFAI-62021", "Spike: multi-program extensibility")} | Verification | Needs owners for the commercial check and the service name |`,
      `| ${ticket("FFAI-72628", "Spike: print component")} | Triage | Keep open. It gates the row below |`,
      `| [FFAI-62019 "[nice to have] Print Action Plan"](https://example.invalid/browse/FFAI-62019) | Triage | Keep. Blocked on the print spike |`,
      `| ${ticket("FFAI-72342", "Close the connector window")} | Triage | Land the connector before this closes |`,
    ].join("\n"),
  );
  push("### Five calls that gate the tickets");
  push(
    [
      "1. The local note contradicts itself on one ticket: one section says close it as superseded, another says never close it. Pick one.",
      "2. A request for stored state plus an error flag reverses two written decisions.",
      "3. One ticket asks the state to drop a field another team expects to use. Settle it first.",
      "4. Does the new admin API own the label overrides, or take upstream titles?",
      "5. Does the P1 stay one ticket, or split now.",
    ].join("\n"),
  );
});

// ---------------------------------------------------------------------------
// Groups 3-11: nine more named groups, filled to reach 82 atoms
// ---------------------------------------------------------------------------

const REST = [
  ["editor", 6],
  ["parser", 6],
  ["board-view", 6],
  ["inline-view", 6],
  ["database", 5],
  ["delivery", 5],
  ["agents", 5],
  ["notes", 5],
  ["backlog", 5],
];

for (const [slug, n] of REST) {
  group(slug, () => {
    push(`## ${slug.replace(/-/g, " ")}`);
    for (let i = 0; i < n - 1; i++) push(filler(i, slug));
  });
}

// ---------------------------------------------------------------------------
// Tail: ungrouped atoms, padded so the total is exactly 82
// ---------------------------------------------------------------------------

const ATOM_TARGET = 82;
const isGroupMarker = (b) => b.startsWith("<!-- <atom-group") || b.startsWith("<!-- </atom-group");
const atomCount = () => blocks.filter((b) => !isGroupMarker(b)).length;

push("## Loose ends");
let pad = 0;
while (atomCount() < ATOM_TARGET) {
  push(filler(pad++, "loose"));
}
if (atomCount() !== ATOM_TARGET) {
  throw new Error(`atom count is ${atomCount()}, wanted ${ATOM_TARGET}`);
}

const groupCount = blocks.filter((b) => b.startsWith("<!-- <atom-group")).length;
if (groupCount !== 11) {
  throw new Error(`group count is ${groupCount}, wanted 11`);
}

writeFileSync(OUT, blocks.join("\n\n") + "\n");

const bin = atomdownBin();

// Ids and digests come from the real tool. A digest is a sha256 of the atom's
// own content and rule 6 runs `atomdown verify`, so there is nothing to fake.
execFileSync(bin, ["materialize", "--digest", "-w", OUT], { stdio: "inherit" });

// Slugs are added here rather than by the tool, because the atomdown binary on
// this machine predates its own `--slugs` flag. A slug is a readable name, not
// identity, and it is not part of the digest — `atomdown verify` below is what
// proves that injecting them changed nothing the tool cares about.
const slugify = (line) =>
  line
    .replace(/^[#>\s|*_-]+/, "")
    .replace(/\[([^\]]*)\]\([^)]*\)/g, "$1")
    .replace(/`/g, "")
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "")
    .split("-")
    .filter(Boolean)
    .slice(0, 7)
    .join("-") || "atom";

{
  const lines = readFileSync(OUT, "utf8").split("\n");
  for (let i = 0; i < lines.length; i++) {
    const m = lines[i].match(/^<!-- <atom id="([0-9A-Z]+)"(.*)\/> -->$/);
    if (!m || m[2].includes("slug=")) continue;
    // The atom's content starts on the next non-blank line.
    let j = i + 1;
    while (j < lines.length && lines[j].trim() === "") j++;
    const slug = slugify(lines[j] ?? "");
    lines[i] = `<!-- <atom id="${m[1]}" slug="${slug}"${m[2]}/> -->`;
  }
  writeFileSync(OUT, lines.join("\n"));
}

const lint = execFileSync(bin, ["lint", OUT], { encoding: "utf8" }).trim();
const verify = execFileSync(bin, ["verify", OUT], { encoding: "utf8" }).trim();
if (lint !== "ok" || !verify.startsWith("ok")) {
  throw new Error(`atomdown rejected the fixture: lint=${lint} verify=${verify}`);
}

console.log(
  `wrote ${OUT}\n  atoms: ${ATOM_TARGET}  groups: ${groupCount}\n` +
    `  atomdown lint:   ${lint}\n  atomdown verify: ${verify}`,
);
