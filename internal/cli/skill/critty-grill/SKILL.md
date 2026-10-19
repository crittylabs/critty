---
name: critty-grill
description: Stress-test a plan, decision, or idea through questions in Critty, optionally recording domain vocabulary and ADRs compatible with grill-with-docs. Use when the user asks to grill or interview them through Critty.
---

# Grill with Critty

Turn an idea or draft into a record of decisions the user can inspect and correct.  Use a question attachment for choices, comments for reasoning, and successive revisions to show what the answers change.  This skill owns the interview; `/critty` owns terminal launch, waiting, feedback collection, and recovery.  Use `/critty-cli` for attachment and result contracts.

Inspired by Matt Pocock's [grilling](https://github.com/mattpocock/skills/blob/main/skills/productivity/grilling/SKILL.md) (`grill-me`) and [grill-with-docs](https://github.com/mattpocock/skills/blob/main/skills/engineering/grill-with-docs/SKILL.md) skills.  Their installation is optional: the interview procedure is defined here, and the documentation contract is bundled in [compatible artifacts](references/artifacts.md).  Critty supplies the interaction; the resulting vocabulary and decisions remain usable by those document workflows.

## Agree on the output and destination

Treat artifact types and their destinations as interview decisions.  Inspect the project, existing documents, and the caller's storage conventions before preparing the first question round.  Honor explicit choices without asking again; present the remaining choices in the question attachment with concrete recommendations and reasons.

Offer the relevant outputs separately, allowing combinations:

- A decision summary or design note that stands on its own, with decisions, rationale, scope, constraints, and deferrals.
- A glossary for settled domain terms.
- ADRs for qualifying architecture decisions, with their alternatives and rationale.
- Conversation-only output when the user wants no standalone files.  Record this choice explicitly; unchecked artifact items do not select it.

Use independent checkboxes for requested files so the user can select a summary, glossary, ADRs, or any combination.  Attach `type=file` and a concrete `default="..."` to each summary or glossary checkbox, and `type=directory` to the ADR checkbox.  Prefer continuing an existing record.  Offer suitable project paths or the user's customary document location, explaining the recommendation; adapt example paths to the actual project.  When only a destination remains, use a standalone typed field.  Leave new output paths unrestricted; use `existing` only when a destination must already exist.

Read selected checkbox inputs from `text`, or standalone path fields from `value`, and require a usable path for every selected file output before final confirmation.  Unselected drafts are not destinations.  If the discussion reveals useful terminology or architecture decisions, offer the relevant artifact rather than silently adding it to the output.  Keep undecided artifact types and destinations open across rounds.

For selected glossary or ADR outputs, follow [compatible artifacts](references/artifacts.md), applying only the selected parts.  A request to work like `grill-with-docs` selects both types; confirm their concrete destinations and honor any explicit narrower scope.  Merely reading an existing document does not select an output type.  Explain when the discussion produces no content qualifying for a selected artifact.


Agree on the languages of question attachments, confirmations, and artifacts alongside the output choices.  Recommend languages from the chat, the project's conventions, and any explicit user preference.  When several choices fit, present language choices in the question attachment; allow different languages for discussion and project documents, or for different artifacts.  Prefer discussing an artifact in its intended language when that suits the user, so translation is not deferred until delivery.  Honor an explicit choice without asking again.  Switch subsequent rounds and pending artifacts when the user changes that choice or the project context calls for an agreed adjustment.  Preserve settled decisions and established domain terms when translating.

Question attachments, JSON exports, and Critty history are interview evidence.  Offer a history export separately when requested; it is not a substitute for the selected artifacts.  For conversation-only output, preserve the decisions, rationale, constraints, and deferrals in the conversation.

## Identify what needs a decision

Read the subject and any supplied documents.  Inspect relevant code and facts before asking the user; ask them for decisions and information unavailable from those sources.  For a document-based interview, cite the section supporting each question and distinguish the document's claims from verified facts.  Keep source documents unchanged during the interview unless editing them is requested.

