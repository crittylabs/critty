package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/crittylabs/critty/internal/config"
	"github.com/crittylabs/critty/internal/review"
)

var (
	commentSession string
	commentPlan    string
	commentOutput  string
	commentAuthor  string
	commentReplyTo string
	commentResolve bool
	commentPath    string
	commentJSON    bool
	commentFile    string
	commentClear   bool
)

var commentCmd = &cobra.Command{
	Use:   "comment [<path>[:<line[-end]>]] <body>",
	Short: "Add, reply to, bulk import, or clear review comments",
	Long: `Add, reply to, bulk import, or clear review comments.

Usage forms:
  critty comment [opts] <body>                        Review-level comment
  critty comment [opts] <path> <body>                 File-level comment
  critty comment [opts] <path>:<line[-end]> <body>    Line-level comment
  critty comment [opts] --reply-to <id> [--resolve] [--path <p>] <body>
  critty comment [opts] --json [--file <path>|-]      Bulk import (JSON array)
  critty comment [opts] --clear                       Remove the review

Bulk import entries are objects with "body" (required) and any of "file"
or "path", "line" (42 or "56-58"), "end_line", "author", "scope" ("review"
or "file"), "reply_to" (a comment ID), and "resolve" (bool).  For example:
  [{"file": "main.go", "line": 42, "body": "Unreachable."},
   {"reply_to": "c_152b0a", "body": "Removed the old names."}]`,
	Args: cobra.ArbitraryArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.LoadCurrent()
		if err != nil {
			return err
		}
		author := commentAuthor
		if author == "" {
			author = cfg.Author
		}
		output := commentOutput
		if output == "" {
			output = cfg.Output
		}

		switch {
		case commentJSON:
			if commentReplyTo != "" {
				return fmt.Errorf("--json and --reply-to cannot be used together; for a single reply use: critty comment --reply-to <id> [--author <name>] <body>")
			}
			if len(args) > 0 {
				return fmt.Errorf("--json takes no positional arguments")
			}
			return runCommentJSON(output, author)
		case commentClear:
			if len(args) > 0 {
				return fmt.Errorf("--clear takes no positional arguments")
			}
			return runCommentClear(output)
		case commentReplyTo != "":
			if len(args) != 1 {
				return fmt.Errorf("usage: critty comment --reply-to <comment-id> [--resolve] <body>")
			}
			return runCommentReply(output, author, commentReplyTo, args[0])
		case len(args) == 1:
			return runReviewComment(output, author, args[0])
		case len(args) == 2:
			return runFileOrLineComment(output, author, args[0], args[1])
		default:
			return fmt.Errorf("usage: critty comment [<path>[:<line[-end]>]] <body> (see --help)")
		}
	},
}

func resolveCommentTarget(output string) (*review.Session, error) {
	if commentPlan != "" {
		if commentSession != "" {
			return nil, fmt.Errorf("--plan cannot be combined with --session")
		}
		return review.ResolvePlan(review.Slugify(commentPlan))
	}
	return review.ResolveTarget(output, commentSession)
}

func runReviewComment(output, author, body string) error {
	sess, err := resolveCommentTarget(output)
	if err != nil {
		return err
	}
	if err := sess.Update(func(s *review.Session) error {
		s.AppendReviewComment(body, author, "")
		return nil
	}); err != nil {
		return err
	}
	fmt.Println("Added review comment")
	return nil
}

