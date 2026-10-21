# Publication requests and forms

Read the relevant sections when preparing publication, commit/tag inputs, or a request with separate permissions or choices.  Ordinary approval needs only a clear operation, destination, and reviewed content.

## Operation details

| Action | Identify before review |
| --- | --- |
| Push | Repository, resolved push URLs, fully qualified source/destination refs, full commit or tag IDs, introduced changes, and CI or deployment effects. |
| PR, issue, comment, or message | Service and destination, operation (create or edit), existing item/thread when applicable, exact title/body, and applicable recipients, labels, visibility, attachments, or scheduling. |
| Release or deployment | Candidate identity, environment, exact notes or artifacts, ordering and dependencies, material impact, and rollback where applicable. |
| Local commit or application | Exact reviewed changes and message or target paths; state explicitly if approval also requests execution. |
| AI review, when authorization is missing or expands | Tool, model, service, task and permitted material, investigation roots, local findings destination, follow-up scope, and limits.  Reuse existing authorization; bind each execution to exact snapshots in its assignment as described in [AI review authorization](../../critty-ai-review/SKILL.md#establish-the-authorization-scope). |

If the review will create a commit, identify it as `reviewed-commit`, record its full parent SHA (`null` for an initial commit), and attach its exact message alongside the reviewed diff.  Record the resulting SHA separately after creation.  For tags to create, specify the target, annotation, and signing requirements.  A changed parent, diff, message, or tag input needs renewed approval.

## Publication metadata

When publication inputs need structured metadata, use YAML frontmatter with block-style mappings and sequences.  Field names are task-specific authoring conventions, not a Critty-enforced schema.  Separate execution inputs from review-only context.

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

For ordinary approval, `APPROVAL.md` can be:

```markdown
Create the issue in example/project using the exact title, labels, and body in the attached PUBLICATION.md.
```

Attach every referenced payload; a path alone is not a reviewed snapshot.  Map only approved service fields to API inputs, leaving review-only metadata such as `operation` out.  Publish only the body.  A commit-message attachment contains the exact message, with approval metadata in a separate request.

## Forms and choices

Use ordinary review approval when no separate answers or permissions are needed.  For preferences or scope selection, collect ordinary answers with `--answers --result <directory>/answers-round-1.json`, using the returned scratch directory.  For example:

```markdown
Which tools should move to mise, and which version policy should they use?

- [ ] Move the formatter to mise. <!-- form: id=formatter -->
- [ ] Move the cloud CLI to mise. <!-- form: id=cloud-cli -->
- [ ] Follow the latest release. <!-- form: id=versions value=latest -->
- [ ] Pin a version. <!-- form: id=versions value=pinned -->
```

These fields collect answers, including partial answers, without granting permission to change the environment.  Keep independent selections editable across rounds.  Read submitted answers even when `verdict: comment` and `approved: false`; use them for preparation already authorized by the caller.  Present concrete changes or operations for any separately required approval, and pass `--no-answers` when reusing the session for that review.

For individual execution permissions, `consent` gates an operation and a choice selects a reviewed alternative.  Use `required` only when an answer or consent is a prerequisite to approval:

```markdown
- [ ] Publish as a draft PR. <!-- form: id=mode value=draft required -->
- [ ] Publish as a ready PR. <!-- form: id=mode value=ready required -->
- [ ] Create the PR with the selected mode and attached payload. <!-- form: id=create-pr consent -->
```

Use the same ID and distinct `value` attributes on all options of a single choice; mark every option `required` when selection is a prerequisite to approval.  Spell out how each choice maps to the operation (here, `draft` sets the PR's draft flag to true, `ready` sets it to false).  Attach all alternative payloads when their text differs.  Limit execution to the reviewed alternatives; a selection requiring newly drafted text needs a new review of that text.

Give independently authorized operations separate consent IDs.  Add `required` only when permission is a prerequisite to approving the request; otherwise approval may decline that operation.  Leave confirmations and consents unchecked in the source.  Initial checkmarks, ordinary checkboxes, and choices do not authorize actions.

Read [attachment forms](../../critty-cli/references/forms.md) for widget and answer-retention contracts before revising a form.  Keep field wording, definitions, and explicit IDs stable; add explanations in surrounding prose or comments.  Changed fields need new answers, and new candidates remain unanswered even after an earlier select-all.
