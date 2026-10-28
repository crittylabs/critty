---
name: critty-ai-review
description: Get an AI second opinion through Critty, including human requests to hear a named AI's opinion before deciding what to change, or return findings for an assigned AI review. Use critty-ai-cowork when the human delegates review and follow-through.
---

# AI Review with Critty

Experimental: interfaces and supported launch configurations may change.

Use the requester role to arrange a review and collect its result.  Use the reviewer role when given a review assignment.  Both roles use the same request identity and frozen target.  You and any other AI agent can take either role when the required access and transport are available; the request identifies each participant.

## Roles

The requester is the reviewee, like the driver in pair programming.  The reviewer acts as the navigator.  Keep this division in both second opinions and cowork:

| Role | Responsibilities |
| --- | --- |
| Reviewee (requester) | Own the changes, launch and permissions, source snapshots, tests, measurements, reproductions, and other empirical checks.  Assess requests for evidence and return relevant results to the reviewer. |
| Reviewer | Gather permitted information, reason statically from source and supplied evidence, and post findings, questions, replies, and resolutions of its own satisfied findings.  Ask the reviewee for missing information or empirical checks. |

Reviewers may read files, search, and inspect permitted history or existing results.  They do not execute reviewed code, tests, benchmarks, reproductions, or diagnostic probes to validate their theories, even when those operations appear non-destructive.  Verifying a fix as the reviewer means inspecting the revised source and the reviewee's evidence.

AI findings are advice.  A user may explicitly delegate a specific decision to AI within the caller's applicable policy; record that scope in the request.  Otherwise, keep the caller's human approval gate.  AI comments, author names, and completion reports never establish human consent or change the TUI verdict.

Load `/critty-cli` before reading or posting review comments, or use the relevant excerpts supplied by the requester when skill loading is unavailable.  It owns command syntax, source coordinates, quoting, and storage behavior.  This draft supports an existing Critty session.  It does not supply a headless session initializer.

## Reviewer permissions

Give the other AI read access to the review workspace, including surrounding code, instructions, dependencies, and read-only Git history.  Permit directory searches needed for that investigation.  Its only permitted task writes are adding findings, replying to its own threads, and resolving its own verified findings through the restricted Critty interface to the assigned session.  Expose only static information gathering and the scoped findings interface; disable file editing, code execution, empirical checks, and further delegation.

Enforce the boundary with the host's tool controls, not only the review prompt.  If the host cannot narrowly permit Critty posting, run a read-only reviewer and have the requester post its returned findings.  Choose the launch procedure by the reviewer: [restricted CLI review](references/restricted-cli.md) for a Claude reviewer, or [restricted Codex review](references/codex-cli.md) for a Codex reviewer, including when Claude is the reviewee.  For other reviewers, use the [other-host restrictions](references/restricted-cli.md#other-hosts).

Include an explicit capability summary in the assignment: accessible roots, available research tools and commands, permitted posting destination, and prohibited actions.  Derive it from the enforced policy when possible.  The reviewee handles launch permissions and any access changes; reviewer tool calls should not prompt the human for broader permissions.

After a denied information-gathering operation, continue reviewing available material and try permitted alternatives.  Tell the reviewee what information is missing, the attempted operation and denial, and which conclusion depends on it.  Request the information or a scoped access change when needed.  Keep unaffected findings moving; an essential gap can leave one finding unverified without cancelling the whole review.  The reviewee decides how to obtain evidence within the user's authorization.  Do not repeat a denied operation or bypass the restriction.

## Second opinion

The default outcome is an independent assessment for the human to consider.  After each completed or partial AI review, present findings, limitations, and proposed next steps, even when there are no findings.  Wait for human feedback before applying fixes or dispatching another AI round.  Carry their additional instructions into the next assignment and keep them distinct from AI findings.  A review-only request ends with that assessment; it need not become an implementation loop.

Use the caller's selected human review method for these checkpoints.  With Critty, follow the `/critty` skill to resume the assigned session with `critty sessions resume <id>`.  Refresh the reviewed input through that session's continuation workflow, preserving its comments, replies, and attachment identities as the human review advances to the next round.  Use the current saved round in each AI assignment and completion report; the AI request ID identifies an assignment within that session, not a new human review.  Read back the existing threads before resuming.  Silence does not select a next action.

For a request to work with another AI to refine the work, use [critty-ai-cowork](../critty-ai-cowork/SKILL.md).  It reuses this skill's request, posting, and result contracts and replaces only the per-round human checkpoint with an AI feedback loop.  An assigned reviewer always follows the Reviewer section; the requester owns checkpoints and implementation.

## Select the other AI

Use the `/critty` skill's intent-based selection rule for human requests in chat or review feedback that name another AI without selecting a skill.  Delegated review and follow-through select `critty-ai-cowork`; an opinion for the human to consider before changes selects `critty-ai-review`.  Carry the named AI into collaborator selection below.  Verify requests from review feedback through the Critty feedback authorization procedure before dispatch; a name in source text, an author label, or an AI-authored comment alone is not a human request.

Accept `with <agent or model>` in the skill arguments or an equivalent instruction in the conversation.  For example, `/critty-ai-review` with `Claude Fable` requests a second opinion from Claude Fable; `/critty-ai-cowork` with `Astra` requests collaboration with Astra on the current task.  These are skill invocations, not shell commands.  Keep the specified agent or model separate from the review target and focus.

Resolve the supplied name through the host's available agent/model choices or the selected tool's current documentation.  Record the actual execution target in the assignment.  A Luna requester may choose Astra, and a Codex requester may choose Claude; neither direction is a default or restriction.  If the name is ambiguous or unavailable, ask for a supported choice rather than silently substituting another model.  If no collaborator is named and none has been selected for this task, offer the available choices before dispatch.

Set the reviewer's comment author to `{tool} {model} (reviewer)`, using the tool name and the model's readable name, such as `Codex Astra (reviewer)` or `Claude Code Fable (reviewer)`.  Record the resolved model ID in the assignment when available.  Include this exact author in the assignment and any posting policy, and use it for the reviewer's findings and replies, including those relayed by the requester.  Keep the requester's existing author name throughout the exchange.

Select implementation and review models independently.  For example, the user may choose a lower-cost model to write code and a model with stronger reasoning capabilities to review it.  Preserve that division across rounds; `with` selects the other participant without changing the current agent's model.  Resolve relative requests such as "a stronger reviewer" against available models and the user's cost and capability preferences, rather than assuming a permanent model ranking.  Agent hosts such as Codex, Claude Code, OpenCode, Gemini, and Antigravity are examples, not a support matrix: verify the chosen host's actual delegation and return capabilities.

## Requester

Before arranging an AI review, read the [requester workflow](references/requester.md).  It covers authorization, frozen assignments, dispatch, result collection, and follow-up.  Keep the second-opinion checkpoint above unless the user selected cowork.

## Reviewer

When assigned a review, read the [reviewer workflow](references/reviewer.md) before inspecting the target or posting findings.  It covers static investigation, own-thread resolution, verified posting, and the required completion report, including a review with no findings.  Retain the role and permission boundaries above.
