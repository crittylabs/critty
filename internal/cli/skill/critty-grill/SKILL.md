---
name: critty-grill
description: Stress-test a plan, decision, or idea through questions in Critty, optionally recording domain vocabulary and ADRs compatible with grill-with-docs. Use when the user asks to grill or interview them through Critty.
---

# Grill with Critty

Turn an idea or draft into a record of decisions the user can inspect and correct.  Use a question attachment for choices, comments for reasoning, and successive revisions to show what the answers change.  This skill owns the interview; `/critty` owns terminal launch, waiting, feedback collection, and recovery.  Use `/critty-cli` for attachment and result contracts.

Inspired by Matt Pocock's [grilling](https://github.com/mattpocock/skills/blob/main/skills/productivity/grilling/SKILL.md) and [grill-with-docs](https://github.com/mattpocock/skills/blob/main/skills/engineering/grill-with-docs/SKILL.md) skills.  Critty supplies the interaction; the resulting vocabulary and decisions remain usable by their document workflows.

## Choose the output

For an interview alone, preserve the shared understanding in the conversation, as `grilling` / `grill-me` do.  For an interview with domain documentation, follow [compatible artifacts](references/artifacts.md): update `GLOSSARY.md` as terms are settled and offer qualifying ADRs under `docs/adr/`.  A request to work like `grill-with-docs` selects this documentation mode.  Merely reading an existing document does not select it.  Honor an explicit output request; clarify only when the intended mode is unclear.

The artifact reference is bundled, so the external skills need not be installed.  Question attachments and JSON exports are interview evidence, not replacements for the glossary, ADRs, or conversation needed by a later specification step.

## Identify what needs a decision

Read the subject and any supplied documents.  Inspect relevant code and facts before asking the user; ask them for decisions and information unavailable from those sources.  For a document-based interview, cite the section supporting each question and distinguish the document's claims from verified facts.  Keep source documents unchanged during the interview unless editing them is requested.

Look for assumptions that change the outcome: who uses the result, what success means, costly tradeoffs, and how failures will be handled.  Keep a compact decision record with the answer, rationale, source, and any remaining uncertainty.  Distinguish a user's decision from a recommendation awaiting their response.

Choose questions the user can answer from the evidence available now.  Combine independent decisions in one attachment so they can be answered together.  Explain when a question must wait for research or an earlier choice; return to it after that prerequisite is resolved.  Let a surprising answer change the next set of questions rather than following a fixed questionnaire.

## Prepare a round

Run `critty tmp create` once for the interview and read the JSON `directory` field.  Keep the question attachment and per-round outputs in that directory under `.critty/tmp/`.  The examples below use `<directory>` for that returned absolute path; quote each substituted path.  Reuse it across rounds and interruptions.  This scratch directory needs no `tmp prepare`, `snapshot`, or `apply` step.

Write `QUESTIONS.md` there with a short topic summary, settled decisions relevant to this round, and numbered questions with stable descriptive IDs.  For each question, state the decision, its consequences, and a recommended answer with a reason.

Use unchecked choice items for mutually exclusive answers, marking the recommendation in prose.  Include an alternative answer via comment when the listed choices are not exhaustive.  Use comments alone for open-ended questions; ask the user to comment on that question.  Unchecked items and silence mean unanswered, not rejection or agreement.  Use ordinary choices rather than `consent`: these are design answers, not permission to execute operations.  Leave questions optional so the user can submit partial answers or challenge the premise.

```markdown
# Retry policy — round 1

Choose answers and add explanations as comments on each question.
Submitting answers advances the interview; it does not authorize implementation.

## Q1. Should failed uploads retry automatically?

Recommendation: retry transient failures up to three times, to recover from
brief outages without leaving requests running indefinitely.

- [ ] Retry transient failures up to three times (recommended). <!-- form: id=upload-retry value=bounded -->
- [ ] Let the caller retry manually. <!-- form: id=upload-retry value=manual -->
- [ ] Use another policy; explain in a comment. <!-- form: id=upload-retry value=other -->

## Q2. What data must never leave the device?

Recommendation: keep credentials and raw customer records local.
Accept this recommendation or describe the boundary in a comment on Q2.
```

Keep unchanged questions' IDs and choice definitions stable.  Use a new ID when the meaning changes, even if the wording is similar.  Do not preselect a recommendation or manufacture human answers.  This workflow uses the existing choice and comment features; it does not require bulk-selection controls.

## Collect and apply answers

Follow `/critty` for task registration, launch, and completion handling.  Start an attachment-only review with the question document; optionally attach necessary source excerpts as context.  For example, after the launch preparation required by that skill:

```sh
critty --null --attachment '<directory>/QUESTIONS.md' --result '<directory>/result-1.json' --summary '<directory>/summary-1.json'
```

Include the task ID and terminal request when required by `/critty`.  Omit `--result` when there are no form fields.  Preserve the original completion output and use fresh result and summary paths for every invocation.

After completion, match the session, round, attachment snapshots, and definitions before reading answers.  Read all comments and replies, including resolved threads and referenced images.  For forms, use the result's `attachments[].answers` values and human provenance; the summary alone has no form answers.  An initial value without human provenance is not an answer.

Comment and Reject submissions can supply interview answers without approving the design.  Treat those answers as provisional and carry them into the next round for confirmation.  If a choice and comment conflict, keep that decision open and ask for clarification; do not silently prefer one.  An unexplained alternative remains open.  An overall Approve does not answer blank questions.

Revise the decision record from the actual responses.  Explain what each changed answer affects, including earlier conclusions that now need another look.  Reply to substantive feedback using `/critty`'s comment workflow.

In documentation mode, record newly settled terms and accepted ADRs before opening the next round, using the artifact reference and the caller's review policy.  Keep tentative answers in the question attachment until confirmed.  Carry a concise account of answers and rationale into the conversation as well, including exact constraints and explicit deferrals, so a following skill can continue without interpreting Critty JSON.

Replace the question attachment with the next round, keeping settled decisions visible in a compact record.  Resume the same session with the complete ordered attachment list and fresh output paths:

```sh
critty --session <id> --attachment '<directory>/QUESTIONS.md' --result '<directory>/result-2.json' --summary '<directory>/summary-2.json'
```

Follow `/critty`'s launch and wait procedure for every round.  Its approval condition ends review of one round; remaining unanswered questions or consequences needing a decision require another.  Preserve those questions across interruptions.  If research is the only remaining prerequisite, complete that research before opening another round.

## Confirm shared understanding

When no actionable questions remain, present the resulting decisions, rationale, scope, explicit deferrals, and remaining risks in the same session.  Ask the user to confirm that this accurately captures the shared understanding.  Unanswered questions remain open unless the user explicitly defers or removes them; a stalled interview is not completion.

Finish only after that confirmation, with no unacknowledged open decisions.  In documentation mode, verify the glossary and ADRs against [compatible artifacts](references/artifacts.md) and report their paths; explain when nothing qualified for a file.  Honor other requested document outputs within their authorized scope.  Return the confirmed decisions, rationale, exact constraints, and deferrals to the conversation for a later `to-spec` or equivalent synthesis step.  Do not run or publish that step without the user's request.

Interview approval confirms understanding only; implementation, publication, and other operations follow the caller's separate authorization rules.  Use `/critty-history` when the interview task is complete, retaining evidence according to its policy.  Remove only the returned scratch directory once its files are no longer needed and the confirmed understanding is available outside it; keep `.critty/tmp/.gitignore` and other tasks' directories.
