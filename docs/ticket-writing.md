# Tickets people can understand

A ticket declares one outcome that someone can finish and verify. Its title and first two sentences must explain the work without requiring a meeting or another document.

Use the shared template in `examples/chief/ticket-template.md` for wiki work, Beads, and Jira-ready drafts. The content contract is independent of any ticket service. Publishing is a separate authorized action; a draft is not a created Jira ticket.

## Titles and opening text

Use a verb, an observable result, and enough context to distinguish the work. Avoid titles such as “Fix issues,” “Phase 2,” “Investigate stuff,” or a bare system name.

Weak title: **Search improvements**

Clear title: **Show matching transcript passages when a user searches prior work**

Opening: “Users can remember a discussion without remembering its session. Search must return the matching passage, source session, and original timestamp so they can resume the right work.”

Describe the problem and the resulting behavior before implementation details. When investigation is the actual deliverable, name its decision and evidence: “Determine whether session steering can preserve an active Claude run.”

## Completion and boundaries

Each acceptance statement should describe a result a reviewer can observe. “Works correctly” and “Tests pass” alone do not explain the expected behavior. Automated tests provide evidence for acceptance; they do not replace it.

Separate required delivery from optional improvements. Save unrelated ideas as separate work instead of expanding the active ticket silently. A parent ticket explains the combined outcome; child tickets each produce a distinct, verifiable part. State which result a dependency supplies instead of listing unexplained IDs.

The user owns commitments and deadlines. Chief may propose scope, priority, or dates, but must label proposals clearly until the user adopts them. Describe blockers and unknowns plainly without turning guesses into requirements.

## One working document

Keep the working text in its original document as questions are answered and decisions change. Atomdown supplies stable block identity; human-readable text supplies the meaning. Link a published ticket back to the source block and preserve its identity on updates. Do not create a new document version merely to clean up wording.

For a task adapter, map the shared content to its native title, description, status, and links. Do not require Jira-specific fields in the human template. If an adapter needs extra metadata, keep that mapping in the adapter and show only fields the reader needs.

Before presenting a draft, verify that the title distinguishes this task, the opening states the outcome, acceptance is observable, and all facts have evidence. Keep unanswered questions explicit. Remove template placeholders and empty headings.
