package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/crittylabs/critty/internal/config"
	"github.com/crittylabs/critty/internal/ipc"
	"github.com/crittylabs/critty/internal/review"
)

var planName string
var planSession string

var planCmd = &cobra.Command{
	Use:   "plan [--name <slug> | --session <id>] [file]",
	Short: "Create or continue a versioned plan review",
	Long: `Create or continue a plan review.  The plan content (from the file
argument or piped stdin) is saved as a new immutable version under the
plan's storage directory, and a review of the latest version opens,
blocking like ` + "`critty review`" + `.

Without --name, the slug is derived from the plan's first heading.
Each new invocation creates an independent session.  Use --session <id>
to save a new version and continue a previous review.`,
	Args:    cobra.MaximumNArgs(1),
	PreRunE: validateScopeFlags,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runPlan(args)
	},
}

func runPlan(args []string) error {
	if planSession != "" {
		return withSessionDirectory(planSession, args, runPlanInDirectory)
	}
	return runPlanInDirectory(args)
}

func runPlanInDirectory(args []string) error {
	inputs := &reviewMode{sessionKey: planSession}
	if err := prepareDocuments(inputs); err != nil {
		return err
	}
	cfg, err := config.LoadCurrent()
	if err != nil {
		return err
	}

	content, sourceFile, err := readPlanContent(args)
	if err != nil {
		return err
	}

	slug := review.Slugify(planName)
	if slug == "" && planSession == "" {
		slug = review.ResolveSlug(content)
		fmt.Fprintf(os.Stderr, "No --name provided, derived slug: %s\n", slug)
	}

	var sess *review.Session
	var mode *reviewMode
	if planSession != "" {
		sess, mode, err = loadReviewSession(planSession)
		if err != nil {
			return err
		}
		if !mode.plan() {
			return fmt.Errorf("session is not a plan review")
		}
		if planName != "" && slug != mode.planSlug {
			return fmt.Errorf("plan name differs from saved session")
		}
		if ipc.Alive(review.SocketPathFor(sess.Key)) {
			return fmt.Errorf("review is active; stop it before updating the plan")
		}
		slug = mode.planSlug
	} else {
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		sess, err = review.NewSession(cfg.Output, review.SessionEntry{CWD: cwd, Mode: "plan", PlanSlug: slug})
		if err != nil {
			return err
		}
		mode = &reviewMode{docPath: filepath.Join(sess.Dir, "current.md"), planSlug: slug, sessionKey: sess.Key}
	}
	if inputs.replaceDocuments {
		mode.documents, mode.replaceDocuments = inputs.documents, true
	}
	mode.documentsPrepared = true
	mode.planFile = sourceFile
	mode.planContent = content
	sess.Meta.Args = []string{"plan", "--name", slug, sourceFile}
	return runReviewFlow(cfg, sess, mode)
}

// readPlanContent reads the plan from the file argument or piped stdin.
func readPlanContent(args []string) (content []byte, sourceFile string, err error) {
	if len(args) == 1 {
		data, err := os.ReadFile(args[0])
		if err != nil {
			return nil, "", fmt.Errorf("reading plan: %w", err)
		}
		if len(data) == 0 {
			return nil, "", fmt.Errorf("plan file %s is empty", args[0])
		}
		return data, args[0], nil
	}

	if term.IsTerminal(int(os.Stdin.Fd())) {
		return nil, "", fmt.Errorf("usage: critty plan [--name <slug>] <file>  (or pipe the plan on stdin)")
	}
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		return nil, "", fmt.Errorf("reading stdin: %w", err)
	}
	if len(data) == 0 {
		return nil, "", fmt.Errorf("no plan content on stdin")
	}
	return data, "", nil
}

func init() {
	rootCmd.AddCommand(planCmd)
	addFocusFlag(planCmd)
	addWatchFlag(planCmd)
	addHeaderFlags(planCmd)
	addAttachmentFlags(planCmd)
	planCmd.Flags().StringVar(&planSession, "session", "", "continue a saved plan review")
	planCmd.Flags().StringVar(&planName, "name", "", "plan slug (derived from the first heading when omitted)")
}
