package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func integrationContent(t *testing.T, target, dest string) string {
	t.Helper()
	for _, f := range integrations()[target] {
		if f.dest == dest {
			data, err := f.content()
			if err != nil {
				t.Fatalf("%s %s: %v", target, dest, err)
			}
			return string(data)
		}
	}
	t.Fatalf("%s has no integration file %s", target, dest)
	return ""
}

func destinations(t *testing.T, target string) []string {
	t.Helper()
	var dests []string
	for _, f := range integrations()[target] {
		if target != "opencode" && f.dest != f.globalDest {
			t.Errorf("%s %s: global destination %q differs", target, f.dest, f.globalDest)
		}
		dests = append(dests, f.dest)
	}
	return dests
}

func TestOpenCodeGlobalFilesUseXDGConfigDir(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(t.TempDir(), "xdg"))
	want := map[string]string{
		filepath.Join(".opencode", "skills", "critty-approval", "SKILL.md"): filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "opencode", "skills", "critty-approval", "SKILL.md"),
		filepath.Join(".opencode", "commands", "critty.md"):                 filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "opencode", "commands", "critty.md"),
		filepath.Join(".opencode", "skills", "critty-cli", "SKILL.md"):      filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "opencode", "skills", "critty-cli", "SKILL.md"),
	}
	for _, rel := range []string{
		"crit-ai-review/SKILL.md", "crit-ai-cowork/SKILL.md",
		"crit-ai-review/references/restricted-cli.md", "crit-ai-review/scripts/claude_review_gate.py",
		"crit-ai-review/references/codex-cli.md", "crit-ai-review/scripts/codex_review_server.py",
		"crit-ai-review/scripts/codex-review-requirements.txt",
	} {
		want[filepath.Join(".opencode", "skills", filepath.FromSlash(rel))] = filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "opencode", "skills", filepath.FromSlash(rel))
	}
	for _, f := range integrations()["opencode"] {
		if f.globalDest != want[f.dest] {
			t.Errorf("%s global destination = %q, want %q", f.dest, f.globalDest, want[f.dest])
		}
		global, err := integrationDest(f, true)
		if err != nil || global != want[f.dest] {
			t.Errorf("integrationDest(%s, global) = %q, %v", f.dest, global, err)
		}
	}
}

func TestIntegrationLayouts(t *testing.T) {
	want := map[string][]string{
		"claude-code": {
			filepath.Join(".claude", "skills", "critty", "SKILL.md"),
			filepath.Join(".claude", "skills", "critty-cli", "SKILL.md"),
			filepath.Join(".claude", "skills", "critty-approval", "SKILL.md"),
		},
		"codex": {
			filepath.Join(".agents", "skills", "critty", "SKILL.md"),
			filepath.Join(".agents", "skills", "critty-cli", "SKILL.md"),
			filepath.Join(".agents", "skills", "critty-approval", "SKILL.md"),
		},
		"opencode": {
			filepath.Join(".opencode", "commands", "critty.md"),
			filepath.Join(".opencode", "skills", "critty-cli", "SKILL.md"),
			filepath.Join(".opencode", "skills", "critty-approval", "SKILL.md"),
		},
		"gemini": {
			filepath.Join(".gemini", "skills", "critty", "SKILL.md"),
			filepath.Join(".gemini", "skills", "critty-cli", "SKILL.md"),
			filepath.Join(".gemini", "skills", "critty-approval", "SKILL.md"),
		},
	}
	for target, dests := range want {
		dir := map[string]string{"claude-code": ".claude", "codex": ".agents", "opencode": ".opencode", "gemini": ".gemini"}[target]
		for _, rel := range []string{
			"crit-ai-review/SKILL.md", "crit-ai-cowork/SKILL.md",
			"crit-ai-review/references/restricted-cli.md", "crit-ai-review/scripts/claude_review_gate.py",
			"crit-ai-review/references/codex-cli.md", "crit-ai-review/scripts/codex_review_server.py",
			"crit-ai-review/scripts/codex-review-requirements.txt",
		} {
			dests = append(dests, filepath.Join(dir, "skills", filepath.FromSlash(rel)))
		}
		if got := destinations(t, target); !slices.Equal(got, dests) {
			t.Errorf("%s destinations = %q, want %q", target, got, dests)
		}
	}
}

