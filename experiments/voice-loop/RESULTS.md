# PTT experiment results

Measured on the existing Home Lab model and speech server, 2026-10-21 Phoenix time.
The optional page is `https://iugum-agent-code.local/proxy/3862/`.
The normal Chat extension remains on its working production speech repair.

## What works

The prototype transcribes snapshots while the talk button is held. It prepares
a private first model decision using normal native context and tool schemas.
Release can adopt that decision without a second model call. Preparation writes
usage accounting, but no conversation messages. It never executes tools.
Cancelled, changed, expired or already-used drafts cannot be adopted.

No per-hold session files or fork directories are created. A single memory slot
and bounded recent hold identities hold the temporary state. The test agent uses
one separate data directory seeded once from the original native context. Tests
changed that isolated copy. Hashes of the original context files remained unchanged.

## Measurements

| Check | Result |
| --- | --- |
| Prepare first synthetic text answer | 9,672 ms |
| Release matching ready answer | 3 ms |
| Model usage records before/after release | 1 / 1 |
| Conversation state before release | Unchanged |
| Repeat same release | HTTP 409; rejected |
| Following ordinary answer | 8,779 ms |
| Generated speech transcription, full pipeline | 494 ms |
| Prepare answer to that transcript | 7,453 ms |
| Release matching answer | 3 ms |
| Reply synthesis, first PCM bytes | 632 ms |

The ordinary answer used a different prompt and a later conversation state. It
is an illustrative comparison, not an identical-context performance benchmark.
The full audio test deliberately waited for preparation before logical release.
It proves synthesis → recognition → native preparation → promotion → reply audio.
It does not prove that normal human speech makes a matching draft ready in time.
The 635 ms release-to-first-PCM figure starts after the final transcript exists.
It excludes final recognition, browser scheduling and physical speaker latency.

The visible browser fixture also passed synthesis → recognition without opening
the microphone. Existing dictation and PTT were accepted by Steve before this
experiment. Physical microphone, keyboard holding, speaker interruption, narrow
layout and dark appearance for the new page still need user/device acceptance.
The wide light layout was visually inspected in Codex's in-app browser.

## Validation

- Full native static Linux build passes. Focused root tests for policy revocation,
  commitment authority, routing persistence and native state isolation also pass.
- Race tests pass for agentcore and agentdesk, except the existing subscription
  ownership fixture was explicitly skipped because its BSD/GNU stat probe fails
  on Linux. That unrelated source was not changed.
- Harness Go race tests pass, including fixed native destination/conversation
  and credential isolation.
- All 20 Node tests pass, including PTT sends once after release, cancellation,
  stale transcript rejection, microphone cleanup and queued playback stopping.
- Independent source review passes for the draft runtime and PTT harness.

## Remaining latency

The native draft currently needs the complete first model decision, taking
7–10 seconds in these samples. Partial transcript changes invalidate an older
answer. A short hold can therefore fall back to a normal turn. Repeated drafts
consume model capacity even when discarded. This prototype does not yet deliver
consistent immediate replies after release.

The next implementation step is incremental/cancellable model preparation and
early sentence delivery, with measurements of how often real holds reuse drafts.
That belongs in the shared Chat extension after this isolated prototype is
accepted. Full hands-free turn detection and the video's trained micro-turn
model remain deferred. No model adapter was downloaded.

## Running instances

The prototype native process uses loopback port 3864. The harness uses loopback
port 3862 through the existing HTTPS code-server proxy. Both live under
`/config/.local/share/iugum/` in dated prototype directories. They do not replace
active agent processes and are not configured as permanent services. A container
restart will require starting them again. See README.md for launch arguments.
No commits, pushes or production extension rollout were performed.

## Follow-up token measurement

The controlled token experiment found 3,500 tokens for one answer versus 10,450
for three completed drafts. See `TOKEN-RESULTS.md` and `token-measurements.json`
for the method, per-request totals and accounting limits.
