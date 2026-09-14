# Deferred agent extension contracts

These are design boundaries, not implemented capabilities. The decision record is in the wiki at `Design/Agents and wiki add-ons`; live work status is in Beads under `iugum-01r`.

## Execution backends

`agentexec.Backend` accepts an agent namespace, argv and working directory. `agentexec.Unsupported` always returns `ErrUnsupported` and never executes a process. Future implementations may mediate terminals, containers or sandboxes. The caller must check current policy before invoking the backend. Ordinary ACP agent processes do not use this interface yet, so their native tools are not represented as contained.

## Memory clones

A clone begins from a frozen, authorized slice. Its writes belong to its own namespace. Live reads reference explicitly granted source resources and must recheck policy at read time. A future merge compares base, source and destination records, reports conflicts, and applies only selected staged records. Do not diff opaque database files or silently import test conversation history.

Policy grants and revocations are separate promotion proposals. They cannot take effect merely because file or memory changes were merged. No automatic merge, Git worktree conversion or copy-on-write storage is claimed by this increment. Reuse and audit the existing clone implementation before extending it; never modify user Git metadata merely to track an agent's differences.

## Search providers

Keep the current wiki text search and transcript index as distinct implementations. A future provider interface should return stable evidence references, source identity, scope and checked time. Embeddings and ranking are optional backends, not requirements for basic wiki search. Unsupported providers must return an explicit configuration error.

## Session connections

Discovery, reading history and sending messages are distinct operations. Local files/indexes and ACP can supply different parts. Match identity using platform, account, working directory and session ID; a matching display title alone is insufficient for dispatch. Each adapter declares listing, history, resume, cancellation and live-steering support. Never silently resend after an ambiguous failure. A successful saved-session resume does not establish live IDE attachment.
