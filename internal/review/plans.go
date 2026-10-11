package review

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/crittylabs/critty/internal/xdg"
)

// Plan reviews mirror crit's layout: each plan lives under a slug-named
// storage directory holding numbered immutable versions, the latest copy as
// current.md, and the review folder as a .crit subdirectory.

// PlansDir returns the root of plan storage.
func PlansDir() string {
	return filepath.Join(xdg.StateHome(), "plans")
}

// PlanStorageDir returns the storage directory for a plan slug.
func PlanStorageDir(slug string) string {
	return filepath.Join(PlansDir(), slug)
}

// PlanCurrentPath returns the path of the plan's latest content.
func PlanCurrentPath(slug string) string {
	return filepath.Join(PlanStorageDir(slug), "current.md")
}

// PlanSessionKey computes the session key for a plan review, matching
// crit's cwd + "__plan:"+slug derivation (the marker hashes in the branch
// position).
func PlanSessionKey(cwd, slug string) string {
	return SessionKey(cwd, "__plan:"+slug, nil)
}

// OpenPlanSession opens the review session stored inside a plan directory.
func OpenPlanSession(slug string) (*Session, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("resolving cwd: %w", err)
	}
	key := PlanSessionKey(cwd, slug)
	s, err := openSessionAt(key, filepath.Join(PlanStorageDir(slug), ".crit"))
	if err != nil {
		return nil, err
	}
	s.Meta = SessionEntry{Key: key, CWD: cwd, Args: []string{"__plan:" + slug}}
	return s, nil
}

var nonAlphaNum = regexp.MustCompile(`[^a-z0-9]+`)

// Slugify converts a string to a URL-safe slug.
func Slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = nonAlphaNum.ReplaceAllString(s, "-")
	return strings.Trim(s, "-")
}

// ResolveSlug derives a plan slug from its content: the first markdown
// heading plus today's date, or a timestamp fallback.
func ResolveSlug(content []byte) string {
	date := time.Now().Format("2006-01-02")
	for _, line := range strings.SplitN(string(content), "\n", 30) {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "# ") {
			if s := Slugify(strings.TrimPrefix(line, "# ")); s != "" {
				return s + "-" + date
			}
		}
	}
	return "plan-" + time.Now().Format("2006-01-02-150405")
}

// SavePlanVersion saves content as the next numbered version and updates
// current.md, returning the 1-based version number.
func SavePlanVersion(slug string, content []byte) (int, error) {
	return SavePlanVersionAt(PlanStorageDir(slug), content)
}

// SavePlanVersionAt versions a plan inside its session's storage directory.
func SavePlanVersionAt(dir string, content []byte) (int, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return 0, fmt.Errorf("creating plan storage dir: %w", err)
	}

	ver := latestPlanVersion(dir) + 1
	versionPath := filepath.Join(dir, fmt.Sprintf("v%03d.md", ver))
	if err := os.WriteFile(versionPath, content, 0o644); err != nil {
		return 0, fmt.Errorf("writing version %d: %w", ver, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "current.md"), content, 0o644); err != nil {
		return 0, fmt.Errorf("writing current.md: %w", err)
	}
	return ver, nil
}

// latestPlanVersion returns the highest version number in the directory,
// or 0 when no versions exist.
func latestPlanVersion(dir string) int {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	maxVer := 0
	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(name, "v") || !strings.HasSuffix(name, ".md") {
			continue
		}
		var n int
		numStr := strings.TrimPrefix(strings.TrimSuffix(name, ".md"), "v")
		if _, err := fmt.Sscanf(numStr, "%d", &n); err == nil && n > maxVer {
			maxVer = n
		}
	}
	return maxVer
}
