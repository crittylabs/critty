# Review edits outside source control

Use `critty tmp` when original files must stay unchanged until review, including new files and splits.  Follow the caller's review and filesystem-access policy.  Run all three subcommands from the same working directory; workspace IDs resolve beneath its `.critty/tmp/`.

1. From the working directory that will own `.critty/tmp/`, run `critty tmp prepare --root <source-directory> -- <file>...`.  Register exact root-relative destinations, including missing files for additions.  A split or rename combines updates, additions, and deletions.  Symlinks and special files are rejected; select intended regular-file targets explicitly.  Keep the returned workspace ID and working directory.
2. Edit only the returned `tree`.  Run relevant checks there.  Scratch files belong outside that tree.  If another destination is needed, prepare a new workspace with the expanded target list and carry over the proposal.
3. Run `critty tmp snapshot <workspace-id>`.  It freezes the candidate and returns absolute `diff`, `attachment`, and `result` paths.  Review those exact files through the normal `/critty` loop:

   ```sh
   critty --diff <diff> --attachment <attachment> --result <result> --task <task-id>
   ```

   Use a fresh terminal request when required by the host.  The application attachment names the source root, destinations, before/after states, and one consent to apply them.  Read all feedback and require both overall approval and that consent.  To revise, edit the tree, take a new snapshot, and resume the same session with the new diff, complete attachment list, and a fresh result path.
4. Follow [Execute the decision](../../critty-approval/SKILL.md#execute-the-decision), then run `critty tmp apply <workspace-id> <snapshot-id> --root <source-directory> --result <result>`.  The explicit root must match the prepared root.  It checks the saved human result, exact patch and application request, candidate contents, and every original before writing.  Stop other writers while applying: creations and updates precede deletions, with atomic replacement per file rather than across the batch.  Replacement preserves reviewed mode bits, but not ownership, ACLs, extended attributes, or hard-link relationships.
5. Verify the JSON receipt has `complete: true`.  On conflicts, preserve both versions and prepare a fresh baseline for review.  After partial failure, retain the workspace and retry the same snapshot and result; the receipt records completed and pending replacements.  Baseline copies remain under `originals/` for recovery.  Do not clean up an incomplete application.

Follow `/critty-history` when the whole task finishes.  Remove only the completed workspace directory under `.critty/tmp/` according to the caller's retention policy.  Keep `.critty/tmp/.gitignore`; it hides this scratch directory without changing global or repository exclude settings.  Use `critty tmp --help` and its subcommand help for the CLI contract.

## Host permissions

Preparation reads the registered sources; application writes the recorded root, which may be outside the working directory or affect live configuration.  Use existing task authorization and the host's normal access controls.  Review consent does not grant filesystem access, and a successful command prompt does not replace review consent.

For Claude Code, command rules, skill `allowed-tools`, permission mode, and sandbox settings jointly determine prompts.  A matching allow rule may also approve an unsandboxed retry.  Keep the application root within authorized sandbox write roots when using sandboxing; do not widen access merely to avoid a prompt.  File-tool rules alone do not bound reads and writes made through Bash.  See the official [permissions](https://code.claude.com/docs/en/permissions) and [sandboxing](https://code.claude.com/docs/en/sandboxing) references when configuring a host.
