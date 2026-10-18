// Package prompt renders the templated agent prompts emitted when a review
// finishes, following crit's hook naming, resolution order, and template
// variables (https://github.com/tomasz-tomczyk/crit).
package prompt

import (
	"bytes"
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/crittylabs/critty/internal/xdg"
)

//go:embed stock/*.md
var stockFS embed.FS

// StockTemplates returns the embedded stock template file names and contents,
// for `critty install prompts` and staleness checking.
func StockTemplates() map[string][]byte {
	entries, err := stockFS.ReadDir("stock")
	if err != nil {
		return nil
	}
	out := make(map[string][]byte, len(entries))
	for _, e := range entries {
		if data, err := stockFS.ReadFile("stock/" + e.Name()); err == nil {
			out[e.Name()] = data
		}
	}
	return out
}

const fallbackPrompt = "Review finished."

// Context carries the values exposed to finish templates.
type Context struct {
	ReviewPath        string
	SessionKey        string
	Mode              string // "diff" (git) or "files"
	InternalMode      string // "git", "diff", "files", or "plan"
	PlanSlug          string
	UnresolvedCount   int
	TotalCount        int
	FilesWithComments []string
	UnresolvedJSON    string
	CommentsJSON      string
	Approved          bool
	NextRoundCmd      string
}

// TemplateData returns the snake_case variable map, matching crit's names.
func (c Context) TemplateData() map[string]any {
	internalMode := c.InternalMode
	if internalMode == "" {
		if c.Mode == "diff" {
			internalMode = "git"
		} else {
			internalMode = "files"
		}
	}
	return map[string]any{
		"review_path":              c.ReviewPath,
		"session_key":              c.SessionKey,
		"mode":                     c.Mode,
		"internal_session_mode":    internalMode,
		"plan_slug":                c.PlanSlug,
		"unresolved_count":         c.UnresolvedCount,
		"total_count":              c.TotalCount,
		"files_with_comments":      c.FilesWithComments,
		"comments_unresolved_json": c.UnresolvedJSON,
		"comments_json":            c.CommentsJSON,
		"approved":                 c.Approved,
		"next_round_cmd":           c.NextRoundCmd,
		"comments_cmd":             fmt.Sprintf("critty comments --json --session %s", c.SessionKey),
		"comments_all_cmd":         fmt.Sprintf("critty comments --json --all --session %s", c.SessionKey),
	}
}

// HookForFinish names the hook that fires for a finish result.
func HookForFinish(approved bool) string {
	if approved {
		return "on_finish_approved"
	}
	return "on_finish_unresolved"
}

// RenderFinish resolves and renders the finish prompt.  prompts is the
// merged config prompts map; projectRoot may be empty.  Rendering errors
// fall back to a plain message rather than blocking the finish.
func RenderFinish(prompts map[string]string, projectRoot string, ctx Context) string {
	hook := HookForFinish(ctx.Approved)
	text, err := resolveTemplateText(prompts, projectRoot, hook, ctx.Mode)
	if err != nil {
		return fallbackPrompt
	}
	rendered, err := render(text, ctx.TemplateData())
	if err != nil {
		return fallbackPrompt
	}
	return strings.TrimRight(rendered, "\n")
}

// resolveTemplateText applies crit's precedence: config prompts entry
// (mode-specific, then generic; project entries already win in the merged
// map), then project and global prompt directories, then the stock template.
// Project prompts take precedence over global prompts.
func resolveTemplateText(prompts map[string]string, projectRoot, hook, mode string) (string, error) {
	for _, key := range []string{hook + ":" + mode, hook} {
		if v, ok := prompts[key]; ok {
			return loadTemplateValue(v, projectRoot)
		}
	}

	names := []string{
		fmt.Sprintf("%s.%s.md", hook, mode),
		hook + ".md",
	}
	var dirs []string
	if projectRoot != "" {
		dirs = append(dirs, filepath.Join(projectRoot, ".critty", "prompts"))
	}
	dirs = append(dirs, filepath.Join(xdg.ConfigHome(), "prompts"))
	for _, dir := range dirs {
		for _, name := range names {
			data, err := os.ReadFile(filepath.Join(dir, name))
			if err == nil {
				return string(data), nil
			}
		}
	}

	data, err := stockFS.ReadFile("stock/" + hook + ".md")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// loadTemplateValue interprets crit's "inline:" and "file:" value forms.
func loadTemplateValue(v, projectRoot string) (string, error) {
	switch {
	case strings.HasPrefix(v, "inline:"):
		return strings.TrimPrefix(v, "inline:"), nil
	case strings.HasPrefix(v, "file:"):
		path := strings.TrimPrefix(v, "file:")
		if !filepath.IsAbs(path) && projectRoot != "" {
			path = filepath.Join(projectRoot, path)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}
		return string(data), nil
	default:
		return "", fmt.Errorf("prompt value must start with inline: or file:, got %q", v)
	}
}

func render(text string, data map[string]any) (string, error) {
	tmpl, err := template.New("prompt").Parse(text)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}
