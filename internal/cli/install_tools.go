package cli

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"slices"

	"github.com/spf13/cobra"

	"github.com/crittylabs/critty/internal/mathrender"
)

type toolInstall struct {
	name string
	args []string
}

func newInstallToolsCmd() *cobra.Command {
	var manager string
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "tools [svg|math|mermaid|all]",
		Short: "Install optional preview tools with mise or Homebrew",
		// Tool setup does not read Critty state; dry runs must not migrate it.
		PersistentPreRunE: func(*cobra.Command, []string) error { return nil },
		Long: `Install optional SVG, math, and Mermaid preview tools for your user.
With no target, install tools for all three features. Existing PATH tools are
kept. Agent integrations and renderer configuration are unchanged.

Auto selection prefers mise, then Homebrew. Use --manager to select one.
The manager must already be installed. Commands run from your home directory;
mise writes its global tool configuration, and Homebrew uses its usual prefix.
Installation downloads packages and may run their installation hooks.

mise installs Merman for SVG and Mermaid, plus RaTeX and TinyTeX for math.
TinyTeX includes an AMS math and dvipng setup step and is supported here on
macOS and x86_64 Linux. Homebrew installs librsvg for SVG, Merman and Fontconfig
for Mermaid, and the full TeX Live distribution for math (a large download).
Homebrew does not install RaTeX. Math uses the existing LaTeX fallback.

Use --dry-run to inspect commands without installing packages or changing
configuration. After installation, refresh your shell's PATH and run
critty doctor. Missing fonts and disabled or overridden renderers may still
need configuration; this command does not install fonts.`,
		Args:      cobra.MaximumNArgs(1),
		ValidArgs: []string{"svg", "math", "mermaid", "all"},
		RunE: func(cmd *cobra.Command, args []string) error {
			target := "all"
			if len(args) != 0 {
				target = args[0]
			}
			if !slices.Contains(cmd.ValidArgs, target) {
				return fmt.Errorf("unknown tool target %q (available: svg, math, mermaid, all)", target)
			}
			selected, err := selectToolManager(manager, exec.LookPath)
			if err != nil {
				return err
			}
			plan, err := previewToolPlan(selected, target, runtime.GOOS, runtime.GOARCH, exec.LookPath, mathrender.FindRaTeX)
			if err != nil {
				return err
			}
			if len(plan) == 0 {
				_, err := fmt.Fprintln(cmd.OutOrStdout(), "Preview tools already available on PATH. Run critty doctor to check configuration.")
				return err
			}
			home, err := os.UserHomeDir()
			if err != nil {
				return err
			}
			for _, step := range plan {
				if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s: %s %q\n", step.name, selected, step.args); err != nil {
					return err
				}
				if dryRun {
					continue
				}
				proc := exec.CommandContext(cmd.Context(), selected, step.args...)
				proc.Dir = home
				proc.Stdin = cmd.InOrStdin()
				proc.Stdout = cmd.OutOrStdout()
				proc.Stderr = cmd.ErrOrStderr()
				if err := proc.Run(); err != nil {
					return fmt.Errorf("install %s: %w (earlier steps are kept; rerun to retry)", step.name, err)
				}
			}
			if !dryRun {
				_, err := fmt.Fprintln(cmd.OutOrStdout(), "Installation finished. Refresh your shell's PATH, then run critty doctor.")
				return err
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&manager, "manager", "auto", "package manager: auto (mise first), mise, brew")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "show installation commands without running them")
	return cmd
}

func selectToolManager(manager string, find func(string) (string, error)) (string, error) {
	if manager == "auto" {
		for _, name := range []string{"mise", "brew"} {
			if _, err := find(name); err == nil {
				return name, nil
			}
		}
		return "", fmt.Errorf("install mise or Homebrew first, then rerun critty install tools")
	}
	if manager != "mise" && manager != "brew" {
		return "", fmt.Errorf("unknown manager %q (available: auto, mise, brew)", manager)
	}
	if _, err := find(manager); err != nil {
		return "", fmt.Errorf("%s not found on PATH", manager)
	}
	return manager, nil
}

func previewToolPlan(manager, target, goos, goarch string, find func(string) (string, error), findRaTeX func(string) (string, error)) ([]toolInstall, error) {
	has := func(name string) bool {
		_, err := find(name)
		return err == nil
	}
	wants := func(name string) bool { return target == "all" || target == name }
	var plan []toolInstall
	add := func(name string, miseArgs []string, formula string) {
		var args []string
		if manager == "brew" {
			args = []string{"install", formula}
		} else {
			args = append([]string{"use", "--global"}, miseArgs...)
		}
		plan = append(plan, toolInstall{name, args})
	}
	needSVG := wants("svg") && !has("rsvg-convert") && !has("merman-cli") && !has("resvg")
	needMermaid := wants("mermaid") && !has("merman-cli") && !has("mmdr")
	if needMermaid || (manager == "mise" && needSVG) {
		add("Merman", []string{"github:Latias94/merman[matching=merman-cli]@latest"}, "merman-cli")
	}
	if manager == "brew" && needSVG {
		add("SVG converter", nil, "librsvg")
	}
	if manager == "brew" && wants("mermaid") && !has("fc-match") {
		add("Fontconfig", nil, "fontconfig")
	}
	if wants("math") {
		if manager == "mise" {
			if path, err := findRaTeX(""); err != nil || path == "" {
				add("RaTeX", []string{"github:erweixin/RaTeX[matching=ratex-cli]@latest"}, "")
			}
		}
		if !has("latex") || !has("dvipng") {
			if manager == "mise" && goos != "darwin" && (goos != "linux" || goarch != "amd64") {
				return nil, fmt.Errorf("TinyTeX installation requires macOS or x86_64 Linux; install latex and dvipng separately, or select --manager brew")
			}
			add("LaTeX and dvipng", []string{
				"--tool-option", `os=["linux/x64","macos"]`,
				"tinytex@latest",
			}, "texlive")
			if manager == "mise" {
				plan = append(plan,
					toolInstall{"Update TinyTeX package manager", []string{"exec", "tinytex@latest", "--", "tlmgr", "update", "--self"}},
					toolInstall{"AMS math and dvipng", []string{"exec", "tinytex@latest", "--", "tlmgr", "install", "amsmath", "amsfonts", "dvipng"}},
				)
			}
		}
	}
	return plan, nil
}

func init() {
	installCmd.AddCommand(newInstallToolsCmd())
}
