# Recovery handoff for a new normal Codex chat

Last verified: **2026-10-19 22:01 America/Phoenix / 2026-10-20 05:01 UTC**. Installation closure was confirmed immediately afterward. This is an explicitly requested context handoff, not a task list or a replacement for Beads.

Steve intends to open a **new typed Codex chat**, have it read this file, then start voice in that same chat. This is a plausible way to avoid the previous chat-routing problem. It has not been proved to solve every browser or voice issue.

## Mission and immediate state

Coordinate the authorized FFAI, Iugum, Beads, and code-server extension work through implementation, meaningful tests, independent review, authorized deployment, and accurate Beads status. Inspect real worker and tmux activity before starting anything. Keep actionable work moving around blocked tracks. Do not start unrelated Ready backlog.

**Iugum Agent extension 0.1.14 and its matching Linux service are installed and independently verified.** The actual private editor channel and typed editor actions passed in an isolated editor. Real Mac/phone mouse acceptance and voice integration remain open. Installed does not mean every feature is enabled or verified end to end.

**FFAI backend repairs are complete and independently reviewed**, according to the FFAI manager's final report. Its uncommitted test footprint and publication remain decisions, not authorization to rewrite or publish.

## Existing ownership: inspect before dispatch

These existing chats are authorized by Steve to receive recovery coordination messages. Reuse them; do not create replacement managers without inspecting them.

| Role | Existing chat | Identity |
| --- | --- | --- |
| Iugum recovery manager | Resume Iugum agents and Beads work after crash | `01a156eb-4d1f-7121-9825-d3a7bc454533` |
| FFAI recovery manager | Resume FFAI container backend repairs after crash | `01a156eb-9519-7811-a341-c8a56ea598b3` |
| Originating recovery chat | Recover Iugum and FFAI progress after crash | `01a156e8-0ae8-78b2-98c2-868e751ad70f` |
| Original precrash chat | Compare t3code with your setup | `01a14730-e481-7da1-a75f-686cf187eae9` |

Use `list_threads`, `read_thread`, and compact `wait_threads` snapshots for current state. Use exact returned titles if identifying a chat to Steve. Saved summaries can lag behind a later turn. `send_message_to_thread` is authorized for these recovery managers; respect any actual tool rejection rather than bypassing it.

The Iugum manager's worker pool completed its current work:

| Worker | Actor / scope | Latest state |
| --- | --- | --- |
| `/root/board_recovery` | `editor-origin-worker`: native extension, masked webview relay, packaging | Source and rollup closed after independent PASS |
| `/root/container_recovery` | `container-recovery`: Go backend, build, installation, runtime proof | Implementation `iugum-6ltd.12` and private-channel proof `iugum-6ltd.7` owner-closed after independent PASS |
| `/root/embedded_board_review` | `codex-companion-acceptance`: companion verification; separate global-doc reviewer identity | Companion source closed after independent PASS; global guidance review complete |
| `/root` in Iugum manager | `iugum-recovery-coordinator`: independent source/build/runtime reviewer | Recovery ticket `iugum-918r` retains the current handoff; real-client acceptance remains open |

Do not assume old subagents can be controlled from a new chat. Inspect the existing manager and ask it for a scoped continuation when necessary. Do not invent a subagent's resumable session ID.

All new Codex subagents must explicitly use **`gpt-6.1-sol` or `gpt-6-luna`**, a scoped prompt, and `fork_turns: "none"`. Use Astra only if Steve specifically requests it. Steve chooses the main model.

## Permissions and preservation

Steve explicitly approved installing the latest reviewed Iugum Chat extension and matching backend in the **existing `iugum-agent` container**, with backups and preservation of configuration, history, active terminal sessions, and windows. Both reviewed updates have now been installed. Earlier rejected copy approvals are historical; later explicit authorization superseded those questions.

Continue reversible, authorized work without asking permission for every routine step. Reuse explicit permissions. A later authorization does not erase a tool denial; report actual current restrictions accurately.

No blanket permission exists to send provider prompts, forward audio to unapproved destinations, install the Chrome companion, or perform intrusive real mouse actions. Existing Mic Bridge previously worked. Voice acceptance means integration with the new chat extension, not a generic microphone test. Do not infer an unrelated spoken model-switch requirement.

