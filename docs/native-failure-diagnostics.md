# Native turn failure diagnostics

The native runtime keeps hard tool failures fatal. It does not retry tools or
replay queued editor actions. Existing typed argument correction remains limited
to execution errors marked as InvalidToolArguments.

The private editor diagnostic snapshot adds an optional failure object with a
fixed category, the registered tool name, and the actual model step at the abort
branch. A queued sibling without a tool result remains unproven. Application
feedback shows the same bounded metadata outside assistant conversation text.
Raw causes stay in the error chain and never enter this metadata.

Categories distinguish policy checks, authorization checks, execution, transport,
MCP calls, remote tool errors, invalid JSON, unknown tools, canceled turns, and
timeouts. Unknown failures remain runtime_error. Unknown model-authored tool
names are omitted.

The installed editor server has no structured rejection codes. For only its
named editor tools, exact single-text remote errors map to fixed categories:
editor_instance_disconnected, editor_command_id_reserved,
editor_capability_unavailable, editor_queue_full, and editor_receipt_not_found.
Changed, additional, or unknown text stays tool_remote_error. These observations
do not grant permission or change recovery. The editor host service is unchanged.

Subscription-provider failures retain their existing generic runtime category.
This diagnostic addition cannot recover a historical cause that was not saved.