func TestAllIncludesEveryAgent(t *testing.T) {
	names, err := integrationTargets("all", integrations())
	if err != nil {
		t.Fatal(err)
	}
	for _, agent := range []string{"claude-code", "codex", "opencode", "gemini"} {
		if !slices.Contains(names, agent) {
			t.Errorf("all target %q lacks %s", names, agent)
		}
	}
	if slices.Contains(names, "prompts") {
		t.Errorf("all target %q must not install prompt templates", names)
	}
}

func TestClaudeSkillsAreVerbatim(t *testing.T) {
	for _, name := range skillNames {
		embedded, err := skillContent.ReadFile("skill/" + name + "/SKILL.md")
		if err != nil {
			t.Fatal(err)
		}
		dest := filepath.Join(".claude", "skills", name, "SKILL.md")
		if got := integrationContent(t, "claude-code", dest); got != string(embedded) {
			t.Errorf("%s is rewritten for Claude Code", dest)
		}
	}
}

func TestCodexSkillsUseCodexConventions(t *testing.T) {
	skill := integrationContent(t, "codex", filepath.Join(".agents", "skills", "critty", "SKILL.md"))
	cli := integrationContent(t, "codex", filepath.Join(".agents", "skills", "critty-cli", "SKILL.md"))

	for _, unwanted := range []string{"`/critty`", "`/critty-cli`", "/critty ", "Claude Code", "$ARGUMENTS", "allowed-tools:", "argument-hint:", "user-invocable:"} {
		for _, content := range []string{skill, cli} {
			if strings.Contains(content, unwanted) {
				t.Errorf("Codex skill still contains %q", unwanted)
			}
		}
	}
	for _, wanted := range []string{"`$critty-cli`", "'Codex'", "critty <arguments>"} {
		if !strings.Contains(skill, wanted) {
			t.Errorf("Codex critty skill lacks %q", wanted)
		}
	}
	if !strings.Contains(skill, "critty --staged") {
		t.Error("Codex critty skill does not document staged-only review")
	}
	if !strings.Contains(cli, "`$critty`") {
		t.Error("Codex critty-cli skill does not point at $critty")
	}
	if !strings.HasPrefix(cli, "---\nname: critty-cli\ndescription: ") {
		t.Errorf("Codex critty-cli frontmatter is malformed:\n%s", cli[:min(len(cli), 200)])
	}
}

func TestOpenCodeCommandKeepsArgumentsAndDropsClaudeKeys(t *testing.T) {
	command := integrationContent(t, "opencode", filepath.Join(".opencode", "commands", "critty.md"))
	cli := integrationContent(t, "opencode", filepath.Join(".opencode", "skills", "critty-cli", "SKILL.md"))

	if !strings.Contains(command, "critty $ARGUMENTS") {
		t.Error("OpenCode command lost $ARGUMENTS")
	}
	if !strings.Contains(command, "critty --staged") {
		t.Error("OpenCode command does not document staged-only review")
	}
	if !strings.HasPrefix(command, "---\ndescription: ") {
		t.Errorf("OpenCode command frontmatter should start with description:\n%s", command[:min(len(command), 120)])
	}
	if !strings.Contains(command, "`critty-cli`") || strings.Contains(command, "`/critty-cli`") {
		t.Error("OpenCode command must reference the critty-cli skill by bare name")
	}
	if !strings.Contains(cli, "`/critty`") {
		t.Error("OpenCode critty-cli must point at the /critty command")
	}
	for _, content := range []string{command, cli} {
		for _, unwanted := range []string{"Claude Code", "allowed-tools:", "argument-hint:", "user-invocable:"} {
			if strings.Contains(content, unwanted) {
				t.Errorf("OpenCode file still contains %q", unwanted)
			}
		}
		if !strings.Contains(content, "'OpenCode'") {
			t.Error("OpenCode file is not attributed to OpenCode")
		}
	}
}