Preserve unrelated dirty files. **No git commits, pushes, PR publication, or Dolt remote sync without explicit instruction.** No new branch, worktree, duplicate manager, or remote service before inspecting existing ownership and work. Preserve the existing remote containers, tmux sessions, provider processes, configuration, and history. FFAI work stays in `ffai-agent`; Iugum work stays in `iugum-agent`.

## Direct Codex browser workflow

Steve wants the **Codex in-app browser (IAB)**. Use the supported browser tools directly in the new owning chat. Prefer its existing visible tab. Do not silently switch him to another chat or Chrome.

1. Use a documented CUA entry call to select the existing tab by known URL or current tab ID. Inspect the returned state.
2. If the requested page has no matching tab, create a visible IAB tab at the requested URL.
3. Inspect fresh accessibility state or a screenshot. Click the actual Iugum activity icon.
4. Verify **IUGUM: AGENT CHAT**, its provider selector, message composer, and voice controls are visible. The Copilot panel on the right is a different chat.
5. Mark the requested output tab as a deliverable so it remains open. Preserve the Beads tab.

Example entry calls, choosing the first one that fits the actual request:

```javascript
let tab = await cua.getTab({url: "https://iugum-agent-code.local/?folder=/config/workspace"}, {browser: "iab"});
// Or, when the actual current tab ID is known:
let tab = await cua.getTab("CURRENT_TAB_ID", {browser: "iab"});
// Or, when no matching tab exists and Steve asked to show the page:
let tab = await cua.createBrowserTab("iab", "https://iugum-agent-code.local/?folder=/config/workspace", {visible: true});
```

Read the tool's documentation before further calls. Use fresh indices from the actual current state, never historical accessibility indices. A failed or hidden subagent tab is not proof that the owning chat cannot control its visible tab. Subagent IAB sessions are separate. An `open_in_codex` response of `queued` is pending work, not visible success.

Evidence: the original precrash main agent directly selected IAB tab `2`, clicked, typed, took screenshots, and marked it deliverable at roughly 18:03–18:05 Phoenix on October 19. It opened the hook-audit drawer and added FFAI to the Beads project selection. The recovery manager later opened and visually verified Iugum Agent in its own IAB tab `2`; screenshot: `/private/tmp/iugum-agent-visible-chat.png`. That ID belongs to that chat, not automatically to this new chat.

The parent later reported returning Steve to the originating chat after the unwanted chat switch. It also reported Beads and two code-server IAB tabs there. A separate Chrome panel loaded successfully, but Chrome is not Steve's requested final browser. Inventory current tabs rather than assuming these remain unchanged.

The failed panel-opening loop lasted 42m41s from the first code-server request at 21:15:39, or 20m04s from the specific chat-panel request at 21:38:16 to the handoff request at 21:58:20. Do not repeat the routing loop or blame Steve. The original direct-working chat had creation source `user`; recovery had source `voice_chat`. Different application-injected instructions appeared at about 18:24 Phoenix; they were not a rule in Steve's AGENTS/config. This makes a new typed chat followed by voice a reasonable experiment, not a guaranteed fix. Do not copy that historical restriction into a new standing limitation.

Respect actual browser/app tool restrictions. One optional attempt to open a second isolated IAB tab for port 3860 was automatically denied for missing connector-origin access. It was not retried through another browser, raw CDP, or another workaround. That specific denial does not establish a general inability to control the already requested IAB page. Report a concrete current denial if one occurs.

### Durable global browser instructions

The canonical global file is `/Users/steve/agent-harness/official/platforms/codex/AGENTS.md`. `/Users/steve/.codex/AGENTS.md` remains its existing symlink. The new **Codex browser control** section covers owning-chat control, matching-tab reuse, visible verification, deliverable retention, truthful queued state, and respecting actual denials. SHA256 at review: `f09f406b17cade2b0ba6230d5bbf2626b802f359877c4b911944a805572eb87a`.

This change was independently reviewed and owner-closed under `iugum-2gln — Persist Codex browser ownership and visible-success guidance globally`, with separate review `iugum-2gln.1`. It already appeared in refreshed AGENTS context. An encrypted original was verified at `/Users/steve/agent-harness/.local/browser-guidance-backups/codex-AGENTS-20261020T044826Z.gpg`. No commit or push occurred. Harness checks found 297 managed links intact and six unrelated preexisting app-managed settings-copy drifts; those settings were not changed.

## Current Iugum installation and evidence

