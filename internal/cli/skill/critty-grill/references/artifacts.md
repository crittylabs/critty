# Compatible interview artifacts

Use this reference for documentation mode.  Its file layout and Markdown conventions follow the output contract of Matt Pocock's [domain-modeling](https://github.com/mattpocock/skills/tree/main/skills/engineering/domain-modeling), which `grill-with-docs` uses.  This is artifact compatibility, not a requirement to reproduce its interview wording or install its skills.

## Locate existing documents

Read the repository's `GLOSSARY-MAP.md` first when present.  Follow its links to the relevant context's glossary and `docs/adr/`; use root `docs/adr/` for decisions spanning contexts.  Ask when the context cannot be determined.  Otherwise use root `GLOSSARY.md` and `docs/adr/`.  Continue existing files and numbering rather than creating a second set under the interview scratch directory.

Create a file only when there is settled content for it.  An interview with no new terminology or qualifying decision may produce neither a glossary nor an ADR.  Explicit user destinations and repository conventions take precedence; report any departure from the reference layout instead of claiming exact compatibility.

## Vocabulary

When the user settles a term, update the appropriate glossary during that round.  Check the proposed meaning against existing definitions and code; bring contradictions back to the user.  Unconfirmed terminology belongs in the question attachment.

Use a context title, a brief context description, and a `Language` section.  Each entry has a bold canonical term followed by a colon, a definition of one or two sentences, and an `_Avoid_` list for competing names when applicable.  For example:

```markdown
# Equipment Lending

The vocabulary used to lend shared equipment to members.

## Language

**Loan**:
An agreement assigning one equipment item to a member for a specified period.
_Avoid_: Rental, checkout
```

Keep entries about domain concepts.  Implementation choices, general programming terms, requirements, and interview notes belong elsewhere.  Group related entries under subheadings when useful.  In a multi-context repository, preserve the map's `Contexts` links and `Relationships` description; keep each term in its owning context.

## Architecture decisions

Offer an ADR only when changing the decision later would be costly, its rationale would otherwise puzzle a future reader, and it resolves a genuine choice between alternatives.  All three conditions must hold.  Record the accepted decision when it is settled, rather than collecting every answer into ADRs at the end.

Use `docs/adr/NNNN-short-slug.md`, taking the next number after the highest existing entry in that directory.  Check again before writing so concurrent work is not overwritten.  The content starts with a descriptive title and a short paragraph explaining the situation, selected approach, and reason.  Additional sections are optional:

- `Considered Options` for alternatives worth retaining.
- `Consequences` for effects a reader might otherwise miss.
- YAML `status` when tracking a lifecycle: `proposed`, `accepted`, `deprecated`, or `superseded by ADR-NNNN`.

Continue the repository's existing ADR style when present.  A record of a single decision need not become a specification or a questionnaire transcript.

## Review and continuation

Documentation mode authorizes preparing these requested artifacts; follow the caller's policy for review and application.  Under a review-before-write policy, prepare the glossary or ADR changes as soon as the decision settles and review them before applying; keep the pending state visible across rounds.  For files outside source control, use the temporary-tree workflow referenced by the main Critty skill.  Final interview confirmation must include any remaining artifact changes.

Keep ordinary decisions that do not qualify for an ADR in the conversation.  Preserve numeric limits, exclusions, rationale, and unresolved or deferred matters accurately.  A later `to-spec` step uses that conversation together with the glossary and ADRs; it must not need Critty-specific form IDs or JSON to recover the agreed design.  A glossary alone is not a complete design handoff.

Before finishing, check the selected context, term definitions, ADR numbering and rationale, and consistency with the confirmed answers.  Report the actual files changed and any intentional omission.  Do not create a spec, tickets, or an issue-tracker entry merely to make the interview appear complete.
