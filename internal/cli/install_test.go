package cli

import (
	"bytes"
	"fmt"
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
		if target != "opencode" && target != "antigravity" && f.dest != f.globalDest {
			t.Errorf("%s %s: global destination %q differs", target, f.dest, f.globalDest)
		}
		dests = append(dests, f.dest)
	}
	return dests
}

// aiReviewResources lists the AI review skill files every agent target installs.
var aiReviewResources = []string{
	"critty-ai-review/SKILL.md", "critty-ai-cowork/SKILL.md",
	"critty-ai-review/references/restricted-cli.md", "critty-ai-review/scripts/claude_review_gate.py",
	"critty-ai-review/references/codex-cli.md", "critty-ai-review/scripts/codex_review_server.py",
	"critty-ai-review/scripts/codex-review-requirements.txt",
}

func TestOpenCodeGlobalFilesUseXDGConfigDir(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(t.TempDir(), "xdg"))
	want := map[string]string{
		filepath.Join(".opencode", "skills", "critty-approval", "SKILL.md"): filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "opencode", "skills", "critty-approval", "SKILL.md"),
		filepath.Join(".opencode", "commands", "critty.md"):                 filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "opencode", "commands", "critty.md"),
		filepath.Join(".opencode", "skills", "critty-cli", "SKILL.md"):      filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "opencode", "skills", "critty-cli", "SKILL.md"),
	}
	for _, rel := range aiReviewResources {
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
			filepath.Join(".codex", "skills", "critty", "SKILL.md"),
			filepath.Join(".codex", "skills", "critty-cli", "SKILL.md"),
			filepath.Join(".codex", "skills", "critty-approval", "SKILL.md"),
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
		"agents": {
			filepath.Join(".agents", "skills", "critty", "SKILL.md"),
			filepath.Join(".agents", "skills", "critty-cli", "SKILL.md"),
			filepath.Join(".agents", "skills", "critty-approval", "SKILL.md"),
		},
	}
	for target, dests := range want {
		dir := map[string]string{"claude-code": ".claude", "codex": ".codex", "opencode": ".opencode", "gemini": ".gemini", "agents": ".agents"}[target]
		for _, rel := range aiReviewResources {
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
	for _, agent := range []string{"claude-code", "codex", "opencode", "antigravity"} {
		if !slices.Contains(names, agent) {
			t.Errorf("all target %q lacks %s", names, agent)
		}
	}
	if slices.Contains(names, "prompts") {
		t.Errorf("all target %q must not install prompt templates", names)
	}
	if slices.Contains(names, "gemini") {
		t.Errorf("all target %q must not install legacy Gemini skills", names)
	}
	legacy, err := integrationTargets("gemini", integrations())
	if err != nil || !slices.Equal(legacy, []string{"gemini"}) {
		t.Fatalf("explicit Gemini target = %v, %v", legacy, err)
	}
}

func TestPromptInstallUsesCrittyPaths(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)
	for _, file := range integrations()["prompts"] {
		name := filepath.Base(file.dest)
		if file.dest != filepath.Join(".critty", "prompts", name) {
			t.Errorf("project prompt destination = %q", file.dest)
		}
		if file.globalDest != filepath.Join(configHome, "critty", "prompts", name) {
			t.Errorf("global prompt destination = %q", file.globalDest)
		}
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
	skill := integrationContent(t, "codex", filepath.Join(".codex", "skills", "critty", "SKILL.md"))
	cli := integrationContent(t, "codex", filepath.Join(".codex", "skills", "critty-cli", "SKILL.md"))

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
			for _, name := range []string{"critty-ai-review", "critty-ai-cowork"} {
				data, err := renderSkill(name, agent)
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Contains(data, []byte("`"+variant.skillRef("critty-ai-cowork")+"` with `Astra`")) {
					t.Errorf("%s has the wrong invocation example for %s", name, agent)
				}
			}
			for _, f := range integrations()[agent] {
				if !strings.Contains(filepath.ToSlash(f.dest), "/critty-ai-review/") {
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
					_, resource, _ := strings.Cut(filepath.ToSlash(dest), "/critty-ai-review/")
					embedded, err := skillContent.ReadFile("skill/critty-ai-review/" + resource)
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

func seedLegacyAISkill(t *testing.T, f integrationFile, global bool) string {
	t.Helper()
	dest, err := integrationDest(f, global)
	if err != nil {
		t.Fatal(err)
	}
	name := filepath.Base(filepath.Dir(dest))
	legacy := strings.Replace(name, "critty-ai-", "crit-ai-", 1)
	dir := filepath.Join(filepath.Dir(filepath.Dir(dest)), legacy)
	data, err := f.content()
	if err != nil {
		t.Fatal(err)
	}
	data = bytes.ReplaceAll(data, []byte("critty-ai-"), []byte("crit-ai-"))
	data = append(data, []byte("\nCustomized installation.\n")...)
	if err := os.MkdirAll(filepath.Join(dir, "scripts"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "scripts", "review.py"), []byte("legacy resource\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestInstallRetiresLegacyAISkills(t *testing.T) {
	for _, agent := range agentTargets {
		for _, global := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/global=%t", agent, global), func(t *testing.T) {
				home := t.TempDir()
				t.Setenv("HOME", home)
				t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
				if global {
					t.Chdir(home)
				} else {
					t.Chdir(t.TempDir())
				}
				var legacyDirs []string
				for _, f := range integrations()[agent] {
					name := filepath.Base(filepath.Dir(f.dest))
					if filepath.Base(f.dest) == "SKILL.md" && (name == "critty-ai-review" || name == "critty-ai-cowork") {
						legacyDirs = append(legacyDirs, seedLegacyAISkill(t, f, global))
					}
				}
				if err := installCmd.RunE(installCmd, []string{agent}); err != nil {
					t.Fatal(err)
				}
				for _, dir := range legacyDirs {
					if _, err := os.Stat(dir); !os.IsNotExist(err) {
						t.Errorf("legacy skill still exists: %s (%v)", dir, err)
					}
				}
				for _, f := range integrations()[agent] {
					dest, err := integrationDest(f, global)
					if err != nil {
						t.Fatal(err)
					}
					got, err := os.ReadFile(dest)
					if err != nil {
						t.Fatal(err)
					}
					want, err := f.content()
					if err != nil || !bytes.Equal(got, want) {
						t.Errorf("replacement differs: %s (%v)", dest, err)
					}
				}
			})
		}
	}
}

func TestInstallPreservesUnsafeOrUnreplacedLegacySkills(t *testing.T) {
	for _, scenario := range []string{"unrecognized", "wrong name", "different replacement", "stale caller", "failed install"} {
		t.Run(scenario, func(t *testing.T) {
			t.Chdir(t.TempDir())
			previousForce := installForce
			installForce = false
			t.Cleanup(func() { installForce = previousForce })
			f := skillFile("critty-ai-review", "claude-code", ".claude")
			dir := seedLegacyAISkill(t, f, false)
			file := filepath.Join(dir, "SKILL.md")
			switch scenario {
			case "unrecognized", "wrong name":
				data := "---\nname: crit-ai-review\n---\nAn unrelated skill mentioning critty.\n"
				if scenario == "wrong name" {
					data = "---\nname: unrelated\n---\n# AI Review with Critty\n"
				}
				if err := os.WriteFile(file, []byte(data), 0o644); err != nil {
					t.Fatal(err)
				}
			case "different replacement":
				if err := os.MkdirAll(filepath.Dir(f.dest), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(f.dest, []byte("custom replacement\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			case "stale caller":
				caller := skillFile("critty", "claude-code", ".claude")
				data, err := caller.content()
				if err != nil {
					t.Fatal(err)
				}
				data = bytes.ReplaceAll(data, []byte("critty-ai-"), []byte("crit-ai-"))
				if err := os.MkdirAll(filepath.Dir(caller.dest), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(caller.dest, data, 0o644); err != nil {
					t.Fatal(err)
				}
			case "failed install":
				blocker := filepath.Join(filepath.Dir(f.dest), "scripts", "codex-review-requirements.txt")
				if err := os.MkdirAll(blocker, 0o755); err != nil {
					t.Fatal(err)
				}
			}
			before, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			err = installCmd.RunE(installCmd, []string{"claude-code"})
			if (err != nil) != (scenario == "failed install") {
				t.Fatalf("unexpected install result: %v", err)
			}
			after, err := os.ReadFile(file)
			if err != nil || !bytes.Equal(after, before) {
				t.Fatalf("legacy skill was changed or removed: %v", err)
			}
			if scenario == "stale caller" {
				installForce = true
				if err := installCmd.RunE(installCmd, []string{"claude-code"}); err != nil {
					t.Fatal(err)
				}
				if _, err := os.Stat(dir); !os.IsNotExist(err) {
					t.Fatalf("legacy skill still exists after forced upgrade: %v", err)
				}
			}
		})
	}
}

func TestInstallRetiresLegacyLinksWithoutDeletingTargets(t *testing.T) {
	for _, directory := range []bool{false, true} {
		t.Run(fmt.Sprintf("directory=%t", directory), func(t *testing.T) {
			t.Chdir(t.TempDir())
			f := skillFile("critty-ai-review", "claude-code", ".claude")
			dir := seedLegacyAISkill(t, f, false)
			file := filepath.Join(dir, "SKILL.md")
			before, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			source := file
			if directory {
				source = dir
			}
			external := filepath.Join(t.TempDir(), "original")
			if err := os.Rename(source, external); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(external, source); err != nil {
				t.Fatal(err)
			}
			if err := installCmd.RunE(installCmd, []string{"claude-code"}); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Lstat(dir); !os.IsNotExist(err) {
				t.Errorf("legacy path still exists: %v", err)
			}
			if directory {
				external = filepath.Join(external, "SKILL.md")
			}
			after, err := os.ReadFile(external)
			if err != nil || !bytes.Equal(after, before) {
				t.Fatalf("symlink target was changed or removed: %v", err)
			}
		})
	}
}

func TestAntigravityIntegration(t *testing.T) {
	t.Chdir(t.TempDir())
	reg := integrations()
	names, err := integrationTargets("agy", reg)
	if err != nil || !slices.Equal(names, []string{"antigravity"}) {
		t.Fatalf("agy targets = %v, %v", names, err)
	}
	if len(reg["antigravity"]) != len(reg["codex"]) {
		t.Fatal("Antigravity lacks bundled resources")
	}
	for _, agent := range []string{"codex", "antigravity"} {
		for _, f := range reg[agent] {
			if err := installIntegrationFile(f, false); err != nil {
				t.Fatal(err)
			}
		}
	}
	for i, f := range reg["antigravity"] {
		original := reg["codex"][i]
		rel, err := filepath.Rel(filepath.Join(".codex", "skills"), original.dest)
		if err != nil {
			t.Fatal(err)
		}
		parts := strings.SplitN(rel, string(filepath.Separator), 2)
		wantRel := filepath.Join(parts[0], parts[1])
		if f.dest != filepath.Join(".agents", "skills", wantRel) || f.globalDest != filepath.Join(".gemini", "antigravity-cli", "skills", wantRel) {
			t.Errorf("wrong destinations: %s, %s", f.dest, f.globalDest)
		}
		got, err := os.ReadFile(original.dest)
		if err != nil {
			t.Fatal(err)
		}
		want, err := original.content()
		if err != nil || !bytes.Equal(got, want) {
			t.Fatalf("Codex content changed: %s", original.dest)
		}
		data, err := os.ReadFile(f.dest)
		if err != nil {
			t.Fatal(err)
		}
		if parts[1] != "SKILL.md" {
			if !bytes.Equal(data, want) {
				t.Errorf("resource changed: %s", f.dest)
			}
			continue
		}
		content := string(data)
		if !strings.Contains(content, "\nname: "+parts[0]+"\n") {
			t.Errorf("wrong identity: %s", f.dest)
		}
		for _, unwanted := range []string{"$ARGUMENTS", "allowed-tools:", "argument-hint:", "user-invocable:", "'Claude Code'"} {
			if strings.Contains(content, unwanted) {
				t.Errorf("%s contains %q", f.dest, unwanted)
			}
		}
		for _, skill := range skillNames {
			if strings.Contains(content, "`$"+skill+"`") || strings.Contains(content, skill+"-antigravity") {
				t.Errorf("unconverted skill reference in %s", f.dest)
			}
		}
	}
}

func TestUserInstallScope(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	t.Setenv("HOME", home)
	previous := installUser
	t.Cleanup(func() { installUser = previous })
	for _, tc := range []struct {
		name, cwd  string
		user, want bool
	}{
		{"home", home, false, true},
		{"project", project, false, false},
		{"explicit-user", project, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Chdir(tc.cwd)
			installUser = tc.user
			got, err := isGlobalInstall()
			if err != nil || got != tc.want {
				t.Fatalf("global = %v, %v; want %v", got, err, tc.want)
			}
		})
	}
	t.Chdir(project)
	installUser = true
	if err := installCmd.RunE(installCmd, []string{"agents"}); err != nil {
		t.Fatal(err)
	}
	for _, name := range skillNames {
		global := filepath.Join(home, ".agents", "skills", name, "SKILL.md")
		data, err := os.ReadFile(global)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(data), "`$critty") || strings.Contains(string(data), "`/critty") {
			t.Errorf("shared skill uses host-specific references: %s", name)
		}
		if _, err := os.Stat(filepath.Join(project, ".agents")); !os.IsNotExist(err) {
			t.Fatalf("user install wrote project files: %v", err)
		}
	}
}

func TestUserInstallTargetPaths(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	t.Chdir(project)
	previous := installUser
	t.Cleanup(func() { installUser = previous })
	if err := installCmd.ParseFlags([]string{"--user"}); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ target, path string }{
		{"claude-code", ".claude/skills/critty/SKILL.md"},
		{"codex", ".codex/skills/critty/SKILL.md"},
		{"opencode", "config/opencode/commands/critty.md"},
		{"agy", ".gemini/antigravity-cli/skills/critty/SKILL.md"},
		{"agents", ".agents/skills/critty/SKILL.md"},
	} {
		t.Run(tc.target, func(t *testing.T) {
			if err := installCmd.RunE(installCmd, []string{tc.target}); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(filepath.Join(home, filepath.FromSlash(tc.path))); err != nil {
				t.Fatal(err)
			}
		})
	}
	entries, err := os.ReadDir(project)
	if err != nil || len(entries) != 0 {
		t.Fatalf("user install changed project: %v, %v", entries, err)
	}
}