Code-server: **https://iugum-agent-code.local/?folder=/config/workspace**.

Source repositories:

- `/Users/steve/projects/github/srhopkins/iugum`: Go program, wiki, Beads, session service, canonical feature tracker.
- `/Users/steve/projects/github/srhopkins/code-server-extensions`: extension monorepo; current extension is `extensions/iugum-chat` and uses JavaScript, not TypeScript.

The chat reused three JavaScript/CSS frontend files from the Iugum SilverBullet wiki. The agent runtime remains Iugum. Steve prefers one extension repository. TypeScript migration, public release, and license cleanup were discussion only, not implementation/publication authority.

### Reviewed bundle D: current

Local immutable artifact directory: `/private/tmp/iugum-reviewed-source-d-tv83yl_d`.

| Artifact | Exact SHA256 |
| --- | --- |
| `iugum-chat-0.1.14.vsix` | `9dbc23dfb1b40a3cfb8b7341b00588fda2c820200884b652d066b4eaf1b1ddc4` |
| `iugum-linux-complete` | `7bed424cc6b6517ea46ca5c8d93634c3f570938788abf591d6ce8eb0701d294c` |
| `iugum-client-companion-source-d.zip` — local only, not installed in Chrome | `fddada8fb26fadd95ca57e130189ca26b35cdfe1657af6bfdeb8e10b2ecf4cb3` |
| `artifact-manifest.json` | `772d8adb253ae7f0935c89cb9f9881dd643e655b20a546f73b04e3e2e396cde7` |

The VSIX contains 28 verified production files and excludes the companion. The companion ZIP contains 22 production files, including the new signed-acceptance verifier. Both closures match the frozen reviewed source.

Go review snapshot: `/private/tmp/iugum-browser-acceptance-review-ux_l1b2u`; full 1,872-file manifest `0155d68a13e20c1de434b0063e0443a89f9c0e180a6a7bd323a2488bcdd09f92`, owned-six manifest `2b593773f54b5cebaf126624fa005beced6ab22316742bcce45b157eae418020`. Native/companion review snapshot: `/private/tmp/iugum-masked-native-source-al8dejzj`; 114-file manifest `/private/tmp/iugum-masked-native-final-manifest.sha256`, SHA `630d18f1ef9b5a80dc5c112f2921d37aafe35ed1287f9568163299e42c7c7275`.

Linux build tree: `/private/tmp/iugum-source-d-linux-build-db3jp15_/source`; manifest `812d4a748514399b5a82af9309f692984717d8af18c26ad5f54b5749fa60c3c2`. Its only source delta from reviewed Go is the reviewed Linux `wikiblob/silverbullet` input, SHA `6045f0ce8b50a66dca7291da21c9425c8f8925f40ddb9f8e904a4810ec0943f5`. Root verified every source hash, static ELF AMD64 without an interpreter, and the complete embedded Linux blob. Build metadata reports actual dirty HEAD `27d5b0a6efe9-dirty`, build `2026-10-20 04:49 UTC`, Go `1.26.5`, CGO off. This does not imply a clean committed source tree.

### Runtime: verified installation

Remote release: `/config/.local/share/iugum/releases/source-d-7bed424c-20261020` in `iugum-agent`.

- Global session service: port **3853**, PID **2077771**, `/proc/2077771/exe` independently matches exact D Linux hash.
- User extension: `/config/.local/share/code-server/extensions/srhopkins.iugum-chat-0.1.14`. All 28 files independently verified; supported installer adds only expected `package.json.__metadata`.
- Service arguments except executable, environment, data directory, account, configuration, and policy match the preceding service. No host-tools, browser-grants, or coordination opt-in flags were added globally.
- Before stopping only old service PID 2046646, the final check found zero canonical conversations, selected owners, child processes, and pending receipts. It was stopped gracefully.
- Private C backup exists under `release/backup`. Configuration/policy/Machine settings compare byte-identically. Non-token history mismatches: zero. Normal internal control-token rotation retained the same private path and mode 0600. Never print its value.
- Extension registry has the same two entries; only Iugum Chat changed from 0.1.13 to 0.1.14.
- Authenticated `/api/agents` remains healthy and reports `codex-session`, `claude-session`, and `cursor-session`. Native Iugum/OpenCode source support exists, but those providers are not configured in this service. No provider prompt was sent to prove live compatibility.
- Existing editor windows were not reloaded. An already open extension host can retain older code until an ordinary reload. Do not claim that an open old window has loaded 0.1.14 merely because installation succeeded.

