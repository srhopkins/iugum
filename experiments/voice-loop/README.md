# Push-to-talk draft experiment

This is Steve's authorized container experiment. The installed Iugum Chat and its
working dictation stay on their current binaries. Full hands-free voice is deferred.
The temporary standalone Go harness is an exception to the one-program rule;
production integration belongs in Iugum and its shared chat components.

## Behavior

Arm the microphone, then hold the talk button. The browser sends growing 16 kHz
recording snapshots for transcription about every 1.2 seconds. One recognition
request runs at a time. Busy partial requests are skipped. The final transcript
waits for recognition already running, then transcribes the complete recording.
This is repeated offline recognition, not a continuous recognizer.

With private drafts enabled, partial text starts a native-agent preparation. The
browser keeps one request active and the latest waiting text. The native agent
uses its normal bounded conversation, instructions, model routing and tool schemas.
Preparation returns before tool authorization or execution. Only an opaque draft
identity reaches the browser. Model usage is still recorded for accounting.

Release submits the final text exactly once. A matching, ready draft can supply
the first model decision. Final text, native history, workspace context, profile,
configuration and tool schemas must still match. Current model and tool policy
checks remain active. A changed or missing draft falls back to a normal turn.
A proposed tool runs only after release through the ordinary agent loop.

Each conversation holds one temporary draft in memory, with a 30-second expiry.
No durable conversation fork is created for a button hold. Escape, pointer
cancellation and Stop discard the hold. The 30-second recording limit also
discards rather than sends. Recent hold identities reject duplicate submissions;
the browser does not retry submissions. This is not a durable idempotency API.

Speech uses the approved Home Lab server. Returned 24 kHz PCM plays incrementally;
Stop reply or another hold cancels requests and queued audio. The microphone stays
armed between holds until Stop, but sends no recording between holds. No silence
or voice-activity timer submits a turn. Echo cancellation and noise suppression
remain enabled. No audio or transcript payload is written to service logs.

## Isolation and launch

Run a separate native agent with `ptt_drafts: true`, an isolated data directory,
and loopback listener `127.0.0.1:3864`. Drafting defaults off. The first experiment
supports the native model API, not subscription CLI sessions. A one-time copy of
existing context can seed the isolated test conversation; test submissions then
belong to that copy, not the active conversation.

Build and test the harness inside Linux:

```sh
go test -race .
node --test core.test.mjs draft.test.mjs lifecycle.test.mjs
CGO_ENABLED=0 go build -o ../voice-lab .
../voice-lab --listen 127.0.0.1:3862 \
  --speech http://192.168.1.42:8770 \
  --native http://127.0.0.1:3864 --conversation main \
  --model-base http://litellm:4000/v1 --model hl-vii-qwen35-9b \
  --model-token-file /config/.local/share/iugum/native-chat/litellm-token
```

The model flags configure the older direct-model benchmark route only. PTT uses
`/api/native/chat` and `/api/native/draft`, forwarded to the fixed native instance
and conversation. The browser cannot choose upstreams. Both listeners stay on
loopback. The page is served through the existing HTTPS code-server proxy:
`https://iugum-agent-code.local/proxy/3862/`.

The harness requires its exact browser origin and a custom same-origin header.
It exposes no CORS route, follows no redirects, and never returns model credentials.

## Validation and timing

Node tests cover cancellation, pending microphone permission, playback stopping,
partial/final recognition ordering, single submission after release and private
draft selection. Go package tests cover state persistence, context invalidation,
single-use promotion and tool-policy checks. Package tests alone do not prove
native-agent closure integration; report actual native integration separately.

The generated speech button tests text-to-speech followed by transcription. It
opens no microphone and sends no model prompt. Physical microphone, speaker echo
and phone checks remain separate. The old `bench.mjs` measures the earlier
hands-free baseline; it is not evidence of PTT draft performance.

The page reports final recognition duration, native response duration and
release-to-scheduled audio time. Native response currently waits for the complete
JSON answer. Prepared replies can remove model work after release, but final
transcription and synthesis remain. A late or changed transcript may prevent
reuse. Repeated preparation also consumes model capacity. Do not promise an
instant response without measurements on the actual path.

## UI contract exception

This disposable harness uses native controls, visible keyboard focus, responsive
cards and system light/dark colors. It does not implement shared wiki themes or
replace the approved chat layout. Check wide/narrow and light/dark before
handoff. Keep the existing visual baselines.

The video's trained micro-turn model remains a separate, deferred experiment.
No new model or adapter is downloaded or activated here.
