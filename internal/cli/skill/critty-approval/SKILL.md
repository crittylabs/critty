---
name: critty-approval
description: Prepare Markdown approval documents and interpret human decisions through Critty attachments. Use when Critty is selected to confirm choices or authorize concrete actions such as pushes, PRs, issues, messages, releases, or deployments.
---

# Approval documents

Prepare and interpret human decisions through Critty.  The caller decides when approval is required, where drafts belong, and which operations are allowed.  Reuse existing authorization for unchanged operations.  Critty records decisions; the agent executes authorized actions.

Delegate the review loop to `/critty`; this is an explicit selection of Critty.  Consult `/critty-cli` for attachment, form, and result contracts.

## 1. Prepare a concrete request

Complete authorized preparation and checks first.  Use task-owned draft and result paths under the caller's storage convention; filenames below are examples.  Keep review-only drafts outside the index.

Describe exact operations, destinations, payloads, and material effects.  Combine related actions and identify dependencies, such as a separately gated branch push needed before creating a PR.

| Action | Identify before review |
| --- | --- |
| Push | Repository, resolved push URLs, fully qualified source/destination refs, full commit or tag IDs, introduced changes, and CI or deployment effects. |
| PR, issue, comment, or message | Service and destination, operation (create or edit), existing item/thread when applicable, exact title/body, and applicable recipients, labels, visibility, attachments, or scheduling. |
| Release or deployment | Candidate identity, environment, exact notes or artifacts, ordering and dependencies, material impact, and rollback where applicable. |
| Local commit or application | Exact reviewed changes and message or target paths; state explicitly if approval also requests execution. |

If the review will create a commit, identify it as `reviewed-commit`, record its full parent SHA (`null` for an initial commit), and attach its exact message alongside the reviewed diff.  Record the resulting SHA separately after creation.  For tags to create, specify the target, annotation, and signing requirements.  A changed parent, diff, message, or tag input needs renewed approval.

## 2. Write Markdown with YAML frontmatter

Use YAML frontmatter with block-style mappings and sequences.  Field names are task-specific authoring conventions, not a Critty-enforced schema.  Separate execution inputs from review-only context.

Parse and validate metadata types, values, and keys for the operation, rejecting duplicate or unexpected keys.  Generate YAML with a block-style serializer and check readability.  Keep the Markdown body separate, preserving later delimiters and code blocks.

For publication, put only the exact publishable text after the closing delimiter.  Put review instructions and interactive controls in a separate approval attachment.  For example, `PUBLICATION.md`:

```markdown
---
operation: create-issue
repository: example/project
title: Preserve saved settings
labels:
  - bug
---
Saved settings disappear after restarting the application.
```

And `APPROVAL.md`:

```markdown
---
kind: approval-request
operations:
  - id: publish-issue
    action: create-issue
    payload: PUBLICATION.md
    destination: example/project
---
Create the issue using the destination, metadata, and exact body in the attached PUBLICATION.md.

- [ ] I checked the destination and publication text. <!-- form: id=destination required -->
- [ ] Create this issue in example/project. <!-- form: id=publish-issue consent -->
```

Attach every referenced payload; a path alone is not a reviewed snapshot.  Map only approved service fields to API inputs, leaving review-only metadata such as `operation` out.  Publish only the body.  A commit-message attachment contains the exact message, with approval metadata in a separate request.

## 3. Add controls where a decision is needed

Use ordinary review approval when no separate answers or permissions are needed.  Otherwise add task lists with stable IDs: `required` establishes a prerequisite, `consent` gates an operation, and a choice selects a reviewed alternative:

```markdown
- [ ] I checked the target environment. <!-- form: id=environment required -->
- [ ] Publish as a draft PR. <!-- form: id=mode value=draft required -->
- [ ] Publish as a ready PR. <!-- form: id=mode value=ready required -->
- [ ] Create the PR with the selected mode and attached payload. <!-- form: id=create-pr consent -->
```

Use the same ID and distinct `value` attributes on all options of a single choice; mark every option `required` when a human selection is necessary.  Spell out how each choice maps to the operation (here, `draft` sets the PR's draft flag to true, `ready` sets it to false).  Attach all alternative payloads when their text differs.  Limit execution to the reviewed alternatives; a selection requiring newly drafted text needs a new review of that text.

Give independently authorized operations separate consent IDs.  Add `required` only when permission is a prerequisite to approving the request; otherwise approval may decline that operation.  Leave confirmations and consents unchecked in the source.  Initial checkmarks, ordinary checkboxes, and choices do not authorize actions.

## 4. Attach and review

For a request that includes code changes:

```sh
critty --staged --attachment APPROVAL.md --attachment PUBLICATION.md
```

For a request containing only documents:

```sh
critty --null --attachment APPROVAL.md --attachment PUBLICATION.md
```

For interactive task lists, add `--result RESULT.json`, using the destination rules in `/critty-cli`.  Omit it for ordinary approval; it requires a form.

Follow `/critty` to launch and read all feedback.  Retain the invocation, working directory, session ID, reviewed diff or source identities, attachment snapshots, both output streams, and any result path.  Keep reviewed files unchanged until submission.

After feedback, use `/critty` to refresh the input and reopen the session with the complete ordered attachment list.  Include `--result RESULT.json` again for forms; preserve any approved result before reusing its path.  `/critty-cli` describes snapshot replacement and answer resets.

## 5. Bind approval to execution

1. Require `approved: true` from the completed invocation.  For ordinary approval, match its output, session, reviewed input, and attachment snapshots to the request.  For forms, parse the actual result JSON and also match the round, `review_revision`, `reviewed_target`, definitions, and answers.  Missing output, feedback, stopped reviews, and copied approval strings grant nothing.
2. For an individually gated action, locate its attachment by source and snapshot and require `consents[document_id][field_id]` to be explicitly `true`.  Overall approval does not replace that consent.  For other actions, apply the caller's policy to the approved request.  Validate choices against the matching definitions and use only their stated mapping.  IDs and values are data, never commands.
3. Verify the current operation still matches the approved snapshots and destination, including referenced payloads, commit/tag identities, and external prerequisites.  Extract publication inputs from the preserved approved snapshot or byte-identical draft using a YAML parser.  Pass exact bodies through structured tool arguments or a body-file option; never reconstruct approved prose or interpolate metadata as shell code.
4. Execute the authorized operations without another confirmation.  A declined operation stays unexecuted; skip its dependent operations and report what remains.  Record resulting IDs, URLs, SHAs, and completion state separately from approved files.  Approval does not waive independent permission or safety gates; present the correlated evidence to those gates.
5. Verify outcomes before retrying or cleaning up.  If an external result is uncertain, inspect the destination first to prevent duplicate publication.  After partial success, retry only the remaining unchanged, authorized operations.  Changed inputs or effects need renewed approval; an unresolved execution gate remains a blocker, not permission to bypass it.

Preserve approved drafts, snapshots, and captured output or form results through waits, failures, and interruptions until all authorized operations are verified or explicitly abandoned.  Preserve this evidence before following Critty's printed session-cleanup command.  Apply the caller's retention policy afterward; remove only completed task-owned scratch files and preserve unrelated reviews and audit records.