func TestGeminiIntegration(t *testing.T) {
	skill := integrationContent(t, "gemini", filepath.Join(".gemini", "skills", "critty", "SKILL.md"))
	cli := integrationContent(t, "gemini", filepath.Join(".gemini", "skills", "critty-cli", "SKILL.md"))

	if !strings.HasPrefix(skill, "---\nname: critty\n") {
		t.Error("Gemini skill frontmatter is malformed")
	}
	for _, content := range []string{skill, cli} {
		for _, unwanted := range []string{"Claude Code", "$ARGUMENTS", "allowed-tools:", "argument-hint:", "user-invocable:", "kind: local", "subagent"} {
			if strings.Contains(content, unwanted) {
				t.Errorf("Gemini skill still contains %q", unwanted)
			}
		}
		if !strings.Contains(content, "'Gemini'") {
			t.Error("Gemini skill is not attributed to Gemini")
		}
	}
	if !strings.Contains(skill, "critty <arguments>") || strings.Contains(skill, "`/critty`") {
		t.Error("Gemini skill invocation is not rewritten")
	}
}

func TestDropFrontmatterKeys(t *testing.T) {
	in := []byte("---\nname: x\nallowed-tools: Bash(x *)\ndescription: d\n---\n\nbody: allowed-tools: keep\n")
	got := dropFrontmatterKeys(in, []string{"allowed-tools"})
	want := []byte("---\nname: x\ndescription: d\n---\n\nbody: allowed-tools: keep\n")
	if !bytes.Equal(got, want) {
		t.Errorf("dropFrontmatterKeys = %q, want %q", got, want)
	}
	if got := dropFrontmatterKeys([]byte("no frontmatter\n"), []string{"name"}); string(got) != "no frontmatter\n" {
		t.Errorf("content without frontmatter was altered: %q", got)
	}
}

func TestApprovalSkillReferencesUseAgentConventions(t *testing.T) {
	for agent, variant := range skillVariants {
		t.Run(agent, func(t *testing.T) {
			data, err := renderSkill("critty-approval", agent)
			if err != nil {
				t.Fatal(err)
			}
			for _, name := range []string{"critty", "critty-cli"} {
				if !bytes.Contains(data, []byte("`"+variant.skillRef(name)+"`")) {
					t.Errorf("missing %s reference for %s", name, agent)
				}
			}
			for _, name := range []string{"critty", "critty-cli"} {
				caller, err := renderSkill(name, agent)
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Contains(caller, []byte("`"+variant.skillRef("critty-approval")+"`")) {
					t.Errorf("%s cannot discover the approval workflow", name)
				}
			}
		})
	}
}

func TestAIReviewSkillResources(t *testing.T) {
	for agent, variant := range skillVariants {
		t.Run(agent, func(t *testing.T) {
			for _, name := range []string{"crit-ai-review", "crit-ai-cowork"} {
				data, err := renderSkill(name, agent)
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Contains(data, []byte("`"+variant.skillRef("crit-ai-cowork")+"` with `Astra`")) {
					t.Errorf("%s has the wrong invocation example for %s", name, agent)
				}
			}
			for _, f := range integrations()[agent] {
				if !strings.Contains(filepath.ToSlash(f.dest), "/crit-ai-review/") {
					continue
				}
				data, err := f.content()
				if err != nil {
					t.Fatal(err)
				}
				dest := filepath.Join(t.TempDir(), f.dest)
				f.dest = dest
				if err := installIntegrationFile(f, false); err != nil {
					t.Fatal(err)
				}
				got, err := os.ReadFile(dest)
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(got, data) {
					t.Errorf("installed asset %s differs", dest)
				}
				if filepath.Base(dest) != "SKILL.md" {
					_, resource, _ := strings.Cut(filepath.ToSlash(dest), "/crit-ai-review/")
					embedded, err := skillContent.ReadFile("skill/crit-ai-review/" + resource)
					if err != nil {
						t.Fatal(err)
					}
					if !bytes.Equal(got, embedded) {
						t.Error("review resource changed during rendering")
					}
				}
			}
		})
	}
}