func runFileOrLineComment(output, author, loc, body string) error {
	sess, err := resolveCommentTarget(output)
	if err != nil {
		return err
	}

	if path, start, end, ok := splitLineSpec(loc); ok {
		if err := validateCommentPath(path); err != nil {
			return err
		}
		if err := sess.Update(func(s *review.Session) error {
			s.AppendLineComment(path, start, end, body, author, "")
			return nil
		}); err != nil {
			return err
		}
		if end > start {
			fmt.Printf("Added comment at %s:%d-%d\n", path, start, end)
		} else {
			fmt.Printf("Added comment at %s:%d\n", path, start)
		}
		return nil
	}

	if err := validateCommentPath(loc); err != nil {
		return err
	}
	if err := sess.Update(func(s *review.Session) error {
		// File-level comments must target a path on disk or in the latest
		// review state loaded inside the update transaction.
		if _, statErr := os.Stat(loc); statErr != nil {
			if _, ok := s.CJ.Files[review.NormalizePath(loc)]; !ok {
				return fmt.Errorf("invalid location %q — expected <path>:<line[-end]>, or a valid file path for file-level comments", loc)
			}
		}
		s.AppendFileComment(loc, body, author, "")
		return nil
	}); err != nil {
		return err
	}
	fmt.Printf("Added file comment on %s\n", review.NormalizePath(loc))
	return nil
}

// splitLineSpec splits "path:12" / "path:12-15" into path and line range.
func splitLineSpec(loc string) (path string, start, end int, ok bool) {
	colonIdx := strings.LastIndex(loc, ":")
	if colonIdx <= 0 || colonIdx == len(loc)-1 {
		return "", 0, 0, false
	}
	start, end, err := review.ParseLineSpec(loc[colonIdx+1:])
	if err != nil || start <= 0 {
		return "", 0, 0, false
	}
	if end < start {
		start, end = end, start
	}
	return loc[:colonIdx], start, end, true
}

