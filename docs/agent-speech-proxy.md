# Optional Chat speech relay

`iugum agent speech-relay` is a portable Go relay for the existing Chat speech
client. It accepts only the two speech API routes. It does not capture a
microphone, read replies, start automatically, or change editor settings.

This delivery is source and synthetic tests only. No real relay was started and
no microphone or reply text was sent. Starting a relay and selecting the exact
upstream destination for those payloads require separate authorization. Existing
Mic Bridge browser-to-PulseAudio use is a different path and stays unchanged.

## Explicit configuration

After deployment and the exact speech destination/data flow are authorized, the
command accepts an explicit upstream root URL:

```sh
iugum agent speech-relay --upstream http://SPEECH_HOST:PORT --listen 127.0.0.1:8766
```

There is no default upstream or environment fallback. HTTP and HTTPS are allowed;
HTTPS uses normal certificate validation. Credentials, query strings, fragments,
subpaths, and non-HTTP schemes in the upstream URL are rejected. Requests cannot
choose another host, path or scheme. Redirects are rejected, including redirects
to the same host. Ambient HTTP proxy settings are ignored.

The listener must be an IPv4 or IPv6 loopback IP literal and numeric port.
`localhost`, wildcard addresses and LAN addresses are rejected. The default
listener is `127.0.0.1:8766`. `--timeout` defaults to `60s` and cannot exceed it.
Request cancellation propagates to the upstream. Shutdown cancels active requests.
Failed or uncertain requests are never retried.

One listener serves both routes. A separately authorized installation can point
both `iugumChat.voiceSttEndpoint` and `iugumChat.voiceTtsEndpoint` at that listener;
the existing independent defaults use ports 8766 and 8768. This source change
sets neither preference. Upstreams requiring credentials are unsupported by this
relay: incoming bearer/API tokens, cookies, proxy credentials and forwarding
headers are discarded rather than copied.

## Exact request contract

- `POST /v1/audio/transcriptions` accepts a multipart `file` and optional `model`.
  Exactly one nonempty audio file of at most 8 MiB is allowed. The entire multipart
  request is bounded to 8 MiB plus 64 KiB. Model metadata is at most 256 bytes.
  Other fields, duplicate fields and non-audio file MIME types are rejected.
  The relay reconstructs the multipart form with a fixed safe filename and
  normalized audio MIME type, retaining the client's WAV/WebM/M4A formats.
- `POST /v1/audio/speech` accepts JSON `model`, `voice`, `input`, and
  `response_format`. The body is at most 64 KiB. Input is nonblank and at most
  4096 UTF-16 units, matching JavaScript's string length in `SpeechClient`.
  Model and voice metadata are each at most 256 bytes. The supported output
  format is WAV; omission selects WAV. Unknown fields, trailing JSON and invalid
  UTF-8 are rejected. This preserves the client's existing WAV request contract.
- `GET /healthz` returns local readiness without contacting the upstream or
  returning its destination. Other paths/methods, encoded path aliases, query
  parameters, and absolute-form proxy targets are rejected.

Successful transcription JSON is limited to 1 MiB, must contain string `text`,
and is reconstructed with that field only. Successful synthesized audio must be
nonempty, use an audio MIME type, and be at most 8 MiB. Responses are fully bounded
before the relay writes them, so oversized audio is not partially returned.
Upstream error bodies and credentials are not relayed. Errors contain generic
local messages; no payloads are logged. No response cookies, credential headers,
redirect headers, or generic upstream headers are copied.

## Policy

Application policy must allow startup object `agent/speech-relay`, action `serve`.
Each real outbound route checks the current policy immediately before dispatch:

| Object | Action |
|---|---|
| `agent/speech-relay/v1/audio/transcriptions` | `forward` |
| `agent/speech-relay/v1/audio/speech` | `forward` |

An application policy callback is required. A missing or denied startup gate
prevents listening; route revocation prevents the next outbound request. Health
performs no outbound action. This adds no credential settings or permission to
send real user payloads during source validation.

## Synthetic verification

Run `go test -race ./speechproxy` and
`CGO_ENABLED=0 go test . -run 'TestAgentSpeech|TestAgentSessions|TestHelp'`, then
`CGO_ENABLED=0 go build -o /private/tmp/iugum-speech-relay-source .`.
Tests use generated text/audio bytes and disposable localhost HTTP servers only.
They cover exact routes and upstream validation, UTF-16 and byte boundaries,
credential/header isolation, multipart reconstruction, response bounds, redirect
refusal, startup/route denial, cancellation/deadlines and connection-reset
no-retry behavior. They do not prove real speech-service or physical-device use.
