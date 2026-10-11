package cli

import (
	"bytes"
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/knu/tcrit/internal/prompt"
	"github.com/knu/tcrit/internal/xdg"
)

//go:embed skill/critty/SKILL.md skill/critty-cli/SKILL.md
var skillContent embed.FS

var installForce bool

// integrationFile is one installable asset with its project- and
// global-relative destinations.
type integrationFile struct {
	content    func() ([]byte, error)
	dest       string // relative to the project root (cwd)
	globalDest string // relative to $HOME, or absolute when starting with "/"
}

// skillVariant describes how the embedded Claude Code skills are rewritten
// for another agent: how that agent invokes a skill or command, how replies
// are attributed, and which frontmatter keys it does not understand.
type skillVariant struct {
	author         string
	skillRef       func(name string) string
	argumentsToken string
	dropKeys       []string
}

var skillVariants = map[string]skillVariant{
	"claude-code": {
		author:         "Claude Code",
		skillRef:       func(name string) string { return "/" + name },
		argumentsToken: "$ARGUMENTS",
	},
	"codex": {
		author:         "Codex",
		skillRef:       func(name string) string { return "$" + name },
		argumentsToken: "<arguments>",
		dropKeys:       []string{"allowed-tools", "argument-hint", "user-invocable"},
	},
	"opencode": {
		author: "OpenCode",
		skillRef: func(name string) string {
			if name == "critty" {
				return "/critty" // installed as a command
			}
			return name
		},
		argumentsToken: "$ARGUMENTS",
		dropKeys:       []string{"allowed-tools", "argument-hint", "user-invocable"},
	},
	"gemini": {
		author:         "Gemini",
		skillRef:       func(name string) string { return name },
		argumentsToken: "<arguments>",
		dropKeys:       []string{"allowed-tools", "argument-hint", "user-invocable"},
	},
}

var skillNames = []string{"critty", "critty-cli"}

// renderSkill returns the embedded skill rewritten for the named agent.
func renderSkill(name, agent string) ([]byte, error) {
	data, err := skillContent.ReadFile("skill/" + name + "/SKILL.md")
	if err != nil {
		return nil, err
	}
	variant := skillVariants[agent]
	base := skillVariants["claude-code"]
	if agent == "claude-code" {
		return data, nil
	}
	for _, skill := range skillNames {
		data = bytes.ReplaceAll(data,
			[]byte("`"+base.skillRef(skill)+"`"),
			[]byte("`"+variant.skillRef(skill)+"`"))
	}
	for _, quote := range []string{"'", `"`} {
		data = bytes.ReplaceAll(data, []byte(quote+base.author+quote), []byte(quote+variant.author+quote))
	}
	data = bytes.ReplaceAll(data, []byte(base.argumentsToken), []byte(variant.argumentsToken))
	return dropFrontmatterKeys(data, variant.dropKeys), nil
}

// dropFrontmatterKeys removes the given top-level keys from the YAML
// frontmatter at the start of data.  Only single-line keys are handled,
// which is all the embedded skills use.
func dropFrontmatterKeys(data []byte, keys []string) []byte {
	if len(keys) == 0 || !bytes.HasPrefix(data, []byte("---\n")) {
		return data
	}
	rest := data[len("---\n"):]
	end := bytes.Index(rest, []byte("\n---\n"))
	if end < 0 {
		return data
	}
	var kept [][]byte
	for _, line := range bytes.Split(rest[:end], []byte("\n")) {
		drop := false
		for _, key := range keys {
			if bytes.HasPrefix(line, []byte(key+":")) {
				drop = true
				break
			}
		}
		if !drop {
			kept = append(kept, line)
		}
	}
	var out bytes.Buffer
	out.WriteString("---\n")
	out.Write(bytes.Join(kept, []byte("\n")))
	out.Write(rest[end:])
	return out.Bytes()
}

func skillFile(name, agent, dir string) integrationFile {
	rel := filepath.Join(dir, "skills", name, "SKILL.md")
	return integrationFile{
		content:    func() ([]byte, error) { return renderSkill(name, agent) },
		dest:       rel,
		globalDest: rel,
	}
}

// integrations maps install target names to their files, following crit's
// naming (the Claude Code agent is "claude-code").
func integrations() map[string][]integrationFile {
	m := map[string][]integrationFile{}

	m["claude-code"] = []integrationFile{
		skillFile("critty", "claude-code", ".claude"),
		skillFile("critty-cli", "claude-code", ".claude"),
	}

	m["codex"] = []integrationFile{
		skillFile("critty", "codex", ".agents"),
		skillFile("critty-cli", "codex", ".agents"),
	}

	// OpenCode reads project files from .opencode/ but global ones from its
	// XDG config directory (~/.config/opencode by default).
	openCodeGlobal := xdg.AppConfigHome("opencode")
	openCodeCLI := skillFile("critty-cli", "opencode", ".opencode")
	openCodeCLI.globalDest = filepath.Join(openCodeGlobal, "skills", "critty-cli", "SKILL.md")
	m["opencode"] = []integrationFile{
		{
			content: func() ([]byte, error) {
				data, err := renderSkill("critty", "opencode")
				if err != nil {
					return nil, err
				}
				return dropFrontmatterKeys(data, []string{"name"}), nil // commands are named by their file
			},
			dest:       filepath.Join(".opencode", "commands", "critty.md"),
			globalDest: filepath.Join(openCodeGlobal, "commands", "critty.md"), // absolute
		},
		openCodeCLI,
	}

	m["gemini"] = []integrationFile{
		skillFile("critty", "gemini", ".gemini"),
		skillFile("critty-cli", "gemini", ".gemini"),
	}

	var prompts []integrationFile
	stock := prompt.StockTemplates()
	names := make([]string, 0, len(stock))
	for name := range stock {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		data := stock[name]
		prompts = append(prompts, integrationFile{
			content:    func() ([]byte, error) { return data, nil },
			dest:       filepath.Join(".tcrit", "prompts", name),
			globalDest: filepath.Join(xdg.ConfigHome(), "prompts", name), // absolute
		})
	}
	m["prompts"] = prompts

	return m
}

