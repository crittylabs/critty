package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"

	"github.com/crittylabs/critty/internal/review"
	"github.com/crittylabs/critty/internal/workspace"
	"github.com/spf13/cobra"
)

func newTmpCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "tmp", Short: "Prepare and apply bounded edits outside source control", Long: "Prepare independent copies under .critty/tmp, review a fixed snapshot, then apply only its approved changes.  Targets are explicit root-relative file paths, including missing paths for additions.  Deletions, renames, and splits are supported.  Symlinks and special files are rejected; select their intended regular-file targets instead.  Commands emit JSON.  Original files are changed only by apply."}
	var source string
	prepare := &cobra.Command{Use: "prepare --root <directory> -- <file>...", Short: "Copy registered targets into an isolated review tree", Args: cobra.MinimumNArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		w, err := workspace.Prepare(cwd, source, args)
		if err != nil {
			return err
		}
		return json.NewEncoder(cmd.OutOrStdout()).Encode(w)
	}}
	prepare.Flags().StringVar(&source, "root", ".", "source root; all targets must stay beneath it")
	snapshot := &cobra.Command{Use: "snapshot <workspace-id>", Short: "Freeze edited copies and generate a diff and application request", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		w, err := workspace.Load(cwd, args[0])
		if err != nil {
			return err
		}
		id, err := w.Snapshot()
		if err != nil {
			return err
		}
		dir := filepath.Join(w.Dir, "snapshots", id)
		return json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]string{"workspace": w.ID, "snapshot": id, "diff": filepath.Join(dir, "review.diff"), "attachment": filepath.Join(dir, "APPLY.md"), "result": filepath.Join(dir, "result.json")})
	}}
	var resultPath, applyRoot string
	apply := &cobra.Command{Use: "apply <workspace-id> <snapshot-id> --root <directory> --result <file>", Short: "Apply exactly the consented snapshot, detecting concurrent edits", Long: "Validate the saved human consent and exact reviewed patch, then apply registered creations and updates before deletions.  The explicit root must match the prepared root.  Check all originals before writing and retain backups and a receipt for recovery.  Replacement is atomic per file, not across the batch; retry the same snapshot after a partial failure.  New directories use 0777 subject to umask.  Replacement does not preserve ownership, ACLs, extended attributes, or hard-link relationships.  Stop other writers during application.  This command does not reload applications or grant host filesystem access.", Args: cobra.ExactArgs(2), RunE: func(cmd *cobra.Command, args []string) error {
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		w, err := workspace.Load(cwd, args[0])
		if err != nil {
			return err
		}
		root, err := filepath.Abs(applyRoot)
		if err != nil {
			return err
		}
		root, err = filepath.EvalSymlinks(root)
		if err != nil {
			return err
		}
		if root != w.Root {
			return fmt.Errorf("application root differs from prepared root")
		}
		receipt, err := w.Apply(args[1], func(s workspace.Snapshot) error { return verifyWorkspaceConsent(resultPath, s) })
		if outputErr := json.NewEncoder(cmd.OutOrStdout()).Encode(receipt); outputErr != nil {
			return outputErr
		}
		return err
	}}
	apply.Flags().StringVar(&resultPath, "result", "", "completed Critty form result for this snapshot")
	apply.Flags().StringVar(&applyRoot, "root", "", "destination root recorded by prepare")
	if err := apply.MarkFlagRequired("root"); err != nil {
		panic(err)
	}
	if err := apply.MarkFlagRequired("result"); err != nil {
		panic(err)
	}
	cmd.AddCommand(prepare, snapshot, apply)
	return cmd
}

func verifyWorkspaceConsent(path string, s workspace.Snapshot) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var result review.FormResult
	if err := json.Unmarshal(data, &result); err != nil {
		return err
	}
	if !result.Approved || result.Verdict != review.VerdictApprove || result.Round < 1 || !review.ValidSessionKey(result.Session) {
		return fmt.Errorf("completed review approval is required")
	}
	entry, err := review.ReadSessionEntry(result.Session)
	if err != nil {
		return err
	}
	saved, err := os.ReadFile(filepath.Join(filepath.Dir(entry.ReviewPath), fmt.Sprintf("result-%d.json", result.Round)))
	if err != nil {
		return err
	}
	var original review.FormResult
	if err := json.Unmarshal(saved, &original); err != nil {
		return err
	}
	if !reflect.DeepEqual(original, result) {
		return fmt.Errorf("result differs from saved review")
	}
	if result.Input.Scope != "diff" || result.Input.Diff != s.Diff {
		return fmt.Errorf("reviewed patch differs from snapshot")
	}
	for _, d := range result.Documents {
		a := d.Answers["apply"]
		if d.Content == s.Request() && !d.Removed && result.Consents[d.ID]["apply"] && a.Human && a.Value == true && a.Revision == result.Revision && a.Round > 0 && a.Round <= result.Round {
			return nil
		}
	}
	return fmt.Errorf("consent to the exact application request is required")
}

func init() { rootCmd.AddCommand(newTmpCommand()) }