Remote receipts under the release directory:

```text
global-upgrade-receipt.json
installed-extension-verification.json
registry-settings-preservation.json
validation/actual-native-private-channel-validation.json
```

Fresh isolated service/editor remain on **3859/3860**, PIDs **2072820/2072821**. They use disposable data and no provider requests. The actual code-server native notification said **“Own webview private channel verified. Mouse remains unavailable.”** Same-fresh-window `open_file`, `focus`, and `inspect` passed against a clean disposable README. The test asserts the trusted workbench notification, not untrusted DOM success text.

Repeatable fixture source: `/private/tmp/iugum-d-runtime-validation.cjs`. Its first cold command search timed out; bounded fresh picker snapshots resolved startup without source changes. Root independently reviewed the actual fixture and receipt, isolated and user installed bytes, process binary, authenticated health, and preservation. Root did not rerun browser UI after the separate connector-origin denial.

### Source correction, tests, and reviews

Code-server masks `window.parent`/`window.top` before extension scripts run. Capturing them early does not fix that. The new supported path uses private `acquireVsCodeApi` outward, typed self-message routing, and exact compiled Chrome document injection. DOM offers/status/errors are untrusted routing data. Native private-panel identity and the two full fingerprint confirmations remain authority.

Backend acceptance now returns exact, one-use Ed25519-signed acknowledgements after actual durable acceptance. The companion verifies command/grant/request/observation/lifecycle bindings, clock and expiry, and current sealed keys. Lost or forged acknowledgements cannot promote freshness or completion and cannot replay execution. This closes a real source gap; it does not prove real Mac input.

Independent current PASS tickets:

| Scope | Review | Implementation state |
| --- | --- | --- |
| Go signed acceptance | `iugum-6ltd.4.13.1` | `iugum-6ltd.4.13` owner-closed |
| Companion acceptance | `iugum-6ltd.4.11.2.1` | `iugum-6ltd.4.11.2` owner-closed |
| Masked native relay and VSIX | `iugum-6ltd.4.11.3.1` | `iugum-6ltd.4.11.3` and design rollup `.4.11` owner-closed |
| Matching D artifacts | `iugum-6ltd.11.1` | `iugum-6ltd.11` owner-closed |
| Installation and preservation | `iugum-6ltd.12.1` | `iugum-6ltd.12` owner-closed; actual private-channel `.7` also owner-closed |

Checks passed: full frozen static Go suite; relevant CGO race suites; 156 extension tests; 90 companion tests; 21 joined Go/native/media/runtime tests, including forged initial offers/status, both confirmation orders, cancellation/window change, actual queued observe/click/fresh-observe/scroll against mocked Chrome APIs, and dropped/forged real Go acknowledgements. Distinguish mocked Chrome calls from the actual installed private editor proof.

Root logs: `/private/tmp/iugum-root-final-masked-npm.log`, `/private/tmp/iugum-root-final-masked-joined.log`, `/private/tmp/iugum-root-companion-ack-tests.log`, `/private/tmp/iugum-root-go-ack-tests.log`. Frozen broader logs: `/private/tmp/iugum-browser-acceptance-frozen-static.log` and `/private/tmp/iugum-browser-acceptance-frozen-race.log`.

### Remaining acceptance and configuration

`iugum-6ltd — Let Iugum agents inspect and control their exact code-server editor` and `iugum-6ltd.4 — Design and verify mouse controls in the actual Mac or phone code-server client` remain open. Their Notes now distinguish installed D and completed source from remaining actual-client proof.

Real Chrome companion installation, exact-client calibration, trusted manual full-fingerprint pairing, and consented real capture/click/scroll are not done. The reviewed ZIP remains local. Global host/browser opt-ins are unchanged/off; optional configuration ticket `.8` is separate. Phone/Safari has no proved equivalent client seam. Do not close mouse acceptance from a mock, installed package, or successful own-webview notification.

Voice source is reviewed and included, but `iugum-db9` still needs actual new-extension voice integration evidence. No new audio was forwarded to the homelab speech service at `192.168.1.42:8770`; that destination does not have blanket permission. Existing Mic Bridge is a previously working path and was left intact.

## Live containers, managers, and Beads

