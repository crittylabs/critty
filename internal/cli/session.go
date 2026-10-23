package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/crittylabs/critty/internal/config"
	"github.com/crittylabs/critty/internal/git"
	"github.com/crittylabs/critty/internal/ipc"
	"github.com/crittylabs/critty/internal/review"
)

func loadReviewSession(key string) (*review.Session, *reviewMode, error) {
	if !review.ValidSessionKey(key) {
		return nil, nil, fmt.Errorf("invalid session ID %q", key)
	}
	entry, err := review.ReadSessionEntry(key)
	if err != nil {
		return nil, nil, err
	}
	cwd, err := os.Getwd()
	if err != nil {
		return nil, nil, err
	}
	if cwd != entry.CWD {
		savedInfo, savedErr := os.Stat(entry.CWD)
		currentInfo, currentErr := os.Stat(".")
		if savedErr != nil || currentErr != nil || !os.SameFile(savedInfo, currentInfo) {
			return nil, nil, fmt.Errorf("session belongs to %s; resume from that directory", entry.CWD)
		}
	}
	sess, err := review.OpenSessionFromEntry(*entry)
	if err != nil {
		return nil, nil, err
	}
	mode := &reviewMode{sessionKey: key}
	documents, err := sess.LoadDocuments()
	if err != nil {
		return nil, nil, err
	}
	mode.documents = documents.Documents
	args := entry.Args
	kind := entry.Mode
	if kind == "" {
		switch {
		case len(args) == 1 && args[0] == "--diff":
			kind = "diff"
		case len(args) == 1 && strings.HasPrefix(args[0], "__plan:"):
			kind = "plan"
		case len(args) == 1 && !strings.HasPrefix(args[0], "--"):
			kind = "files"
		default:
			kind = "git"
		}
	}
	switch kind {
	case "diff":
		patch, err := git.LoadPatch(sess.DiffPath())
		if err != nil {
			return nil, nil, err
		}
		mode.patch, mode.files = patch, patch.Changes()
	case "plan":
		mode.planSlug = entry.PlanSlug
		if mode.planSlug == "" && len(args) > 0 {
			mode.planSlug = strings.TrimPrefix(args[0], "__plan:")
		}
		mode.docPath = filepath.Join(sess.Dir, "current.md")
		if entry.Mode == "" {
			mode.docPath = review.PlanCurrentPath(mode.planSlug)
		}
		if len(sess.CJ.CliArgs) >= 4 {
			mode.planFile = sess.CJ.CliArgs[3]
		}
	case "files":
		if len(args) != 1 {
			return nil, nil, fmt.Errorf("invalid document session arguments")
		}
		mode.docPath = args[0]
	case "git":
		if entry.Branch != "" {
			branch, err := git.CurrentBranch()
			if err != nil {
				return nil, nil, err
			}
			if branch != entry.Branch {
				return nil, nil, fmt.Errorf("session belongs to branch %s", entry.Branch)
			}
		}
		scope := "all"
		if len(args) == 1 && args[0] == "--staged" {
			scope = "staged"
		}
		if len(args) == 2 && args[0] == "--scope" {
			scope = args[1]
		}
		source, err := resolveSource(scope)
		if err != nil {
			return nil, nil, err
		}
		files, err := source.Files()
		if err != nil {
			return nil, nil, err
		}
		mode.source, mode.files, mode.ref, mode.staged = source, files, source.Base, source.Scope == "staged"
	default:
		return nil, nil, fmt.Errorf("unknown review mode %q", kind)
	}
	return sess, mode, nil
}

func resumeDiff(key string, args []string) error {
	sess, mode, err := loadReviewSession(key)
	if err != nil {
		return err
	}
	if mode.patch == nil {
		return fmt.Errorf("--diff requires a diff session")
	}
	if ipc.Alive(review.SocketPathFor(key)) {
		return fmt.Errorf("review is active; stop it before replacing its diff")
	}
	updated, err := resolveReviewMode(args)
	if err != nil {
		return err
	}
	updated.documents, updated.sessionKey = mode.documents, key
	sess.Meta.Args = updated.persistedCLIArgs()
	cfg, err := config.LoadCurrent()
	if err != nil {
		return err
	}
	return runReviewFlow(cfg, sess, updated)
}
