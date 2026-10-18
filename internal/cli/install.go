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

	"github.com/crittylabs/critty/internal/prompt"
	"github.com/crittylabs/critty/internal/xdg"
)

//go:embed skill/critty/SKILL.md skill/critty-cli/SKILL.md skill/critty-approval/SKILL.md skill/critty-ai-review/SKILL.md skill/critty-ai-review/references/restricted-cli.md skill/critty-ai-review/scripts/claude_review_gate.py skill/critty-ai-cowork/SKILL.md
//go:embed skill/critty-ai-review/references/codex-cli.md skill/critty-ai-review/scripts/codex_review_server.py skill/critty-ai-review/scripts/codex-review-requirements.txt
//go:embed skill/critty-cli/references/configuration.md
var skillContent embed.FS

var installForce bool
var installUser bool

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
	"antigravity": {
		author:         "Antigravity CLI",
		skillRef:       func(name string) string { return "/" + name },
		argumentsToken: "<arguments>",
		dropKeys:       []string{"allowed-tools", "argument-hint", "user-invocable"},
	},
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
	"agents": {
		author:         "<agent-name>",
		skillRef:       func(name string) string { return name },
		argumentsToken: "<arguments>",
		dropKeys:       []string{"allowed-tools", "argument-hint", "user-invocable"},
	},
}

var skillNames = []string{"critty", "critty-cli", "critty-approval", "critty-ai-review", "critty-ai-cowork"}

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
	if agent == "agents" {
		data = bytes.Replace(data, []byte("\n---\n"), []byte("\n---\n\nReplace `<agent-name>` in command examples with your actual agent name.  Load referenced skills by name using your host's skill mechanism.\n"), 1)
	}

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

func skillResource(name, resource, dir string) integrationFile {
	rel := filepath.Join(dir, "skills", name, filepath.FromSlash(resource))
	return integrationFile{
		content: func() ([]byte, error) {
			return skillContent.ReadFile("skill/" + name + "/" + resource)
		},
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
		skillFile("critty-approval", "claude-code", ".claude"),
	}

	m["codex"] = []integrationFile{
		skillFile("critty", "codex", ".codex"),
		skillFile("critty-cli", "codex", ".codex"),
		skillFile("critty-approval", "codex", ".codex"),
	}
	m["agents"] = []integrationFile{
		skillFile("critty", "agents", ".agents"),
		skillFile("critty-cli", "agents", ".agents"),
		skillFile("critty-approval", "agents", ".agents"),
	}

	// OpenCode reads project files from .opencode/ but global ones from its
	// XDG config directory (~/.config/opencode by default).
	openCodeGlobal := xdg.AppConfigHome("opencode")
	openCodeCLI := skillFile("critty-cli", "opencode", ".opencode")
	openCodeCLI.globalDest = filepath.Join(openCodeGlobal, "skills", "critty-cli", "SKILL.md")
	openCodeApproval := skillFile("critty-approval", "opencode", ".opencode")
	openCodeApproval.globalDest = filepath.Join(openCodeGlobal, "skills", "critty-approval", "SKILL.md")
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
		openCodeApproval,
	}

	m["gemini"] = []integrationFile{
		skillFile("critty", "gemini", ".gemini"),
		skillFile("critty-cli", "gemini", ".gemini"),
		skillFile("critty-approval", "gemini", ".gemini"),
	}

	m["antigravity"] = []integrationFile{
		skillFile("critty", "antigravity", ".agents"),
		skillFile("critty-cli", "antigravity", ".agents"),
		skillFile("critty-approval", "antigravity", ".agents"),
	}

	for agent, dir := range map[string]string{
		"claude-code": ".claude", "codex": ".codex", "opencode": ".opencode", "gemini": ".gemini", "agents": ".agents", "antigravity": ".agents",
	} {
		files := []integrationFile{
			skillFile("critty-ai-review", agent, dir),
			skillFile("critty-ai-cowork", agent, dir),
			skillResource("critty-ai-review", "references/restricted-cli.md", dir),
			skillResource("critty-ai-review", "scripts/claude_review_gate.py", dir),
			skillResource("critty-ai-review", "references/codex-cli.md", dir),
			skillResource("critty-ai-review", "scripts/codex_review_server.py", dir),
			skillResource("critty-ai-review", "scripts/codex-review-requirements.txt", dir),
			skillResource("critty-cli", "references/configuration.md", dir),
		}
		if agent == "opencode" {
			for i := range files {
				rel := strings.TrimPrefix(files[i].dest, dir+string(filepath.Separator))
				files[i].globalDest = filepath.Join(openCodeGlobal, rel)
			}
		}
		m[agent] = append(m[agent], files...)
	}

	for i := range m["antigravity"] {
		f := &m["antigravity"][i]
		rel := strings.TrimPrefix(f.dest, ".agents"+string(filepath.Separator))
		f.globalDest = filepath.Join(".gemini", "antigravity-cli", rel)
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
			dest:       filepath.Join(".critty", "prompts", name),
			globalDest: filepath.Join(xdg.ConfigHome(), "prompts", name), // absolute
		})
	}
	m["prompts"] = prompts

	return m
}

