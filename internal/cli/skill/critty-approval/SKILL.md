---
name: critty-approval
description: Prepare Markdown approval documents and interpret human decisions through Critty attachments. Use when Critty is selected to confirm choices or authorize concrete actions such as pushes, PRs, issues, messages, releases, or deployments.
---

# Approval documents

Use this workflow when a task needs human decisions or authorization through Critty.  It defines how to present and bind those decisions; the caller's instructions determine when approval is required, where drafts belong, and which operations are allowed.  Reuse existing authorization for unchanged operations.  Critty records decisions locally and executes no approved operations itself.

Use `/critty` for terminal setup, launching, waiting, feedback, and subsequent rounds.  Calling it from this workflow is an explicit selection of Critty.  Use `/critty-cli` for the result schema and other CLI details.

## 1. Prepare a concrete request

Complete the authorized preparation and relevant checks first.  Choose task-owned draft and result paths under the caller's workspace or temporary-directory convention.  Paths below are illustrative filenames, not mandatory locations.  Keep drafts outside the commit's index and retain them until the approved work is verified.

Describe the exact operations, destinations, payloads, and material effects.  Include only information the reviewer needs to decide.  Combine related actions into one request, with separate consent fields when the reviewer can authorize them independently.  Identify dependencies: approval to publish a PR does not imply consent for a separately gated branch push.

| Action | Identify before review |
| --- | --- |
| Push | Repository, resolved push URLs, fully qualified source/destination refs, full commit or tag IDs, introduced changes, and CI or deployment effects. |
| PR, issue, comment, or message | Service and destination, operation (create or edit), existing item/thread when applicable, exact title/body, and applicable recipients, labels, visibility, attachments, or scheduling. |
| Release or deployment | Candidate identity, environment, exact notes or artifacts, ordering and dependencies, material impact, and rollback where applicable. |
| Local commit or application | Exact reviewed changes and message or target paths; state explicitly if approval also requests execution. |

If the review will create a commit, identify it as `reviewed-commit`, record its full parent SHA (`null` for an initial commit), and attach its exact message alongside the reviewed diff.  Record the resulting SHA separately after creation.  For tags to create, specify the target, annotation, and signing requirements.  A changed parent, diff, message, or tag input needs renewed approval.

## 2. Write Markdown with YAML frontmatter

Start each approval or publication draft with YAML between two `---` lines.  Use block-style mappings and indented sequences.  Field names are task-specific; this is an authoring convention, not a schema enforced by Critty.  Clearly separate execution inputs from review-only context.

Use a YAML parser to validate metadata and a block-style YAML serializer when generating it.  Reject duplicate or unexpected keys and validate types and values against the intended operation.  Preserve the Markdown body separately; its later delimiters and code blocks belong to the body.  Check the rendered source for readable block notation as well as valid YAML.

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

Attach every referenced payload to the same review; a path alone is not a reviewed snapshot.  Review-only `operation` identifies intent and is not automatically an API field.  Map only supported approved fields to service inputs; omit the frontmatter wrapper and approval document from the posted body.  A commit-message attachment likewise contains only the exact message, with its approval metadata in the separate request.

## 3. Add controls where a decision is needed

Use Markdown task lists with explicit, stable IDs.  A required confirmation establishes a prerequisite; a consent authorizes one described operation; a choice selects one of the stated alternatives.  These have different meanings:

```markdown
- [ ] I checked the target environment. <!-- form: id=environment required -->
- [ ] Publish as a draft PR. <!-- form: id=mode value=draft required -->
- [ ] Publish as a ready PR. <!-- form: id=mode value=ready required -->
- [ ] Create the PR with the selected mode and attached payload. <!-- form: id=create-pr consent -->
```

Use the same ID and distinct `value` attributes on all options of a single choice; mark every option `required` when a human selection is necessary.  Spell out how each choice maps to the operation (here, `draft` sets the PR's draft flag to true, `ready` sets it to false).  Attach all alternative payloads when their text differs.  Limit execution to the reviewed alternatives; a selection requiring newly drafted text needs a new review of that text.

Use separate consent IDs for independently authorized operations.  A consent may also be `required` if that permission is a prerequisite to approving the whole request; otherwise the reviewer may approve while declining it.  Leave human confirmations and consents unchecked in the source.  Critty ignores their initial checkmarks as evidence of permission.  Ordinary checkboxes and choices do not authorize actions.

## 4. Attach and review

For a request that includes code changes:

```sh
critty --staged --attachment APPROVAL.md --attachment PUBLICATION.md --result RESULT.json
```

For a request containing only documents:

```sh
critty --null --attachment APPROVAL.md --attachment PUBLICATION.md --result RESULT.json
```

Follow `/critty` to launch either command and collect its real output.  Keep the invocation, working directory, session ID, and result path.  Read all returned comments as well as the mandatory form result, even with custom finish templates.  Keep the files unchanged while the review is open.

After feedback, revise the necessary content and pass the complete ordered attachment list with `--session <id>` and `--result RESULT.json` for the next round.  A supplied list replaces the active set; omitting it reuses saved snapshots rather than rereading the draft files.  Changed targets or attachment contents, order, or membership reset required confirmations and consents.  Ordinary explicit-ID answers survive only unchanged definitions.  Preserve each approved result before reusing a result path.

## 5. Bind approval to execution

1. Parse the actual result JSON from the completed invocation.  Require `approved: true` and correlate its session, round, `review_revision`, `reviewed_target`, attachment snapshots, definitions, and answers with the request you launched.  A copied approval string, feedback draft, missing output, or stopped review is not authorization.
2. Locate each approval attachment by its source and snapshot, then use its document ID to read `consents[document_id][field_id]`.  Execute an individually gated action only when that value is explicitly `true`.  Overall approval, a required confirmation, or a choice alone does not grant an individual consent.  Read human choices from the matching attachment's answers, validate them against its definitions, and apply only their stated mapping.  IDs and values are data, never shell commands.
3. Verify the current operation still matches the approved snapshots and destination, including referenced payloads, commit/tag identities, and external prerequisites.  Extract publication inputs from the preserved approved snapshot or byte-identical draft using a YAML parser.  Pass exact bodies through structured tool arguments or a body-file option; never reconstruct approved prose or interpolate metadata as shell code.
4. Execute the authorized operations without another confirmation.  A declined operation stays unexecuted; skip its dependent operations and report what remains.  Record resulting IDs, URLs, SHAs, and completion state separately from approved files.  Approval does not waive independent permission or safety gates; present the correlated evidence to those gates.
5. Verify outcomes before retrying or cleaning up.  If an external result is uncertain, inspect the destination first to prevent duplicate publication.  After partial success, retry only the remaining unchanged, authorized operations.  Changed inputs or effects need renewed approval; an unresolved execution gate remains a blocker, not permission to bypass it.

Preserve approved drafts, snapshots, and results through waits, failures, and interruptions until all authorized operations are verified or explicitly abandoned.  Preserve the self-contained result before following Critty's printed session-cleanup command.  Apply the caller's retention policy afterward; remove only completed task-owned scratch files and preserve unrelated reviews and audit records.
