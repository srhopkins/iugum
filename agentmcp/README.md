# MCP tool source

`agentmcp` connects explicitly configured Model Context Protocol (MCP) servers through the official Go SDK. This first adapter supports Streamable HTTP tool discovery and calls. It does not launch local processes, discover unconfigured servers, or enable remote prompts, resources, sampling, or OAuth flows.

```yaml
mcp:
  - name: example
    url: https://tools.example.org/mcp
    auth_env: CHIEF_EXAMPLE_MCP_TOKEN
```

`auth_env` is optional. When configured, its environment value supplies the Bearer token; an empty value fails startup. Do not put credentials in the URL. No endpoint is enabled by default.

Call `Connect(ctx, configs, actor, gate)` to obtain native `agentcore.Tool` values and an idempotent cleanup function. Close on shutdown; cancellation also closes sessions. Configured server connection or discovery failures are startup errors identifying the server. The adapter does not silently omit an unavailable source.

Policy checks use these objects and actions:

| Object | Action | When |
| --- | --- | --- |
| `mcp:example` | `connect` | Before connection and again before every tool call |
| `mcp:example` | `list` | Before each discovery page |
| `mcp:example:remote_tool` | `call` | Immediately before the remote call |

A remote tool becomes `mcp_example__remote_tool`. Server names permit letters, digits, underscores, and hyphens up to 24 characters; tool names use the same characters up to 30. Unsupported or duplicate names fail clearly. Tool schemas are preserved. Remote descriptions and outputs remain untrusted input, not authorization.

HTTP redirects and automatic reconnect retries are disabled. Responses are bounded to 4 MiB, returned result text to 32,000 bytes, discovery to 100 tools and ten pages, and HTTP requests to 45 seconds. Oversized result text carries a truncation notice. Tool calls receive the caller's cancellation context. Transport failures are not retried automatically, because a remote mutation may already have occurred.

Implementation references: official SDK `examples/http/main.go`, `mcp/streamable.go`, and `mcp/protocol.go`, version 1.7.0. Tests use a local synthetic SDK server and verify policy revocation before a second call.