var agentTargets = []string{"claude-code", "codex", "opencode", "antigravity"}

func integrationTargets(target string, reg map[string][]integrationFile) ([]string, error) {
	if target == "agy" {
		target = "antigravity"
	}
	if target == "all" {
		return agentTargets, nil
	}
	if _, ok := reg[target]; ok {
		return []string{target}, nil
	}

	available := make([]string, 0, len(reg)+2)
	available = append(available, "agy")
	for name := range reg {
		available = append(available, name)
	}
	sort.Strings(available)
	return nil, fmt.Errorf("unknown target %q (available: %s, all)", target, strings.Join(available, ", "))
}

var installCmd = &cobra.Command{
	Use:   "install [--user] [--force] <claude-code|codex|opencode|antigravity|agy|gemini|agents|prompts|all>",
	Short: "Install agent integrations or stock prompt templates",
	Long: `Install agent integrations (skills) or the stock finish prompt
templates.

Use --user or run from your home directory to install for your user.
Otherwise, run from a repository root to install for that project only.

Targets:
  claude-code  Claude Code skills
  codex        Codex skills
  opencode     OpenCode command (/critty) and skills
  antigravity  Antigravity CLI (agy) skills
  agy          Alias for antigravity
  gemini       Legacy Gemini CLI skills (explicit installation only)
  agents       Shared, agent-neutral skills under .agents/skills
  prompts      Stock finish prompt templates (customize after copying)
  all          claude-code + codex + opencode + antigravity

Agent targets include critty-cli, critty-approval, critty-ai-review, and
critty-ai-cowork, with their reference files and review gate. The interactive
critty workflow is a skill except in OpenCode, where it is a command.

Codex uses .codex/skills in either scope. OpenCode uses .opencode in
projects and its XDG config directory for users. Antigravity uses
.agents/skills in projects and ~/.gemini/antigravity-cli/skills for users.
The agents target uses .agents/skills in either scope. Prefer a shared
installation or selected host variants to avoid duplicate skill names.
Existing files in old locations are left untouched.

After installing replacements, recognized legacy crit-ai-review and
crit-ai-cowork skill folders and their resources are removed once all
integration files are up to date.`,
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
		for _, name := range names {
			if name == "prompts" {
				continue
			}
			current, err := integrationFilesCurrent(reg[name], global)
			if err != nil {
				return err
			}
			if !current {
				fmt.Printf("Skipping legacy cleanup for %s (integration files differ, reinstall with --force)\n", name)
				continue
			}
			for _, f := range reg[name] {
				if err := removeLegacyAISkill(f, global); err != nil {
					return err
				}
			}
		}
		return nil
	},
}

// isGlobalInstall selects user scope explicitly or when run from $HOME.
func isGlobalInstall() (bool, error) {
	if installUser {
		return true, nil
	}
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

func integrationFilesCurrent(files []integrationFile, global bool) (bool, error) {
	for _, f := range files {
		dest, err := integrationDest(f, global)
		if err != nil {
			return false, err
		}
		installed, err := os.ReadFile(dest)
		if err != nil {
			return false, err
		}
		want, err := f.content()
		if err != nil {
			return false, err
		}
		if !bytes.Equal(installed, want) {
			return false, nil
		}
	}
	return true, nil
}

func removeLegacyAISkill(f integrationFile, global bool) error {
	if filepath.Base(f.dest) != "SKILL.md" {
		return nil
	}
	name := filepath.Base(filepath.Dir(f.dest))
	var marker string
	switch name {
	case "critty-ai-review":
		marker = "# AI Review with Critty\n"
	case "critty-ai-cowork":
		marker = "Read [crit-ai-review](../crit-ai-review/SKILL.md)"
	default:
		return nil
	}
	dest, err := integrationDest(f, global)
	if err != nil {
		return err
	}
	legacyName := strings.Replace(name, "critty-ai-", "crit-ai-", 1)
	legacyDir := filepath.Join(filepath.Dir(filepath.Dir(dest)), legacyName)
	legacyFile := filepath.Join(legacyDir, "SKILL.md")
	legacy, err := os.ReadFile(legacyFile)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if !bytes.Contains(legacy, []byte("\nname: "+legacyName+"\n")) || !bytes.Contains(legacy, []byte(marker)) {
		return nil
	}
	// RemoveAll removes symlinks themselves without traversing their targets.
	if err := os.RemoveAll(legacyDir); err != nil {
		return fmt.Errorf("removing %s: %w", legacyDir, err)
	}
	fmt.Printf("Removed legacy skill: %s\n", legacyDir)
	return nil
}

func init() {
	rootCmd.AddCommand(installCmd)
	installCmd.Flags().BoolVarP(&installForce, "force", "f", false, "overwrite files that differ")
	installCmd.Flags().BoolVar(&installUser, "user", false, "install for the current user from any directory")
}
