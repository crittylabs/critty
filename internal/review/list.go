package review

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/crittylabs/critty/internal/git"
)

// ListedComment is a comment flattened for CLI output, matching crit's
// `comments` format.  It embeds Comment so new review.json fields flow
// through automatically, alongside file identity and conversion metadata.
type ListedComment struct {
	Textconv git.Textconv `json:"textconv,omitzero"`
	Scope    string       `json:"scope"`
	Path     *string      `json:"path,omitempty"`
	Comment
}

func commentScope(c Comment) string {
	switch c.Scope {
	case "review", "file":
		return c.Scope
	default:
		return "line"
	}
}

// ListComments flattens a review into display order: review-level comments
// first, then files sorted by path, file-level before line-level, line-level
// sorted by start line.
func (cj *CritJSON) ListComments(unresolvedOnly bool) []ListedComment {
	var out []ListedComment

	for _, c := range cj.ReviewComments {
		if unresolvedOnly && c.Resolved {
			continue
		}
		out = append(out, toListedComment("review", "", c))
	}

	paths := make([]string, 0, len(cj.Files))
	for path := range cj.Files {
		paths = append(paths, path)
	}
	sort.Strings(paths)

	for _, path := range paths {
		var fileLevel, lineLevel []Comment
		for _, c := range cj.Files[path].Comments {
			if unresolvedOnly && c.Resolved {
				continue
			}
			if commentScope(c) == "file" {
				fileLevel = append(fileLevel, c)
			} else {
				lineLevel = append(lineLevel, c)
			}
		}
		sort.Slice(lineLevel, func(i, j int) bool {
			return lineLevel[i].StartLine < lineLevel[j].StartLine
		})
		for _, c := range fileLevel {
			entry := toListedComment("file", path, c)
			entry.Textconv = cj.Files[path].Textconv
			out = append(out, entry)
		}
		for _, c := range lineLevel {
			entry := toListedComment("line", path, c)
			entry.Textconv = cj.Files[path].Textconv
			out = append(out, entry)
		}
	}
	return out
}

func toListedComment(scope, path string, c Comment) ListedComment {
	lc := ListedComment{Scope: scope, Comment: c}
	if path != "" {
		lc.Path = &path
	}
	if scope == "line" && c.EndLine == 0 {
		lc.EndLine = c.StartLine
	}
	return lc
}

// FormatCommentsText renders the human-readable `comments` listing.
func FormatCommentsText(entries []ListedComment, unresolvedOnly bool) string {
	n := len(entries)
	if n == 0 {
		if unresolvedOnly {
			return "No unresolved comments."
		}
		return "No comments."
	}
	var b strings.Builder
	if unresolvedOnly {
		fmt.Fprintf(&b, "%d unresolved comment%s:\n", n, plural(n))
	} else {
		fmt.Fprintf(&b, "%d comment%s:\n", n, plural(n))
	}
	for _, e := range entries {
		b.WriteByte('\n')
		b.WriteString(formatCommentHeader(e))
		if e.Quote != "" {
			b.WriteByte('\n')
			b.WriteString(indentLines(2, "quote:  "+e.Quote))
		}
		if e.Anchor != "" {
			b.WriteByte('\n')
			b.WriteString(indentLines(2, "anchor: "+e.Anchor))
		}
		b.WriteByte('\n')
		b.WriteString(indentLines(2, "body:   "+e.Body))
		if len(e.Replies) > 0 {
			b.WriteString("\n  replies:")
			for _, r := range e.Replies {
				author := r.Author
				if author == "" {
					author = "?"
				}
				b.WriteByte('\n')
				b.WriteString(indentLines(4, fmt.Sprintf("- [%s] %s: %s", r.ID, author, r.Body)))
			}
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

func formatCommentHeader(e ListedComment) string {
	header := fmt.Sprintf("[%s] %s", e.ID, e.Scope)
	if e.Path != nil {
		header += " " + *e.Path
		if e.Scope == "line" {
			header += formatLineLoc(e.StartLine, e.EndLine)
		}
	}
	if e.Drifted {
		header += " (drifted)"
	}
	return header
}

func formatLineLoc(start, end int) string {
	if end == 0 || end == start {
		return fmt.Sprintf(":%d", start)
	}
	return fmt.Sprintf(":%d-%d", start, end)
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

func indentLines(spaces int, s string) string {
	pad := strings.Repeat(" ", spaces)
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = pad + line
	}
	return strings.Join(lines, "\n")
}

// EncodeCommentsJSON marshals a listing, normalizing nil to [].
func EncodeCommentsJSON(entries []ListedComment) ([]byte, error) {
	if entries == nil {
		entries = []ListedComment{}
	}
	return json.MarshalIndent(entries, "", "  ")
}
