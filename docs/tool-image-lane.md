# Typed PNG tool output into the existing model path

This is source-only. No model/browser requests, capture, configuration change or
runtime enablement is authorized. It adds no screenshot provider or new model API.

Keep Tool.Execute returning string for existing tools. Add optional ExecuteResult
returning ToolResult{Text,Images}; runtime prefers it when present. Existing text
execution, authorization and policy order remain unchanged. An image-aware MCP
tool exposes both entry points; a legacy string consumer encountering images
gets an explicit unsupported-image error, never JSON/base64 truncation.

The MCP adapter recognizes actual ImageContent, allows at most one PNG image and
8MiB decoded data per result, and validates MIME/signature/CRC/chunks/trailing
bytes/dimensions/pixels/full decode. Add a NEW pure exported ValidatePNG wrapper
file in browsergrant using the existing strictPNG helper and shared8MiB/16MP/
8192dimension limits; do not edit frozen authority/source files. Agentcore uses
that validator. Preserve original PNG bytes (no reencode), compute SHA256 and
keep observation/tool metadata in Text. Across the entire run/model context,
cap decoded PNG bytes at16MiB and image count at16; cap the final image-bearing
model JSON request at32MiB. Reject aggregate overflow explicitly before the next
model request; never discard, substitute or silently evict images/metadata.
These aggregate limits are rechecked on every continuation/fallback. With images, all nonimage MCP result data
(including structured content) remains JSON text bounded64KiB and rejects overflow
instead of truncating metadata. Plain text results retain legacy32k truncation.

Add explicit MCP allow_images option (defaultfalse), which permits this typed
lane and bounded12MiB response wire size ONLY for that configured MCP transport.
Default MCP wire limit stays4MiB. Image results without opt-in fail clearly.
Wire overflows reject rather than silently cutting image bytes; SDK decode and
strict base64/PNG validation remain mandatory. Errors include no image payload.

Message retains its existing Content string, with optional in-memory Images
marked json:"-". The actual request adapter translates these only for outgoing
model wire messages. Normal JSON/history and usage logs do not serialize PNGs;
actual native history currently saves only human/finalassistant messages.

For OpenAI-compatible Chat Completions, TOOL content remains TEXT with original
tool_call_id and metadata. After ALL pending tool responses have been appended,
append clearly labeled UNTRUSTED multimodal USER context containing a text part
that names the original tool/tool_call_id and the matching observation metadata,
plus image_url:{url:"data:image/png;base64,<exact original bytes>"}. Never put an
image part in TOOL content. This is tool evidence, not a fresh human request;
retain the original human request and explicit untrusted-output instruction.
There is no arbitrary remote image URL or image fetching. Batch order is tested.

Add profile vision:true opt-in (defaultfalse) for the existing OpenAI-compatible
wire adapter. Recheck the ACTUALLY selected profile on every model continuation,
including budget fallback. If images are present and selected profile lacks
vision support, fail explicitly before that model request; no silent strip,
automatic model switch or guessed capability. All image data is validated again
before serialization. Codex subscription/decision adapters remain text-only:
reject image context or image tool result explicitly before another CLI/model
invocation. Existing text adapters and text-only calls remain unchanged.

Tests: failfirst typedMCP ImageContent→actualToolResult→actual Runtime.Run second
HTTP request contains exact data-URI PNG bytes/hash and full observationmetadata;
assert tool_call_id/text-only tool role, labeled user image context after all
batch responses. Real SDK syntheticMCPserver + synthetichttptest model only.
Test text tools unchanged, opt-in denial, unsupported subscription/profile and
fallback zero secondmodelcalls, overflow/malformed/base64/MIME/SVG/HTML/CRC/
trailing/active PNG chunks rejection,8MiB/16MP bounds and cancellation. No real
provider/environment credentials. Separate current independent source review
required before owner closure; immutable earlier release artifacts untouched.
