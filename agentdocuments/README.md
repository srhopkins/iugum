# Working commitments

The configured Markdown document is authoritative. The JSON workspace stores chat and a migration counter, not a second copy of commitments.

Each commitment is a top-level paragraph with a stable Atomdown identity. `iugum-kind="commitment"` selects it; `iugum-ref` preserves a readable legacy reference such as `c1`. Visible fields carry the human work:

```markdown
<!-- <atom id="4P8W2H6K" iugum-kind="commitment" iugum-ref="c1"/> -->
Title: Deliver clear ticket drafts  
Status: focus  
Due: 2026-09-12  
Updated: 2026-09-10T09:00:00-07:00
```

Status is `open`, `focus`, `deferred`, or `done`. Due is an explicit `YYYY-MM-DD` civil date, or `none`. Callers resolve relative dates such as “today” in the user's configured timezone before writing. Dates do not silently become UTC midnight deadlines.

Reads reparse the working document, so human edits appear on the next status or context read. Writes preserve other document content, unchanged directive bytes, extension attributes, and unknown lines in the commitment paragraph. Existing review digests are not refreshed: text changes correctly invalidate a prior review. A pre-write comparison rejects a detected concurrent document edit. The store assumes one iugum writer; it is not a cross-process transaction manager.

The implementation uses the published Atomdown parser and ID generator. Small surgical writes avoid rebuilding unrelated Markdown. Beads links can be preserved as extension attributes; automatic Beads synchronization is not implemented by this package.
