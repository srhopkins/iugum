# Managed session destination feedback

claude_send accepts only destinations managed by its Iugum process. The tool
checks session-specific send and read policy before looking up destination state.
Existing account and project checks still apply before any message is queued.

A missing authorized state snapshot, or the manager's prequeue unmanaged result,
returns fixed argument-correction feedback. A private manager-owned marker proves
the exact branch. The bridge checks its immediate type and reason. Policy errors
retain their cause through a separate authorization wrapper and cannot forge that
marker by wrapping a destination sentinel. The model can select an ID from
claude_sessions within the existing bounded turn. The feedback never includes
the selected ID, account, filesystem path, or prompt.

An empty session list does not authorize starting or reconnecting anything.
The model must use another already-authorized supported route or request
explicit managed-session start approval. Existing start approvals are unchanged.

Policy failures, account revocation, invalid identity syntax, unreadable or corrupt
state, and journal/save/write uncertainty remain hard failures. A missing-state
sentinel from a later queued operation is never treated as argument correction.
Other control actions retain their existing behavior.
