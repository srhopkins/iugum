# Proposed source-only browser clock probe

This is a narrow source protocol proposal for iugum-6ltd.4.8, awaiting coordinator
approval. It does not change the frozen core or deployment bundle, enable a
browser, capture pixels, or dispatch input.

Private route: POST `/api/host/browser/clock-probe` with existing authenticated
`{instance,lease,request:{pair,nonce}}`. The lease/token remain in the native
extension host. Only the privileged companion background generates nonce: exactly
32 random bytes, canonical unpadded base64url (43 ASCII characters), before its
local monotonic and wall-clock send timestamps. It never accepts a page-chosen
nonce. The relay carries only opaque typed records and correlated replies.

Backend generates the command UUID. It verifies the exact current confirmed
pair, service/key/window/client/document/tab/origin binding and live private
editor lease, then performs the actual observe Casbin check serialized with
durable reservation. It consumes at most ONE probe ATTEMPT for that pair, including
failed/uncertain attempts; no automatic retries. Restart does not restore pairing,
and durable command tombstones are never dropped. Storage/signing/expiry failure
returns no timing proof and leaves the attempt consumed/uncertain.

The signed record has EXACTLY the v2 grant scalar fields plus `probeNonce`:
version,pair,authority,serverKeyId,clientKeyId,instance,windowLease,client,document,
tabId,origin,commandId,operation,argsDigest,observation,issuedAt,expires,probeNonce.
Version is2; operation is `observe`, argsDigest is the approved digest of
{operation:"observe",args:{}}, observation is empty, probeNonce exactly echoes
the request nonce. Expiry is at most issuedAt+1000 and the pairing lease.

Sign UTF8("iugum/browser/clock-probe/2\n") followed by RFC8785 canonical record,
using the paired server Ed25519 key. Envelope is {record,signature}, with canonical
unpadded base64url64-byte signature. The grant-domain signature, if internal grant
reservation code creates one, is NEVER exposed, logged or returned. The durable
receipt is already consumed as clock-only before replying. Only the clock-probe
signature reaches the browser; a grant executor rejects its different domain.
No probe enters the browser command queue or calls capture/hit-test/Input.

The background accepts only the exact pending nonce+pair+full binding and verifies
the clock-probe domain signature with the pinned server key. It consumes that
pending challenge once. A proof obtained before the local request cannot predict
the nonce; a relay can delay/race after seeing it, causing only a conservative
interval or denial. No page timestamp or boolean assertion is trusted.

The signed issuedAt and the browser's send/receive timestamps produce a conservative
server-minus-local offset interval, inflated for timestamp granularity. The
receiver must apply worst-case bounds, its reviewed <=100ms clock uncertainty
rule and monotonic request budget; never assume zero offset, use an unverified
midpoint, or reset a budget on delayed delivery. The companion clock implementation
and exact shared timing fixtures require separate current source review.

After this one no-action proof, ordinary grants remain coupled to exact agent
queued commands and cannot be minted with page-chosen UUIDs. A clock-probe cannot
be replayed as screenshot authority, as an ordinary execution grant, for another
pair/nonce, or as a terminal execution receipt.