var agentTargets = []string{"claude-code", "codex", "opencode", "gemini"}

func integrationTargets(target string, reg map[string][]integrationFile) ([]string, error) {
	if target == "all" {
		return agentTargets, nil
	}
	if _, ok := reg[target]; ok {
		return []string{target}, nil
	}

	available := make([]string, 0, len(reg)+1)
	for name := range reg {
		available = append(available, name)
	}
	sort.Strings(available)
	return nil, fmt.Errorf("unknown target %q (available: %s, all)", target, strings.Join(available, ", "))
}

var installCmd = &cobra.Command{
	Use:   "install [--force] <claude-code|codex|opencode|gemini|prompts|all>",
	Short: "Install agent integrations or stock prompt templates",
	Long: `Install agent integrations (skills) or the stock finish prompt
templates.

Run from your home directory to install globally, or from a repository
root to install for that project only, following crit's convention.

Targets:
  claude-code  Claude Code skills (critty, critty-cli)
  codex        Codex skills (critty, critty-cli)
  opencode     OpenCode command (/critty) and skill (critty-cli)
  gemini       Gemini CLI skills (critty, critty-cli)
  prompts      Stock finish prompt templates (customize after copying)
  all          claude-code + codex + opencode + gemini`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		target := args[0]
		reg := integrations()
		names, err := integrationTargets(target, reg)
		if err != nil {
			return err
		}

		global, err := isGlobalInstall()
		if err != nil {
			return err
		}
		for _, name := range names {
			for _, f := range reg[name] {
				if err := installIntegrationFile(f, global); err != nil {
					return err
				}
			}
		}
		return nil
	},
}

// isGlobalInstall reports whether the install targets global locations:
// crit's rule is simply "run from $HOME".
func isGlobalInstall() (bool, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return false, err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return false, err
	}
	return cwd == home, nil
}

func integrationDest(f integrationFile, global bool) (string, error) {
	if !global {
		return f.dest, nil
	}
	if filepath.IsAbs(f.globalDest) {
		return f.globalDest, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, f.globalDest), nil
}

func installIntegrationFile(f integrationFile, global bool) error {
	dest, err := integrationDest(f, global)
	if err != nil {
		return err
	}
	data, err := f.content()
	if err != nil {
		return err
	}

	if existing, err := os.ReadFile(dest); err == nil {
		if bytes.Equal(existing, data) {
			fmt.Printf("Up to date: %s\n", dest)
			return nil
		}
		if !installForce {
			fmt.Printf("Skipping %s (exists with different content, use --force to overwrite)\n", dest)
			return nil
		}
	}

	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return fmt.Errorf("creating directory: %w", err)
	}
	if err := os.WriteFile(dest, data, 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", dest, err)
	}
	fmt.Printf("Installed %s\n", dest)
	return nil
}

var checkCmd = &cobra.Command{
	Use:   "check",
	Short: "Check installed integrations for missing or stale files",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		reg := integrations()
		names := make([]string, 0, len(reg))
		for name := range reg {
			names = append(names, name)
		}
		sort.Strings(names)

		stale := 0
		for _, name := range names {
			// Prompt templates are meant to be customized after copying,
			// so differing content is not staleness.
			if name == "prompts" {
				continue
			}
			for _, f := range reg[name] {
				for _, global := range []bool{false, true} {
					dest, err := integrationDest(f, global)
					if err != nil {
						continue
					}
					existing, err := os.ReadFile(dest)
					if err != nil {
						continue // not installed here — not an error
					}
					data, err := f.content()
					if err != nil {
						continue
					}
					if bytes.Equal(existing, data) {
						fmt.Printf("ok:    %s\n", dest)
					} else {
						fmt.Printf("stale: %s (reinstall with `critty install --force %s`)\n", dest, name)
						stale++
					}
				}
			}
		}
		if stale > 0 {
			return fmt.Errorf("%d stale integration file(s)", stale)
		}
		fmt.Println("All installed integrations are up to date.")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(installCmd)
	rootCmd.AddCommand(checkCmd)
	installCmd.Flags().BoolVarP(&installForce, "force", "f", false, "overwrite files that differ")
}
