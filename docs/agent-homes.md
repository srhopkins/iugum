# Native agent homes and lifecycle

Create a private instance with no implicit permission grants:

```sh
iugum agent home-init assistant /private/agents/assistant
```

The home contains `agent.yaml`, `instructions.md`, `mission.md`, `policy.csv`, and private `data/`. New homes point at an empty private transcript source; importing external history requires configuring sources. Configure a model or subscription transport in `agent.yaml` using the existing native-agent schema. The namespace is `agent:<name>`; alternate namespace mappings are not supported yet. Distinct instances must use distinct names and data directories. Do not reuse one data directory across instances.

Review a proposed complete policy before installing it:

```sh
iugum agent policy-apply --home /private/agents/assistant --file proposed-policy.csv
```

The command displays current and proposed policy and defaults to No. `--yes` explicitly approves this one replacement. It is an operator CLI, not an agent tool. It currently targets the scaffold's `policy.csv`; do not use it for a home configured with a different policy path. Native mediated operations reload policy per call. An external process with unrestricted filesystem access can bypass this file boundary; operating-system enforcement remains future work.

```sh
iugum agent start --home /private/agents/assistant
iugum agent attach --home /private/agents/assistant
iugum agent status --home /private/agents/assistant
iugum agent stop --home /private/agents/assistant
```

`start` detaches the process from the terminal. `attach` sends chat messages to that running instance; `/detach` or EOF leaves it running. A completed job does not exit the service. `run --home DIRECTORY` is the foreground form for supervisors. A per-home file lock prevents duplicate runs; a random runtime capability protects the stop endpoint. Runtime records and logs are private files in `data/`.

`--open` on `start` or `run` explicitly bypasses the agent's mediated policy for that process only. It prints a warning and is not written to configuration. It does not override the operator's outer iugum policy, external platform permissions or operating-system restrictions. Persistent permission changes belong in policy, not this flag.

`iugum agent supervisor --home DIRECTORY` prints a launchd plist for review. It does not install or start the service. The executable path must remain installed at that location. Configure a stable listen address and a suitable supervisor environment before installation. Full install/remove tooling and Linux supervisor output remain future work.

`mission.md` is loaded with instructions on native model turns, so an agreed mission edit persists across restarts. Automatic mission-edit tools are not added by this increment. Schedule integration and notification delivery must reuse the existing scheduler and messaging packages; this lifecycle change does not create autonomous jobs by itself.

The `plugins` list can reference shared directories with root `plugin.json` and portable `skills/*/SKILL.md`. Skills are exposed through a policy-checked `skill_read` tool, including supporting assets confined to the skill folder. Loading does not execute scripts. Automatic plugin `mcp.json` import currently fails explicitly; native hooks and platform-specific extensions are not executed. Existing manually configured MCP servers and native tools remain available. See the Beads epic `iugum-01r` for remaining package loading, UI management and policy profile work.

## Named roles

The built-in Casbin model supports role membership (`g`) with deny taking precedence:

```csv
p, wiki-reader, wiki*, read, allow
g, agent:assistant, wiki-reader
```

This example grants only matching reads; it is not a complete runtime profile. Add each required runtime/API grant deliberately. Removing the membership line revokes that role on the next mediated call. These are ordinary Casbin policies, not a separate permission language.