Look for assumptions that change the outcome: who uses the result, what success means, costly tradeoffs, and how failures will be handled.  Keep a compact decision record with the answer, rationale, source, and any remaining uncertainty.  Distinguish a user's decision from a recommendation awaiting their response.

Choose questions the user can answer from the evidence available now.  Combine independent decisions in one attachment so they can be answered together.  Explain when a question must wait for research or an earlier choice; return to it after that prerequisite is resolved.  Let a surprising answer change the next set of questions rather than following a fixed questionnaire.

## Prepare a round

Run `critty tmp create` once for the interview and read the JSON `directory` field.  Keep the question attachment and per-round outputs in that directory under `.critty/tmp/`.  The examples below use `<directory>` for that returned absolute path; quote each substituted path.  Reuse it across rounds and interruptions.  This scratch directory needs no `tmp prepare`, `snapshot`, or `apply` step.

Write `QUESTIONS.md` there with a short topic summary, settled decisions relevant to this round, and numbered questions with stable descriptive IDs.  For each question, state the decision, its consequences, and a recommended answer with a reason.

Use unchecked choice items for mutually exclusive answers.  Tag the one recommended choice of each question with `tags=recommended`, and put an item with `select-all=recommended` before the questions so the user can accept every recommendation at once and then change individual answers.  Add `text` to an "Other" choice when the listed choices are not exhaustive, and offer a `value=defer text` choice so the user can postpone a decision with a reason.  Use a standalone `text` item for open-ended questions, and a `rank` field when the decision is an order of priorities.  Keep comments for challenging a premise or discussing a question.

Unchecked items and silence mean unanswered, not rejection or agreement.  A deferral is an answer that keeps the decision open.  Use ordinary choices rather than `consent`: these are design answers, not permission to execute operations.  Leave questions optional so the user can submit partial answers.

```markdown
# Retry policy — round 1

Choose answers, and comment on any question whose premise looks wrong.
Confirming answers advances the interview; it does not authorize implementation.

- [ ] Accept all recommendations <!-- form: id=accept-recommended select-all=recommended -->

## Q1. Should failed uploads retry automatically?

Recommendation: retry transient failures up to three times, to recover from
brief outages without leaving requests running indefinitely.

- [ ] Retry transient failures up to three times (recommended). <!-- form: id=upload-retry value=bounded tags=recommended -->
- [ ] Let the caller retry manually. <!-- form: id=upload-retry value=manual -->
- [ ] Use another policy. <!-- form: id=upload-retry value=other text -->
- [ ] Decide later. <!-- form: id=upload-retry value=defer text -->

## Q2. What data must never leave the device?

Recommendation: keep credentials and raw customer records local.

- [ ] Describe the boundary. <!-- form: id=local-data text -->

## Q3. How should the final understanding be delivered?

Recommendation: save documents so the decisions can be maintained with the project.

- [ ] Save documents (recommended). <!-- form: id=delivery value=documents tags=recommended -->
- [ ] Keep the result in the conversation only. <!-- form: id=delivery value=conversation -->

## Q4. If saving documents, which artifacts and destinations?

Choose one or more and edit their paths.  Recommendation: start with a decision
summary; add a glossary or ADRs if the discussion needs them.

- [ ] Decision summary (recommended). <!-- form: id=output-summary checkbox type=file default="docs/retry-policy.md" tags=recommended -->
- [ ] Domain glossary. <!-- form: id=output-glossary checkbox type=file default="GLOSSARY.md" -->
- [ ] Architecture decisions. <!-- form: id=output-adrs checkbox type=directory default="docs/adr/" -->
```

Ask only unresolved output questions; omit file choices for an explicit conversation-only request.  If conversation-only is selected, disregard file selections and their drafts.  If documents are selected but no artifact or usable destination is chosen, keep the output decision open.

Keep unchanged questions' IDs and choice definitions stable.  Use a new ID when the meaning changes, even if the wording is similar.  Leave every item unchecked; recommendations are tags, and only the user's own selections are answers.

## Collect and apply answers

