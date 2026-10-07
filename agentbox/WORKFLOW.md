# Shared browser and demo workflow

You share the visible Chrome browser and code-server editor with the person
watching the Selkies stream. Code-server is VS Code in the browser. Work in
`/config/workspace`; put demo files in `/config/recordings`.

## Browser control and code walkthroughs

Read `/config/.agents/skills/playwriter/SKILL.md` for Playwriter's current API.
The installed MCP connection uses `playwriter-mcp`, which attaches to Chrome
at `127.0.0.1:9222`. Reuse that browser so the person sees your actions.

For a code walkthrough, open the file in the code-server tab, select the lines
being discussed, and explain one change at a time. Check the visible editor
after each navigation. Use `say "explanation"` when spoken narration is wanted.
Keep the walkthrough paced for the person, and wait when they ask a question.

## Demo recording

1. Start the application and open its URL in the shared Chrome browser.
2. Ask the person to open the Selkies stream and enable its audio playback.
3. Start `agentbox-record /config/recordings/demo.mkv` in an interactive terminal.
4. Drive the application with Playwriter. Narrate with `say` if requested.
5. Stop recording with Ctrl-C in its terminal. Wait for ffmpeg to finish.
6. Run `ffprobe /config/recordings/demo.mkv` and inspect video and audio before reporting success.

The recorder consumes Selkies' H.264 socket and records the desktop audio
monitor, including synthesized speech. Keep a streaming browser connected
while recording. If the socket is absent, report the error; do not substitute
a headless recording and describe it as the shared desktop.

For screenshots, run `agentbox-screenshot /config/recordings/app.png URL_SUBSTRING`.
This captures the matching Chrome tab's viewport. With no URL filter it uses
the last tab. Playwriter can also capture a specific tab through its API.
Inspect the PNG before presenting it as evidence. These are page screenshots,
not whole-desktop screenshots or a substitute for the stream recording.

## Speech

`say "text"` uses local eSpeak NG and plays through the desktop audio server.
`SPEECH_VOICE` and `SPEECH_RATE` configure its voice and words per minute.
When `TTS_URL` is configured, `say` calls that server's `/v1/audio/speech` endpoint
instead. `TTS_MODEL`, `TTS_VOICE`, and `TTS_SPEED` configure the request.
An external voice service is optional. A failed service request is an error,
not a silent fallback. Enable audio in Selkies to hear speech.

## Accounts and persistence

Agent CLIs are installed; each person signs into their own providers.
Config, browser state, work, and recordings persist in `/config`.
Read project-specific instructions before editing a repository. Provider or
project credentials are supplied by the operator, never baked into the image.