Use the existing Docker context **`homelab`**, container **`iugum-agent`**, user **`abc`**, home `/config`. Example read-only entry:

```bash
docker --context homelab exec -i -u abc iugum-agent tmux list-sessions
```

For a shell use `bash --noprofile --norc`; an existing bashrc syntax error was observed and not repaired. Do not recreate containers or clear shell state as a shortcut.

Verified surviving Iugum tmux sessions: **`pm`**, **`t3-server`**, **`wiki`**. Original Claude processes **4856, 5876, 7807** survived. Existing read-only Beads proxy **3854 / PID1922074** survived. Earlier isolated host-tool service/editor **3855/3856 / PIDs1923967/1928384** and C validation **3857/3858 / PIDs2038202/2038256** also survived. Recheck PIDs before acting; a historical PID is not current ownership proof. Disposable D 3859/3860 remain for inspection, not permission to stop another listener.

Local Beads board: **http://127.0.0.1:38492/?project=iugum&project=ffai&lanes=1**. Local launchd label: `com.steve.beadview`. Board recovery included default 14-day closed filtering, All closed override, project collapse/swimlanes, activity/session metadata, and metadata text decoding. Totals count historical closed tickets even when older cards are hidden; an earlier check showed 131 hidden. The browser polls roughly every 15 seconds.

The activity circle spins only with fresh structured **Working** execution metadata and an unexpired lease. Plain `in_progress`, comments, or stale metadata are not evidence of activity. Read `docs/beadview-activity.md` before repairing metadata. Known good helper form:

```bash
iugum bead-activity --actor ACTUAL_WORKER --run ACTUAL_RUN_ID \
  --session ACTUAL_CODEX_THREAD_ID --platform codex \
  --label 'Descriptive worker role' --ttl 5m \
  --reason 'Concrete current action' ISSUE_ID working
```

Use the actual current thread ID, not this handoff's old coordinator ID. If a subagent session ID is unavailable, use explicit `--run` and `--identity-unavailable 'Reason'`; never copy a parent ID. State can be `paused` when waiting on review or `completed` after actual completion. Refresh during meaningful work; do not create immortal heartbeat loops to make circles spin. Current manager activity identity was actor `iugum-recovery-coordinator`, run `recovery-01a156eb-4d1f-7121-9825-d3a7bc454533-host2`. Host is now `Stevens-MBP-2.localdomain`; preserve historical runs from `Stevens-MacBook-Pro-2.local`.

Local recovered binary reported `27d5b0a6efe9-dirty`, build 2026-10-20 01:43, and was installed through the existing install process. Check `iugum version`, `bd --version`, the board process, and actual health rather than assuming it matches the newer remote D build. Local backups are under `~/.local/share/iugum/recovery-backups/20261020`. `bd` is the existing shim. Do not replace the local binary merely to match a remote version stamp.

Beads workflow comes from project `AGENTS.md` and `.agents/skills/beads/SKILL.md`: run `bd prime` when context is stale; use Beads for durable tracking; meaningful comments are the visible work log, Notes the current handoff. Actual worker claims Ready work, moves to In Progress, then In Review after self-checks. A separate reviewer claims a separate ticket and records identity, exact reviewed state, findings, and checks on both. Original owner closes after current independent PASS. Block only for a real unresolved prerequisite and record the dependency. Stale In Progress is not active work. Store durable facts with `bd remember`, not ad hoc memory or Markdown TODO files.

Read project source boundaries and `docs/wiki-ui-contract.md` before edits. Preserve upstream `silverbullet/` and the separate Beads fork boundaries. Run the required `scripts/agentchat-fe-check.sh` for wiki chat/keyboard changes, plus `--prove-regression` for keyboard handling. Use project build/install instructions and keep static builds compiling. This handoff adds no exception to these rules.

## FFAI: completed repairs, unresolved decisions

The following is the existing FFAI manager's verified recovery report, relayed by the parent during this handoff. That reply performed no new remote inspection. Recheck current ownership before changing anything.

- Both backend repairs are complete and independently reviewed. Disposable local integration database verification passed. Frontend verification passed with limits recorded by that manager.
- Temporary servers and databases were removed. Six backend files remain uncommitted and unpublished.
- The agents created the integration suite/setup; these were not inherited team tests. Setup creates/drops schemas and writes data. A repaired guard refuses the wrong database target.
- Test footprint is **1,851 lines versus an approximately 600-line target**. Overlap, reuse of established patterns, simplification, publication, and immediate refresh behavior are under discussion. No permission exists to delete tests, simplify them, or publish yet. Preserve evidence and present a concrete decision.
- The verifier accidentally cleared browser cookies. The pgAdmin page was restored, but deleted cookies and unknown closed tabs were not recovered. Do not repeat destructive browser cleanup.