Follow `/critty` for task registration, launch, and completion handling.  Start an attachment-only review with the question document; optionally attach necessary source excerpts as context.  For example, after the launch preparation required by that skill:

```sh
critty review --null --answers --attachment '<directory>/QUESTIONS.md' --result '<directory>/result-1.json' --summary '<directory>/summary-1.json'
```

`--answers` labels the Comment verdict as **Confirm answers** for question rounds; the verdict remains `comment`.  Include the task ID and terminal request when required by `/critty`.  Omit `--result` when there are no form fields.  Preserve the original completion output and use fresh result and summary paths for every invocation.

After completion, match the session, round, attachment snapshots, and definitions before reading answers.  Read all comments and replies, including resolved threads and referenced images.  For forms, use the result's `attachments[].answers` values, `text` notes, and human provenance; the summary alone has no form answers.  An initial value without human provenance is not an answer.  `via` marks answers set by accepting all recommendations.  Text answers are Markdown; read images they reference through the result's `image_base`.

Confirm answers, Comment, and Reject submissions can supply interview answers without approving the design.  Treat those answers as provisional and carry them into the next round for confirmation.  If a choice and comment conflict, keep that decision open and ask for clarification; do not silently prefer one.  An unexplained alternative remains open.  An overall Approve does not answer blank questions.

Revise the decision record from the actual responses.  Explain what each changed answer affects, including earlier conclusions that now need another look.  Reply to substantive feedback using `/critty`'s comment workflow.

For selected glossary and ADR outputs, record newly settled terms and accepted ADRs before opening the next round, using the artifact reference and the caller's review policy.  Keep tentative answers in the question attachment until confirmed.  Carry a concise account of answers and rationale into the conversation as well, including exact constraints and explicit deferrals, so a following skill can continue without interpreting Critty JSON.

Replace the question attachment with the next round, keeping settled decisions visible in a compact record.  Resume the same session with the complete ordered attachment list and fresh output paths:

```sh
critty --session <id> --attachment '<directory>/QUESTIONS.md' --result '<directory>/result-2.json' --summary '<directory>/summary-2.json'
```

Follow `/critty`'s launch and wait procedure for every round.  Its approval condition ends review of one round; remaining unanswered questions or consequences needing a decision require another.  Preserve those questions across interruptions.  If research is the only remaining prerequisite, complete that research before opening another round.

## Confirm shared understanding

When no actionable questions remain, present the resulting decisions, rationale, scope, explicit deferrals, remaining risks, and each selected artifact type with its destination path in the same session.  Specify exact paths for authored files, or the task and session IDs and any chosen export folder for history.  Include the artifact contents or pending changes in this confirmation.  For conversation-only output, state that choice explicitly.  Resume with `--no-answers` so the user approves with the ordinary Approve.  Ask the user to confirm that this accurately captures the shared understanding.  Unanswered questions remain open unless the user explicitly defers or removes them; a stalled interview is not completion.

After confirmation, save or apply every agreed artifact under the caller's review and permission rules.  Read back the saved files, verify them against the confirmed content, and return clickable links to their actual paths.  For selected history output, use `/critty-history` to verify task membership and retention, and return the task and session IDs and any exported document links.  A save failure leaves the deliverable incomplete; preserve the drafts and report the blocker before cleanup.  Finish only after confirmation and verified delivery, with no unacknowledged open decisions.

Verify selected glossary and ADR outputs against [compatible artifacts](references/artifacts.md); explain when nothing qualified for a file.  In every document written from the interview, copy images from answers and comments into that document's own asset directory and link them relative to the document before the session is cleaned up.  Return the confirmed decisions, rationale, exact constraints, and deferrals to the conversation for a later `to-spec` or equivalent synthesis step.  Do not run or publish that step without the user's request.

Interview approval confirms understanding only; implementation, publication, and other operations follow the caller's separate authorization rules.  Use `/critty-history` when the interview task is complete, retaining evidence according to its policy.  Remove only the returned scratch directory once its files are no longer needed and the confirmed understanding is available outside it; keep `.critty/tmp/.gitignore` and other tasks' directories.
