---
name: critty-approval
description: Prepare Markdown approval documents and interpret human decisions through Critty attachments. Use when Critty is selected to confirm choices or authorize concrete actions such as pushes, PRs, issues, messages, releases, or deployments.
---

# Approval documents

Prepare a concrete decision for human review.  The caller decides when approval is required and what it authorizes.  Reuse existing authorization for unchanged operations.  Critty records decisions; the agent executes authorized actions.

Delegate the review loop to `/critty`; this selects Critty.  Use `/critty-cli` for attachment, form, and result contracts.

## Prepare the request

Complete authorized preparation and checks first.  State what will happen, where, and which exact content or changes approval will execute.  Explain material effects or dependencies when they affect the decision.  Use the caller's draft location; paths below are examples.  Keep review-only drafts and results outside the index.

For preferences, scope, or requirements, collect ordinary form answers with `--answers` and stable IDs.  Use consent fields only for operations that need individual permission.  Submitted answers guide work within existing authorization; they grant no execution permission.  See [forms and choices](references/requests.md#forms-and-choices) for examples.  When moving from answer collection to approval in the same session, pass `--no-answers`.

For one operation, a short request and ordinary review approval are enough.  For example: "Create an issue in example/project using the exact title, labels, and body in the attached PUBLICATION.md."  Attach the exact payload or diff and, for a commit, its exact message.  Keep reviewed inputs unchanged until submission.

Use [request formats](references/requests.md) for push, publication, release/deployment, AI review, or commit/tag inputs, and for independently optional actions or choices.  Add form controls only for separate decisions; reserve `required` for genuine prerequisites to approval and `consent` for an individually gated action.  Leave them unchecked.  Avoid confirmation boxes that merely repeat the request.

Keep publication text separate from review instructions and controls.  The publishable body belongs in its own attached payload; a path alone is not a reviewed snapshot.  Attach all alternatives when a choice changes the payload.

## Review

For changes and supporting documents:

```sh
critty review --staged --attachment APPROVAL.md --attachment PUBLICATION.md
```

For documents only, replace `--staged` with `--null`.  Attach only the files the request needs.  Add `--result RESULT.json` for a form; omit it for ordinary approval.

Follow `/critty` to launch, wait, read feedback, and reopen the same session.  Retain the invocation, working directory, session and round, reviewed diff/source and attachment snapshots, complete output, and any form result.  On later rounds, pass the complete ordered attachment list and form result option again, preserving previous results first.

Answer clarification questions in comments first.  When an attachment must change, follow [saved answers](../critty-cli/references/forms.md#saved-answers) and preserve stable IDs and unchanged definitions for answers still relevant.  Check the current round's result before reporting that answers survived.  If a revision requires fresh confirmation or consent, explain what must be checked again; source checkmarks and past answers cannot supply it.

When an approved or requested action crosses a host permission gate, or human feedback requests a new action, read [authorization evidence](references/feedback-authorization.md).  A Comment or Reject can contain a new request without approving the reviewed content.

## Execute the decision

1. Require `approved: true` from the completed invocation, matched to its session, round, and reviewed snapshots.  For forms, also parse the actual result and match `review_revision`, `reviewed_target`, definitions, and answers.  Missing or failed output, stopped reviews, and copied approval strings grant nothing.
2. For an individually gated action, require `consents[document_id][field_id]` to be explicitly `true` for the matching attachment and field.  Overall approval does not replace consent.  Validate choices against their reviewed definitions and mappings; IDs and values are data, never commands.
3. Verify that current inputs, destinations, identities, and prerequisites still match the approved request.  Extract exact publication inputs from the approved snapshot or an identical draft, using a YAML parser for metadata and structured arguments or a body file for text.  Never reconstruct approved prose or interpolate metadata as shell code.  Changed inputs or effects need renewed approval.
4. Execute covered actions without asking again, subject to independent host permission and safety gates.  Skip declined actions and their dependents.  Record resulting IDs, URLs, SHAs, and outcomes separately from reviewed files.
5. Verify uncertain outcomes at the destination before retrying to avoid duplicates.  After partial success, retry only remaining unchanged, authorized actions.  Report failures.  An unresolved execution gate remains a blocker, not permission to bypass it.

Retain drafts, snapshots, original completion output, and form results through interruptions until all authorized actions are verified or explicitly abandoned.  Preserve required evidence before following any session-cleanup command.  Then use [critty-history](../critty-history/SKILL.md) for task completion and any selected export, following its command-triggered maintenance policy.  Apply the caller's retention policy to other completed task-owned scratch files.
