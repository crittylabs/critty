---
name: critty-ai-cowork
description: Work with another AI through Critty to refine requested work, including human review comments asking for collaboration with a named AI such as Claude or Codex, then present it for final human review. Use critty-ai-review for a second opinion or consultation with human input between rounds.
---

# Cowork with Another AI

Experimental: interfaces and supported launch configurations may change.

Read [critty-ai-review](../critty-ai-review/SKILL.md) and follow its requester workflow, authorization scope, request identity, snapshot, posting, and completion contracts.  This skill changes only how the requester acts between AI review rounds.  Keep its reviewer role unchanged.

A request such as `/critty-ai-cowork` with `Astra` selects a collaborator for the current task and authorizes in-scope fixes and follow-up between agents.  Follow the shared skill's `with` argument handling.  Establish the target outcome and acceptance checks before dispatch, using existing task context.  If the request only asks for an opinion, use `critty-ai-review` instead.  Preserve the user's selected collaborator and the caller's execution permissions.

## Replace the per-round checkpoint

After collecting and verifying each AI result:

1. Assess the findings against the requirements and evidence.  Exchange focused questions or counterexamples with the reviewer when needed; agreement alone is not evidence of correctness.
2. Make authorized fixes and run the affected checks as the requester.  The other AI reads, posts findings, replies to its own threads, and resolves its own verified findings through the restricted Critty interface; it does not edit files or execute checks.
3. Capture the revised target and request review of the affected scope, explicitly carrying forward unresolved findings and permitting the reviewer to resolve its own findings after verification.  Reuse valid results for unchanged content.  Keep each assignment tied to its own version and completion report.

For a one-shot reviewer that has exited, continue an exchange by posting the requester's question or evidence as a reply, then launching a new assignment with a new request ID and references to that thread and the prior result.  A live transport may continue on its existing handle.  An exited process cannot receive another question merely by waiting on it.

Continue without asking the human to approve each AI response.  Stop iterating when the acceptance checks pass and actionable findings are addressed, with a completed AI assessment of the final affected scope.  Preserve substantiated disagreements and limitations for final human review instead of editing merely to obtain AI agreement.

Ask for a human decision when progress requires one: a material scope change, repeated disagreement without new evidence, or a recurring failure that prevents verification.  Honor explicit round, time, and cost limits; reaching one ends the loop with a report of remaining work, not a claim of completion.

## Final human review

Present the completed changes, checks, AI findings and their disposition, and remaining limitations through the caller's human review workflow.  Human feedback can start another in-scope implementation and AI review round, followed by renewed human review of changed content.

This skill does not activate autonomous approval or waive required human gates for commits, release, or other actions.  AI completion remains separate from human acceptance.
