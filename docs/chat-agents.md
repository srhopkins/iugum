# Reusable wiki chat

`iugum wiki SPACE` remains a standalone wiki. `iugum agent run --config FILE`
adds the optional **Agent chat** plug. Controls are named Chat, Send, and Close
chat. The Agent selector displays configured identities and model labels.

The default entry is the native agent defined by the configuration. Optional
`chat_agents` entries connect other agents over ACP (Agent Client Protocol):

```yaml
name: coordinator
runtime: native
listen: 127.0.0.1:3850
wiki_dir: wiki
wiki_port: 3050
data_dir: data
chat_agents:
  local:
    name: Local helper
    command: [opencode, acp]
    cwd: ./local-work
    model: ollama/gemma4:12b-mlx
    allow_tools: false
```

The example model must already exist in the configured OpenCode Ollama provider.
Ollama serves the model; OpenCode provides ACP. Other ACP executables can be used
with a configured argv list, without shell interpolation. Optional `env` entries
configure the child process. Selectable model IDs must be supported by that agent's
`session/set_config_option` model option. There is no silent fallback on errors.

Each choice stores its history, ACP session reference, and reported usage in
`data/chat-agents/ID`. Switching preserves each draft in the browser. It does not
copy another agent's chat or Chief's private memory. The ACP agent must support
`session/load` to continue its saved conversation across process restarts.

ACP commands are passed through to the selected agent; native Chief commitment
commands are not injected. Wiki search remains a shared, policy-checked service.
Every prompt receives the current timestamp. The adapter records reported token
usage in `usage.jsonl`; missing usage is null, not zero. It does not estimate prices
or enforce native model-pool budgets for an external ACP process.

Policy resources use `agent/acp/ID/`: `run`, `model/MODEL`, `agent/select`, and
`tool/TITLE`. Tool permission requests are declined unless `allow_tools: true`
and current policy allow the request. Only an `allow_once` option is selected.
Client-provided filesystem and terminal capabilities are not advertised or
implemented. This is a protocol permission boundary, not an OS sandbox: the
configured external executable retains its own process privileges.

The Go integration uses `github.com/coder/acp-go-sdk`. Standard Markdown rendering,
search, keyboard editing, and sticky prompts are shared by all choices.
Run `scripts/chief-fe-check.sh` for browser regressions and
`CGO_ENABLED=0 go test ./agentacp ./agentdesk .` for protocol and routing tests.

For compatibility the source directory and CSS selectors still use their original
`plugs/chief` names. The installed plug and command are **iugum-agent-chat** and
**Chat: Open**. Installation retires the old managed plug to a `.previous` backup.