### Exact locations and publication holds

Use Docker context **`homelab`**, container **`ffai-agent`**, user **`abc`**. `ffai-agent-t3` is a web proxy, not the implementation container. No test removal, commits, pushes, publication, PR closure, or shared database work is authorized. The intended consolidated backend PR is **3985**; local changes remain unpublished. Keep older PRs open until the replacement is published and verified.

| Purpose | Verified state |
| --- | --- |
| Shared meta-repository | `/config/projects/FutureFit-ai` — never switch its branch while managers share it |
| Pathways repository | `/config/projects/FutureFit-ai/ffai-pathways` |
| Backend repair worktree | `/config/projects/FutureFit-ai/.worktrees/pathways-lean-be` |
| Backend branch / HEAD | `feat/ffai-62020-programs-read-path` / `87c4cff2dc2ee8637c5b86a20d957dde1b2d34f0` |
| Existing runtime worktree | `/config/projects/FutureFit-ai/ffai-pathways/.worktrees/local-lean` — detached at committed `87c4cff2d`; uncommitted repairs were **not** loaded there |
| Verified frontend branch / HEAD | `wk/4127-merge-main` / `aa80fbf79` |
| Independent frontend verification worktree | `/config/tmp/bnbi1-verify/web` |

The frontend owner's absolute worktree path was not independently resolved; earlier Notes referenced `.worktrees/fe-4127-merge`. Do not treat that relative reference as a verified absolute path.

Six backend files remain dirty under `/config/projects/FutureFit-ai/.worktrees/pathways-lean-be`:

```text
M  apps/eng_services/programs-service/src/participants/map-resea-steps.ts
M  apps/eng_services/programs-service/src/participants/map-resea-steps.spec.ts
M  apps/eng_services/programs-service/src/participants/upstream-connector.service.spec.ts
M  apps/eng_services/programs-service/src/programs/programs.int-spec.ts
?? apps/eng_services/programs-service/src/programs/int-db-target.ts
?? apps/eng_services/programs-service/src/programs/int-db-target.spec.ts
```

The protected malformed-data repair's final diff checksum remained `f7a07b9ede4759b7fee0e8842f9221556c7d7cd7f72ef702289143f939614594`.

### Completed tickets and evidence

| Beads task | Current result |
| --- | --- |
| `bdffai-3wkg` — Reject malformed Mass Hire values before replacing the last good answer | Original implementer closed after independent scoped review `.1` PASS |
| `bdffai-cboo` — Enforce local-only Prisma integration-test setup | Original implementer closed after independent harness `.1` and real-database `.2` gates |
| `bdffai-bnbi.1` — Verify latest consolidated frontend with lean backend | Closed, **PASS WITH FINDINGS**, limits below |
| `bdffai-eo5w` — Final readiness for gateway PR 3984 | Original owner closed after confirming that PR was already merged |

Malformed-data tests reproduced failure before the fix; 78 unit tests passed afterward. Final harness work passed 151 unit tests, type checking, and lint. A separate reviewer ran 110 executable guard/parser probes. Four real integration tests passed against a new disposable Postgres on `127.0.0.1:55439`. The disposable container was removed; no proof containers remained and no existing stack was restarted.

Limits: existing malformed saved rows are not repaired; a URL guard cannot determine whether a loopback port forwards elsewhere; IPv6 brackets may fail safely; unique test schemas remain until the disposable database is removed.

Frontend evidence: code generation against the real local gateway on port 4100, normal type/lint checks, 1,686 Vitest tests, and 61 tests across four RESEA component suites passed. Local login, Home card, modal, and deadline display passed. The card showed three of six requirements complete; the modal showed six steps and eight action steps. Refresh requests occurred, but the backend returned the same answer during its five-minute cache and a new timestamp after expiry.

