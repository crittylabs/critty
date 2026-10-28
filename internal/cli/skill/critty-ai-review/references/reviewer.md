# Reviewer workflow

## Reviewer

### Inspect the frozen target

Verify the request identity, access to the named snapshots, and the Critty destination before posting.  Read the requirements, applicable instructions, and existing threads.  Review the supplied version; staged, committed, and attached content may differ from files on disk.  If the input changes or cannot be verified, report the mismatch and retain findings against the original snapshot without assigning them to new line numbers.

Inspect surrounding code, dependencies, and history needed to assess the requested scope within the authorized read roots.  Ask the reviewee for context outside those roots, reporting denied reads without abandoning permitted investigation.  If a hypothesis needs empirical evidence, describe the suspected condition, proposed check, and result that would confirm or refute it; the reviewee decides whether to run it and supplies the evidence.  Distinguish static reasoning, reviewee-reported results, and unverified hypotheses in findings.  Keep all source and supporting files unchanged.  For each actionable finding, explain the trigger, consequence, evidence, and source location; distinguish an unresolved question from a demonstrated defect.  Consolidate duplicate findings and read existing replies before adding to a thread.

### Return findings and completion

Read replies and the revised target for each of your earlier findings.  Resolve your own finding when the response or fix is valid, or when the finding is no longer applicable.  Reply with the evidence or reason and set `--resolve`, or use the restricted `resolve_finding` tool.  Keep unresolved findings open when evidence is insufficient or the issue remains.  This skill authorizes resolution of your own findings under the CLI reference's user-authorization rule; it grants no authority over human or other reviewers' threads.

Before resolving, read the latest full thread and other threads about the same issue.  If another reviewer, including the human, appears to have an unresolved or unanswered concern there, or you cannot tell whether it was addressed, leave the thread open.  State that your own concern is satisfied and why, using a reply without `--resolve` or the restricted `reply_finding` tool.  Your satisfaction does not settle another reviewer's concern.  Unrelated open findings elsewhere in the review do not prevent resolving your own thread.

Read the thread back to verify that the reply and intended resolution state were saved.  If the transport cannot post the decision, return its ID, path, and reason to the requester for verified relay under your reviewer author; include `resolve: true` only when closing is appropriate under the rule above.  The requester relays your decision rather than deciding on your behalf.  Report blocked or failed resolution separately from the assessment.

Post findings to the explicit session with the reviewer author specified in the assignment.  Include the request ID in comment bodies so concurrent reviews can be distinguished.  Use source coordinates from the reviewed snapshot and the supplied path mapping.  For deleted lines or uncertain anchors, use a file-level finding with quoted context instead of inventing a new-side line number.

Before returning, post an overall (review-level) completion comment under the same reviewer author, including when no actionable findings were found.  Write a short assessment for the human: lead with the conclusion, then summarize coverage and material verification limits.  Include the request ID and a compact reference to the frozen target, session, and round.  Keep full hashes, tool configuration, and execution bookkeeping in the final response and saved records rather than the comment.  Distinguish checks you performed from requester-supplied results.  With the restricted Codex MCP server, call `post_finding` with the report body and omit `location`; with the CLI, omit the file path.  Read the comment back and include its ID in the final response.  If posting is unavailable, return the report as a bulk JSON entry with `scope: "review"` for the requester to relay, and state that it is not yet saved.

Use the allowed posting interface with literal arguments.  If quoting or bulk input would require creating a file or executing a shell pipeline, return the payload to the requester to prepare and post.  After posting, read `critty comments --session <id> --all --json` and match the saved author, bodies, paths, and ranges; retain their IDs.  After an uncertain write, inspect saved comments before retrying so a partial success does not create duplicates.  Report a persistence failure separately from the review findings.

Return an explicit final response even when there are no findings.  Include:

- The request ID, reviewed target identity, session, and round.
- Whether the requested review is complete, partial, or blocked, with actual coverage and checks performed.
- Findings and their saved comment IDs, resolved own findings and their reasons, and findings left open; or an explicit statement that no actionable findings were found in the reviewed scope.
- The saved overall completion comment ID, or its unsaved review-level payload for relay.
- Unchecked areas, failed checks, uncertainties, and whether all intended comments were confirmed saved.  If storage is inaccessible, include the bulk JSON payload for the requester to post.

These are contents of the agent's final response, not a new Critty JSON schema.  A completed review may contain findings.  Keep completion separate from a clean assessment and from human approval.  Critty's `--summary` describes an interactive human review round; do not use it as an AI completion signal or edit its output to manufacture one.

Keep human and other reviewers' comments and resolution state, forms, and verdicts intact.  The reviewer role authorizes inspection and the assigned findings channel; it does not authorize source edits, session cleanup, or additional reviewers.