func validateCommentPath(path string) error {
	normalized := strings.ReplaceAll(path, `\`, "/")
	if review.IsAbsoluteOrTraversal(normalized) {
		return fmt.Errorf("path %q must be relative and within the repository", path)
	}
	return nil
}

func runCommentReply(output, author, replyTo, body string) error {
	sess, err := resolveCommentTarget(output)
	if err != nil {
		return err
	}
	err = sess.Update(func(s *review.Session) error {
		return s.AppendReply(replyTo, body, author, "", commentResolve, commentPath)
	})
	var notFound *review.CommentNotFoundError
	if errors.As(err, &notFound) && commentSession == "" && commentPlan == "" {
		// The target may live in another registered review; redirect there.
		found, findErr := review.FindSessionsByCommentID(replyTo, sess.Key)
		if findErr != nil || len(found) == 0 {
			return err
		}
		if len(found) > 1 {
			return fmt.Errorf("comment %q found in multiple review sessions; use --session <id>", replyTo)
		}
		sess = found[0]
		fmt.Fprintf(os.Stderr, "Note: comment %s found in session %s\n", replyTo, sess.Key)
		if err := sess.Update(func(s *review.Session) error {
			return s.AppendReply(replyTo, body, author, "", commentResolve, commentPath)
		}); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	fmt.Printf("Replied to %s\n", replyTo)
	return nil
}

func runCommentJSON(output, author string) error {
	data, err := readCommentJSONInput()
	if err != nil {
		return err
	}
	var entries []review.BulkCommentEntry
	if err := unmarshalBulkEntries(data, &entries); err != nil {
		return err
	}

	sess, err := resolveCommentTarget(output)
	if err != nil {
		return err
	}
	sess, redirected, err := redirectBulkTarget(sess, entries)
	if err != nil {
		return err
	}
	stats, err := sess.ApplyBulk(entries, author, "")
	if err != nil {
		return err
	}
	if redirected {
		fmt.Fprintf(os.Stderr, "Note: replies routed to session %s (not the cwd-resolved review)\n", sess.Key)
	}
	fmt.Println(formatBulkResult(stats))
	return nil
}

// redirectBulkTarget applies crit's single-target rule: a bulk call writes to
// exactly one review.  When every reply ID is missing from the primary and
// they all live in one other session, the whole bulk goes there; a split
// across files is an error.
func redirectBulkTarget(primary *review.Session, entries []review.BulkCommentEntry) (*review.Session, bool, error) {
	seen := map[string]bool{}
	var replyIDs []string
	for _, e := range entries {
		if e.ReplyTo == "" || seen[e.ReplyTo] {
			continue
		}
		seen[e.ReplyTo] = true
		replyIDs = append(replyIDs, e.ReplyTo)
	}
	if len(replyIDs) == 0 {
		return primary, false, nil
	}

	var inPrimary, missing []string
	for _, id := range replyIDs {
		if primary.CJ.ContainsCommentID(id) {
			inPrimary = append(inPrimary, id)
		} else {
			missing = append(missing, id)
		}
	}
	if len(missing) == 0 {
		return primary, false, nil
	}
	if len(inPrimary) > 0 {
		return nil, false, fmt.Errorf(
			"bulk targets multiple review files: %v exist in session %s, but %v do not — split into per-file bulks",
			inPrimary, primary.Key, missing)
	}
	if commentSession != "" || commentPlan != "" {
		return nil, false, fmt.Errorf("reply targets %v not found in selected review session", missing)
	}

	var alt *review.Session
	for _, id := range missing {
		found, err := review.FindSessionsByCommentID(id, primary.Key)
		if err != nil {
			return nil, false, err
		}
		if len(found) == 0 {
			return nil, false, fmt.Errorf("reply target %s: comment not found in any review session", id)
		}
		if len(found) > 1 {
			return nil, false, fmt.Errorf("reply target %s found in multiple review sessions; use --session <id>", id)
		}
		if alt == nil {
			alt = found[0]
			continue
		}
		if found[0].Key != alt.Key {
			return nil, false, fmt.Errorf(
				"bulk targets multiple review files: %s in session %s, %s in session %s — split into per-file bulks",
				missing[0], alt.Key, id, found[0].Key)
		}
	}
	return alt, true, nil
}

func readCommentJSONInput() ([]byte, error) {
	if commentFile != "" && commentFile != "-" {
		data, err := os.ReadFile(commentFile)
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", commentFile, err)
		}
		return data, nil
	}
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		return nil, fmt.Errorf("reading stdin: %w", err)
	}
	return data, nil
}

func unmarshalBulkEntries(data []byte, entries *[]review.BulkCommentEntry) error {
	if err := json.Unmarshal(data, entries); err != nil {
		return fmt.Errorf("parsing comment JSON: %w", err)
	}
	return nil
}

func formatBulkResult(stats review.BulkStats) string {
	var parts []string
	if stats.Comments > 0 || stats.Replies == 0 {
		parts = append(parts, fmt.Sprintf("%d comment%s", stats.Comments, pluralSuffix(stats.Comments)))
	}
	if stats.Replies > 0 {
		parts = append(parts, fmt.Sprintf("%d repl%s", stats.Replies, pluralY(stats.Replies)))
	}
	return "Added " + strings.Join(parts, " and ")
}

func pluralSuffix(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

func pluralY(n int) string {
	if n == 1 {
		return "y"
	}
	return "ies"
}

func runCommentClear(output string) error {
	sess, err := resolveCommentTarget(output)
	if err != nil {
		return err
	}
	if err := clearSavedReview(sess); err != nil {
		return err
	}
	fmt.Println("Cleared review file")
	return nil
}

func init() {
	rootCmd.AddCommand(commentCmd)
	commentCmd.Flags().StringVar(&commentSession, "session", "", "target a review session by ID")
	commentCmd.Flags().StringVar(&commentPlan, "plan", "", "target a plan review by slug")
	commentCmd.Flags().StringVarP(&commentOutput, "output", "o", "", "review data root")
	commentCmd.Flags().StringVar(&commentAuthor, "author", "", "comment author (defaults to config author)")
	commentCmd.Flags().StringVar(&commentReplyTo, "reply-to", "", "reply to an existing comment")
	commentCmd.Flags().BoolVar(&commentResolve, "resolve", false, "resolve the parent after replying")
	commentCmd.Flags().StringVar(&commentPath, "path", "", "file path to disambiguate a reply target")
	commentCmd.Flags().BoolVar(&commentJSON, "json", false, "read bulk comments as a JSON array")
	commentCmd.Flags().StringVarP(&commentFile, "file", "f", "", "read bulk JSON from a file (- for stdin)")
	commentCmd.Flags().BoolVar(&commentClear, "clear", false, "remove the review folder")
}