Frontend limits: the runtime used committed `87c4cff2d` without the dirty repairs. The local Events tab was disabled, so the real “Watch CCS Now” button was not clicked; the verifier invoked the exact refresh function directly. Gates-off behavior was checked through code/component tests, not a runtime flag change. No first-visit database reset occurred. A completed action still showed a due date; wording needs a decision. Standalone Cypress typechecking has inherited configuration failures, recorded in deferred `bdffai-6rdw`. `bdffai-pnj` records the cache/refresh decision and missing live-button fixture.

The temporary frontend on 3013 was stopped. Ports **3013, 3014, 55439** were free. The local runtime owner was told verification ended and could resume its own restarts.

### Test provenance and existing conventions

The integration file is absent from team `origin/main`. Our agents added it for RESEA. Claude subagent `a1553b729c0791239`, session `5b8f8f12-13dc-4d8f-b050-f14a88c56b86`, directly wrote the reduced suite and unsafe setup on October 19 at **13:36 Phoenix**. Relevant commits: `655fc2ce2` reduced the suite; `7c9ed1047` added upsert-race proof. The original file entered the feature branch in `f7b2b0d2d` on October 9. Steve's Git identity alone does not prove who wrote that original version.

The four integration behaviors are user-separated progress persistence, a first-write connector race producing one row through native upsert, same-process request sharing of one upstream call, and the public query returning the requesting user's saved progress. The agent brief requested one integration test per public query; additional concurrency tests followed review findings. No separate team mandate for all four was established.

| Database-related subset | Lines |
| --- | ---: |
| Integration suite | 274 |
| New database guard/setup helper | 150 |
| New guard regression tests | 337 |
| **Subset total** | **761** |

The previous reduced integration file had 296 lines, so this repair added **465 net lines** across these three files. The **1,851-line overall test footprint** is a broader count; do not conflate it with this 761-line subset or the approximately 600-line target.

Comparable team `origin/main` patterns:

- `apps/eng_services/career-events-service/src/career_events/events-cache.service.spec.ts`: mocked Prisma calls for the nearest Postgres-cache feature.
- `apps/eng_services/user-profile-service/src/profile/test/integration/profile.empty-db.int-spec.ts`: real GraphQL/Mongoose tests against configured Mongo.
- `packages/node_packages/db-query-ir/src/test/mongo-integration.setup.ts`: owned ephemeral `MongoMemoryServer` with teardown.
- `scripts/local_databases/migrate-local.sh`: loopback, expected-port, named-container, and checkout ownership checks before Prisma migration; it owns a developer stack, not a disposable suite.

Real-database testing is established in the repository. This generic URL guard and Prisma test setup are new infrastructure; no directly compatible shared Postgres helper was found. The manager's recommendation remains **discussion only**: retain two real query cases, including the upsert race, and simplify setup around an owned disposable database. No tests were removed.

### Remote managers, identities, and Beads access

Existing tmux managers **`mgr-resea-be`**, **`mgr-resea-fe`**, and **`mgr-resea-local`** remained intact. All recovery-launched `resea-*` worker sessions finished. Original coordinator Claude session **`7a86af36-a295-437c-914c-af530ea2e590`** was observed alive outside tmux; transcript `/config/.claude/projects/-config-projects-FutureFit-ai/7a86af36-a295-437c-914c-af530ea2e590.jsonl`. No duplicate coordinator was launched. Manager Claude UUIDs were not sufficiently confirmed to list as authoritative.

| Completed worker | Actor | Claude session |
| --- | --- | --- |
| Malformed-data repair | `ffai-resea-repair` | `e3a07a84-621b-4927-a70f-b09269c76715` |
| Harness repair | `ffai-resea-cboo-impl` | `fed6e956-a869-4a35-b2f7-b37a70f48d5c` |
| Harness review | `ffai-resea-cboo-review` | `6c806201-0ba1-4b09-8924-84c8caf4cd5b` |
| Real database proof | `ffai-resea-cboo-proof` | `f3693ab0-ce87-4443-b709-69b1a47c62c1` |
| Frontend verification | `ffai-resea-bnbi1-verify` | `c3e9662c-f5b0-4f12-8f24-2675924ddde3` |

Canonical remote Beads access, **inside `ffai-agent`**, requires:

```bash
source /config/.config/beads/server.env
export BEADS_DOLT_SERVER_MODE=1
/usr/local/bin/iugum beads -C /config/projects/FutureFit-ai …
```

