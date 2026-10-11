package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/crittylabs/critty/internal/git"
)

func validateScopeFlags(cmd *cobra.Command, _ []string) error {
	if cmd.Flags().Changed("focus") && reviewFocus == "" {
		return fmt.Errorf("--focus requires a file path")
	}
	if cmd.Flags().Changed("diff") && reviewDiff == "" {
		return fmt.Errorf("--diff requires a file path or -")
	}
	if cmd.Flags().Changed("scope") && reviewScope == "" {
		return fmt.Errorf("--scope requires a value")
	}
	return nil
}

func resolveCodeScope() (*reviewMode, error) {
	if reviewStaged && reviewUnstaged {
		return nil, fmt.Errorf("--staged cannot be combined with --unstaged")
	}
	scope := reviewScope
	alias := ""
	if reviewStaged {
		alias = "staged"
	} else if reviewUnstaged {
		alias = "unstaged"
	}
	if alias != "" {
		if scope != "" && scope != alias {
			return nil, fmt.Errorf("--%s conflicts with --scope=%s", alias, scope)
		}
		scope = alias
	}
	if scope == "" {
		scope = "all"
	}
	source, err := resolveSource(scope)
	if err != nil {
		return nil, err
	}
	files, err := source.Files()
	if err != nil {
		return nil, err
	}
	if len(files) == 0 && len(attachmentPaths) == 0 {
		return nil, fmt.Errorf("no changes in the selected scope")
	}
	return &reviewMode{files: files, ref: source.Base, staged: source.Scope == "staged", source: source}, nil
}

func resolveSource(scope string) (*git.ReviewSource, error) {
	source := git.ReviewSource{Scope: scope, Base: "HEAD"}
	if scope != "all" && scope != "staged" && scope != "unstaged" {
		resolved, err := git.ResolveRange(scope)
		if err != nil {
			return nil, err
		}
		source = resolved
	}
	return &source, nil
}
