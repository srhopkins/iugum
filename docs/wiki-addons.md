# Optional wiki add-ons

Run the wiki without an agent home:

```sh
iugum wiki --addons /path/to/wiki-addons.yaml /path/to/notes
```

The ordinary `iugum wiki` command remains available. An explicit add-ons file enables chat and search independently. Paths inside that file resolve relative to its directory. The supplied wiki space remains the positional CLI argument. Keep the add-ons configuration, data directory and agent homes outside the served notes space; the command rejects paths inside it.

```yaml
search: true
chat: false
data_dir: .iugum-wiki
policy_file: wiki-policy.csv
```

The initial search backend searches wiki Markdown text and page names, with all-word matching and `-word` exclusions. It is a bounded direct file search, not an embedding index. Transcript search remains available in the existing native-agent workspace; the standalone wiki does not silently import private transcripts.

For chat, set `chat: true` and configure ACP connections:

```yaml
search: true
chat: true
data_dir: .iugum-wiki
policy_file: wiki-policy.csv
agents:
  local:
    name: Local
    command: [opencode, acp]
    cwd: ./work
    model: ollama/gemma4:12b-mlx
    allow_tools: false
```

Choose an installed model and authenticated ACP executable. The wiki uses its configured actor for wiki operations and each connection ID for that agent's policy checks. Read `docs/chat-agents.md` for ACP limits. External processes are not operating-system sandboxed by Casbin.

The managed plugs are `_plug/iugum-agent-chat.plug.js` and `_plug/iugum-search.plug.js`. Setting a feature false retires its plug to `.disabled` and disables its backend route. Removing a plug removes that interface on reload; set the configuration false to disable its backend too. Upgraded plug copies are preserved as `.previous`.

The add-ons currently require loopback binding. The front URL proxies one internal SilverBullet server; there is one notes space. No Chief configuration or model is needed for search. The public agent-home lifecycle design is tracked separately in iugum-01r.

## Attach a running managed home

```yaml
chat: true
search: true
homes:
  assistant: /private/agents/assistant
```

Start that home with `iugum agent start --home /private/agents/assistant`. The picker connects to the same HTTP conversation used by CLI attach. Each request resolves the current runtime record, so a stopped agent reports unavailable rather than silently creating a new conversation. Configure a grant for the wiki actor to `attach` to `agent:assistant`; the destination also applies its own policy. The picker offers Start agent and Stop agent for configured managed homes. Those actions require their own policy grants. The Agents link opens a full page with availability and managed-home controls. An archived-agent registry remains follow-up work.

## Chat prompt colors

Past user prompts default to light blue with dark blue text. Themes or SilverBullet
Space Style can override these CSS properties on the root element; they inherit
into the chat panel:

```css
:root {
  --iugum-chat-prompt-background: #e7f2ff;
  --iugum-chat-prompt-color: #17365b;
}
```

Set both colors for contrast when defining a dark theme. Use an opaque background
so replies do not show through pinned prompts. Colors apply to collapsed, expanded,
and pinned prompts. Prompt width and scrolling behavior are unchanged.

Drag the chat panel's left edge to change its width. The resize handle also accepts
Left/Right arrow keys when focused. Resizing preserves the full-width prompts;
pinned prompts meet the top of the scroll area so replies cannot appear above them.

Long prompts fade at the bottom when collapsed. Click the prompt card, or focus it
and press Enter or Space, to expand or collapse it. Short prompts stay fully visible.
Links and text selection keep their normal behavior. Replies fade as they pass
under a pinned prompt; this fade uses the theme's page background.

Search keeps its source/project filter behind the **+** options button. The query
bar keeps its width and expands downward on focus; results remain attached below it and reopen without a new
request when the unchanged query is focused again. Click outside to dismiss.
The options menu exposes existing search filters; it does not add upload or model tools.

The conversation navigator sits at the chat's right edge. Each mark represents a
turn. Hover or focus a mark to preview its prompt; click or press Enter/Space to
jump there. The active turn is highlighted as you scroll. Switching agents clears
and rebuilds the navigator from that agent's history.

Chat controls live at the far right of the top menu: history jumps to recorded
prompts, more options opens Agents or Commitments, and the panel icon toggles chat.
Search icons use native action-button colors and hover colors. Themes can override
`--iugum-search-icon-border` and `--iugum-search-icon-background`; both default to
an unbordered, transparent appearance. Search keeps its modest 9px corner radius.

The expanded search row offers All (default), Wiki, Chats, and Filters. Chats uses
the transcript source; Filters exposes the project/source field and exclusion help.
