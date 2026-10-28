# Optional image attachment on the existing Codex subscription adapter

This narrowly extends the approved typed PNG lane. Local `codex exec --help`
explicitly advertises `-i, --image <FILE>...` initial prompt attachments. The
current Iugum wrapper is text-only; that is not proof the provider cannot accept
images. No authenticated CLI/model invocation is authorized by this design.

Add explicit `models.subscription_vision_models` exact model-ID allowlist,
default empty. Before EVERY invocation, the actual chosen subscription model
must be listed if image context exists; otherwise return an explicit unsupported
error. Do not guess capability from names, enable a model by default, switch
models or strip images. Keep API profile vision opt-in independent.

Reuse typed validated PNGs and perrun16MiB/16image plus8MiBperimage bounds. For an
image-capable subscription call, defensively copy and revalidate images, create
owned temporary PNG files beneath the existing0700 ephemeral invocation directory
using0600 exclusive files and fixed generated names, and append fixed repeated
`--image <owned absolute path>` arguments before the final stdin `-`. No model,
tool text, URI or page chooses a filename or CLI option. No screenshot/base64 is
inserted into normal JSON/history or prompt text. Prompt includes the matching
ordered toolcall/observationmetadata and untrusted-image-context label, original
human request and refreshed clock as before. Attachment ordering matches labeled
context ordering. No arbitrary source path, extra shell command or new process
transport is introduced.

The existing policy/context gates, argument authorization, ephemeral/read-only
sandbox, environment secret filtering and disallowed tool-event checks stay.
`--disable view_image` only disables tool execution; initial --image attachments
use the CLI's documented input path and are not claimed blocked by that flag.
Keep the fixed disabled-tool list unless independently required otherwise.

Clean files and directory on success, provider/tool error, cancellation and
startup failure. No permission is broadened; symlinks/existing files cannot be
selected. Unknown CLI versions that reject --image fail visibly through existing
process error handling, never silently retry without images. No actual CLI/model
compatibility claim is made until separately authorized runtime validation.

Fake CLI fixtures inspect the exact argv, count/order, PNG hashes,0600 files and
0700 directory while running, prompt metadata/currenthumananchor, absence of
base64/payload paths in history JSON and removal after success/error/cancel.
Text-only CLI argv remains unchanged. Joined actual SDK MCP ImageContent →
actual RunSubscriptionTools second fake CLI invocation receives original image
bytes plus matching metadata, with unsupported model causing zero nextinvocations.
All tests use synthetic raster/keys/data; no real provider or browser requests.
