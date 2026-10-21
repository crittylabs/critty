---
name: critty-ai-cowork
description: Work with another AI through Critty to refine requested work, including human requests that delegate review and follow-through to a named AI such as Claude or Codex, then present it for final human review. Use critty-ai-review for a second opinion or consultation with human input between rounds.
---

# Cowork with Another AI

Experimental: interfaces and supported launch configurations may change.

Read [critty-ai-review](../critty-ai-review/SKILL.md) and follow its requester workflow, authorization scope, request identity, snapshot, posting, and completion contracts.  This skill changes only how the requester acts between AI review rounds.  Keep its reviewer role unchanged.

A request such as `/critty-ai-cowork` with `Astra` selects a collaborator for the current task and authorizes in-scope fixes and follow-up between agents.  Follow the shared skill's `with` argument handling.  Establish the target outcome and acceptance checks before dispatch, using existing task context.  If the request only asks for an opinion, use `critty-ai-review` instead.  Preserve the user's selected collaborator and the caller's execution permissions.

## Replace the per-round checkpoint

After collecting and verifying each AI result:

1. Assess each finding against the requirements and evidence.  Reply in its thread with the accepted change or a reasoned disagreement.  Exchange focused questions or counterexamples until both sides agree on its disposition or identify a decision for the human; agreement alone is not evidence of correctness.
2. As the reviewee, make authorized fixes and gather any evidence needed to assess the findings, including tests, measurements, or reproductions.  For a comment or file entry carrying `textconv`, follow [Edit originals from textconv feedback](../critty-cli/SKILL.md#edit-originals-from-textconv-feedback) before changing the original file.  Answer the reviewer's information and access requests within the task's authorization and supply the relevant results.  The reviewer keeps the shared skill's static-review role throughout the loop; verifying a fix never transfers empirical checks to it.
3. Capture the revised target and request review of the affected scope, carrying forward unresolved findings and the requester's replies.  Require the reviewer to verify each disposition and resolve its own finding once satisfied, including when an explanation or counterexample settles it without a code change.  Follow the shared skill's resolution and readback rules, preserving other reviewers' concerns.  Reuse valid results for unchanged content.  Keep each assignment tied to its own version and completion report.

For a one-shot reviewer that has exited, continue an exchange by posting the requester's question or evidence as a reply, then launching a new assignment with a new request ID and references to that thread and the prior result.  A live transport may continue on its existing handle.  An exited process cannot receive another question merely by waiting on it.

Continue without asking the human to approve each AI response.  Complete the loop when acceptance checks pass, both sides agree on the findings' dispositions, and the reviewer has assessed the final affected scope.  An agreed escalation counts as a disposition and leaves the finding open for the human.  Verify that satisfied reviewer findings are saved as resolved, except where the shared resolution rules require keeping another reviewer's concern open.  Record the requester's agreement and any remaining concerns or limitations as a review-level comment under the requester's author, alongside the reviewer's completion report.  Use the shared skill's fallback for relayed resolution decisions; an unsaved decision leaves that step incomplete.

Preserve substantiated disagreements for a human decision instead of editing merely to obtain AI agreement.  Identify them as unresolved; a completed AI assessment alone does not establish agreement or resolution.

Ask for a human decision when progress requires one: a material scope change, repeated disagreement without new evidence, or a recurring failure that prevents verification.  Honor explicit round, time, and cost limits; reaching one ends the loop with a report of remaining work, not a claim of completion.

## Final human review

Present the completed changes, checks, AI findings and their disposition, and remaining limitations through the caller's human review workflow.  Human feedback can start another in-scope implementation and AI review round, followed by renewed human review of changed content.

This skill does not activate autonomous approval or waive required human gates for commits, release, or other actions.  AI completion remains separate from human acceptance.
