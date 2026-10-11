package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/crittylabs/critty/internal/document"
	"github.com/crittylabs/critty/internal/git"
)

var reviewFocus string

func addFocusFlag(cmd *cobra.Command) {
	cmd.Flags().StringVar(&reviewFocus, "focus", "", "focus a reviewed file, optionally at a line: PATH[:LINE]")
}

func parseFocus(value string) (string, int, error) {
	path, line := value, 0
	if i := strings.LastIndexByte(value, ':'); i >= 0 {
		suffix := value[i+1:]
		if suffix != "" && strings.Trim(suffix, "+-0123456789") == "" {
			var err error
			line, err = strconv.Atoi(suffix)
			if err != nil || line < 1 {
				return "", 0, fmt.Errorf("line number must be positive")
			}
			path = value[:i]
		}
	}
	if path == "" {
		return "", 0, fmt.Errorf("file path required")
	}
	return path, line, nil
}

func resolveFocus(mode *reviewMode) error {
	if reviewFocus == "" {
		return nil
	}
	path, line, err := parseFocus(reviewFocus)
	if err != nil {
		return err
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		abs = resolved
	}
	match := func(candidate string) bool {
		full, err := filepath.Abs(candidate)
		if resolved, resolveErr := filepath.EvalSymlinks(full); resolveErr == nil {
			full = resolved
		}
		return err == nil && full == abs
	}
	for _, doc := range mode.documents {
		if doc.Removed || (!match(doc.Source) && path != doc.Path()) {
			continue
		}
		if line > 0 && !document.FromContent(doc.Path(), []byte(doc.Content)).HasLine(line) {
			return fmt.Errorf("focus line %d is not in the review: %s", line, path)
		}
		mode.focusPath, mode.focusLine = doc.Path(), line
		return nil
	}
	var data []byte
	if !mode.code() {
		if !match(mode.docPath) && (mode.planFile == "" || !match(mode.planFile)) {
			return fmt.Errorf("focus file is not in the review: %s", path)
		}
		path = mode.docPath
		if line > 0 {
			if mode.planContent != nil {
				data = mode.planContent
			} else {
				data, err = os.ReadFile(path)
			}
		}
	} else {
		found := false
		for _, file := range mode.files {
			if !match(file.Path) {
				continue
			}
			found, path = true, file.Path
			if line == 0 {
				break
			}
			if file.IsBinary() || file.Status == git.StatusDeleted {
				return fmt.Errorf("focus line is unavailable in %s", path)
			}
			switch {
			case mode.patch != nil:
				file := mode.patch.File(path)
				doc := document.FromContent(path, []byte(file.Content))
				doc.Known = file.Known
				if !doc.HasLine(line) {
					return fmt.Errorf("focus line %d is not in the review: %s", line, path)
				}
				data = []byte(file.Content)
			case mode.source != nil:
				data, err = mode.source.Content(path)
			case mode.staged:
				data, err = git.FileContentFromIndex(path)
			default:
				data, err = os.ReadFile(path)
			}
			break
		}
		if !found {
			return fmt.Errorf("focus file is not in the review: %s", path)
		}
	}
	if err != nil {
		return err
	}
	if line > 0 && !document.FromContent(path, data).HasLine(line) {
		return fmt.Errorf("focus line %d is not in the review: %s", line, path)
	}
	mode.focusPath, mode.focusLine = path, line
	return nil
}