Do not print the environment values. Plain `bd` without this environment previously attempted port 0. The reviewed activity helper is staged at `/config/tmp/iugum-bead-activity-reviewed-9a13d95a`, SHA256 `9a13d95ae497cc26030eaf4d4fef55198eda567adaffbc036c59f195254e738b`; the installed runtime was not replaced. Completed workers recorded truthful completed activity and actual session metadata; canonical readback passed.

Other reconciled tickets: `bdffai-bnbi` remains blocked on publication/schema prerequisites. `bdffai-pkt` is unassigned; its original Codex owner in chat `01a1433c-6346-7130-b3a7-54cca6c02f47` released ownership, and the prerequisite is post-merge follow-up rather than a missing agent. `bdffai-d69` awaits Steve's DNS draft review. `bdffai-5ai.2` is Steve's In Progress epic, not an executing worker. The unrelated Ready backlog was not dispatched.

Reports are `/config/tmp/resea-harness-manager-result.txt`, `/config/tmp/bnbi1-verify/report.md`, `/config/tmp/cboo-review/review.md`, and `/config/tmp/resea-cboo-proof.log`.

Browser incident precision: the verifier reused an existing browser context, cleared cookies, closed its other pages, and navigated the pgAdmin page to the test frontend. Local/session storage clearing happened after navigation on the test frontend; pgAdmin storage clearing was not established. The known pgAdmin page was restored to **http://192.168.1.42:5050/browser/** and HTTP 200 verified. Cookies were not restored; other closed pages are unknown because no prior inventory existed. Use an owned disposable context for future authorized frontend tests.

No recovery worker remains active. Steve's decisions are the reduced test footprint/publication scope and the immediate-freshness contract after the CCS video. Preserve all six dirty files and publication holds meanwhile.

## Saved research: do not start it automatically

`iugum-vii1 — Evaluate local agent scope and code-quality guardrails` is **Ready, unclaimed, side-request**, saved for later. Steve wants minimal blast radius and reuse of existing patterns, with Beads hygiene kept accurate. No guardrail stack, new CI, hook installation, or unrelated FFAI cleanup is authorized now.

Research leads: local Fallow checkout `/Users/steve/projects/github/fallow-rs/fallow` (MIT, JS/TS, changed-reference/report gating, exclusions/rules; it does not replace TypeScript typechecking), Semgrep CE for patterns/conventions/security, dependency-cruiser for JS/TS import boundaries, ArchUnit for Java, ast-grep for structural rules, and Danger JS for programmable PR checks. Danger is not an autonomous scope/architecture detector. Local Git hooks can gate commits; the same checks in CI provide a backstop. Independent reuse/minimal-diff review remains useful. Verify current licenses, product boundaries, and hosted pricing before recommending a purchase; do not claim universal free/OSS availability or unsupported popularity/uniqueness.

## Stale questions and automation

Four old async question cards asked permission for recovery reporting, installing the reviewed extension, sending helper instructions to FFAI, and copying/installing into a disposable profile. Later explicit reporting/coordination/installation authorization resolved them. No supported question-card dismissal tool was found; no answers were fabricated. They are not four current blockers.

An existing automation named **`keep-ffai-and-iugum-work-moving`** runs about every 10 minutes on the old parent. Its prompt says stay quiet unless meaningful progress, failure, completion, or a decision is needed; no unrelated backlog, commits, pushes, or audio. The parent goal is paused. No automation retarget has been done. Inspect the existing automation with the supported automation tools and saved configuration before changing it. Retarget or pause the old coordinator under Steve's requested coordination scope; do not create a duplicate that competes with it.

Steve previously said stop UI changes, then explicitly resumed browser investigation and requested this handoff. This was not a blanket cancellation of already-authorized implementation workers. At handoff, the Iugum source/build/install workers have finished; inspect both managers before dispatching further work.

## New-session startup

1. Read this handoff and current project instructions; run `bd prime` and check `git status` without changing dirty work.
2. Inspect the existing Iugum and FFAI managers plus actual tmux/process activity. Reconcile stale Beads activity; recheck the recorded FFAI work paths and outstanding decisions.
3. Inspect the existing automation so only one coordinator owns future follow-up.
4. Directly select or open the visible IAB tab in this new chat. Verify the actual Iugum panel and preserve the Beads tab. Then Steve can start voice in this same chat.
5. Continue authorized actionable work. Distinguish real-client mouse/voice prerequisites and configuration decisions from completed source, installation, and native editor proof. Do not ask Steve to reconstruct this context.
