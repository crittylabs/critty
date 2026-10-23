package cli

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/crittylabs/critty/internal/config"
	"github.com/crittylabs/critty/internal/git"
	"github.com/crittylabs/critty/internal/review"
)

func TestReviewProjectLinkedWorktree(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	root := filepath.Join(t.TempDir(), "project")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_SYSTEM", os.DevNull)
	worktree := filepath.Join(t.TempDir(), "different-name")
	sibling := filepath.Join(filepath.Dir(root), "project@topic")
	subdir := filepath.Join(root, "subdirectory")
	if err := os.Mkdir(subdir, 0o755); err != nil {
		t.Fatal(err)
	}
	directories := []string{root, worktree, sibling, subdir}
	for _, args := range [][]string{
		{"init", "-q"}, {"config", "user.name", "Test"}, {"config", "user.email", "test@example.com"},
		{"commit", "--allow-empty", "-qm", "base"}, {"worktree", "add", "--detach", worktree},
		{"worktree", "add", "--detach", sibling},
	} {
		if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			t.Fatalf("%s: %v", out, err)
		}
	}
	for _, dir := range directories {
		if got := reviewProject(dir); got != "project" {
			t.Errorf("reviewProject(%q) = %q, want project", dir, got)
		}
	}
	var saved []*review.Session
	for _, dir := range directories {
		if err := os.Chdir(dir); err != nil {
			t.Fatal(err)
		}
		sess, err := openReviewSession(&config.Config{}, &reviewMode{docPath: "a.md"})
		if err != nil {
			t.Fatal(err)
		}
		if sess.Meta.GitCommonDir == "" {
			t.Fatal("new review did not save its Git project identity")
		}
		saved = append(saved, sess)
	}
	// Old registry entries retain discovered identities without changing review timestamps.
	for _, sess := range saved[1:] {
		sess.Meta.GitCommonDir = ""
		data, err := json.Marshal(sess.Meta)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(review.SessionsDir(), sess.Key+".json"), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	other := filepath.Join(root, "independent", "project")
	if err := os.MkdirAll(other, 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("git", "init", "-q")
	cmd.Dir = other
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%s: %v", out, err)
	}
	if err := os.Chdir(other); err != nil {
		t.Fatal(err)
	}
	independent, err := openReviewSession(&config.Config{}, &reviewMode{docPath: "other.md"})
	if err != nil {
		t.Fatal(err)
	}
	independent.Meta.GitCommonDir = ""
	data, err := json.Marshal(independent.Meta)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(review.SessionsDir(), independent.Key+".json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, dir := range directories {
		items, err := collectProjectSessions(dir, false)
		if err != nil || len(items) != len(saved) {
			t.Fatalf("project sessions from %q: %d, %v", dir, len(items), err)
		}
		if items[0].ProjectID != items[1].ProjectID || items[0].ProjectName != "project" {
			t.Fatalf("worktrees did not share a project: %+v", items)
		}
		if err := os.Chdir(dir); err != nil {
			t.Fatal(err)
		}
		legacy, err := collectReviewSessions("", false)
		if err != nil || len(legacy) != 1 {
			t.Fatalf("legacy status changed directory scope: %d, %v", len(legacy), err)
		}
	}
	for _, sess := range saved[1:] {
		entry, err := review.ReadSessionEntry(sess.Key)
		if err != nil || entry.GitCommonDir != saved[0].Meta.GitCommonDir || entry.StartedAt != sess.Meta.StartedAt {
			t.Fatalf("listing did not retain the legacy project identity: %+v, %v", entry, err)
		}
	}
	// Git registration identifies legacy sessions even after a worktree disappears.
	// Simulate a legacy entry first seen only after its directory has disappeared.
	legacyGone, err := review.NewSession("", review.SessionEntry{CWD: worktree})
	if err != nil {
		t.Fatal(err)
	}
	if err := legacyGone.Save(); err != nil {
		t.Fatal(err)
	}
	saved = append(saved, legacyGone)
	if err := os.RemoveAll(worktree); err != nil {
		t.Fatal(err)
	}
	if items, err := collectProjectSessions(root, false); err != nil || len(items) != len(saved) {
		t.Fatalf("lost a registered missing worktree: %d, %v", len(items), err)
	}
	all, err := collectProjectSessions(root, true)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range all {
		if item.Key == saved[1].Key && item.ProjectID != "git:"+saved[0].Meta.GitCommonDir {
			t.Fatalf("all-project listing lost a registered missing worktree: %+v", item)
		}
	}
	// Project identity remains available when a recorded worktree is gone.
	gone, err := review.NewSession("", review.SessionEntry{
		CWD: filepath.Join(t.TempDir(), "removed-worktree"), GitCommonDir: saved[0].Meta.GitCommonDir,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := gone.Save(); err != nil {
		t.Fatal(err)
	}
	if items, err := collectProjectSessions(root, false); err != nil || len(items) != len(saved)+1 {
		t.Fatalf("lost a saved project identity: %d, %v", len(items), err)
	}
	outside := filepath.Join(t.TempDir(), "document-project")
	if err := os.Mkdir(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	if got := reviewProject(outside); got != "document-project" {
		t.Errorf("non-Git project = %q", got)
	}
}

func TestReviewTopicLabel(t *testing.T) {
	custom, empty := "Header display", ""
	for _, tc := range []struct {
		name   string
		mode   reviewMode
		branch string
		topic  *string
		want   string
	}{
		{name: "working tree", branch: "feature", want: "feature"},
		{name: "document", branch: "feature", mode: reviewMode{docPath: "a.md"}},
		{name: "patch", branch: "feature", mode: reviewMode{patch: &git.Patch{}}},
		{name: "detached", branch: "HEAD"},
		{name: "custom document", mode: reviewMode{docPath: "a.md"}, topic: &custom, want: custom},
		{name: "hidden", branch: "feature", topic: &empty},
		{name: "range", branch: "feature", mode: reviewMode{source: &git.ReviewSource{Scope: "range", Range: "main...other"}}, want: "other"},
		{name: "implicit HEAD", branch: "feature", mode: reviewMode{source: &git.ReviewSource{Scope: "range", Range: "main.."}}, want: "feature"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sess := &review.Session{Meta: review.SessionEntry{Branch: tc.branch, Topic: tc.topic}}
			if got := reviewTopicLabel(sess, &tc.mode); got != tc.want {
				t.Errorf("topic = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestHeaderLabelsSurviveSessionReload(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	if err := os.WriteFile("a.md", []byte("document\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	mode := &reviewMode{docPath: "a.md"}
	sess, err := openReviewSession(&config.Config{}, mode)
	if err != nil {
		t.Fatal(err)
	}
	for _, topic := range []string{"Header display", ""} {
		sess.Meta.Topic = &topic
		sess.Meta.Project = &topic
		if err := saveReviewMode(sess, mode); err != nil {
			t.Fatal(err)
		}
		loaded, restored, err := loadReviewSession(sess.Key)
		if err != nil {
			t.Fatal(err)
		}
		if loaded.Meta.Topic == nil || reviewTopicLabel(loaded, restored) != topic {
			t.Fatalf("saved topic = %v, want %q", loaded.Meta.Topic, topic)
		}
		if loaded.Meta.Project == nil || reviewProjectLabel("unused", loaded) != topic {
			t.Fatalf("saved project = %v, want %q", loaded.Meta.Project, topic)
		}
		data, err := os.ReadFile(loaded.Path())
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(data), `"topic"`) || strings.Contains(string(data), `"project"`) {
			t.Fatal("header labels changed the CritJSON format")
		}
	}
}

func TestApplyHeaderFlags(t *testing.T) {
	for _, cmd := range []*cobra.Command{rootCmd, reviewCmd, resumeCmd, planCmd} {
		for _, value := range []string{"Display name", ""} {
			t.Run(cmd.Name()+"/"+value, func(t *testing.T) {
				oldProject, oldTopic := reviewProjectName, reviewTopic
				projectFlag, topicFlag := cmd.Flags().Lookup("project"), cmd.Flags().Lookup("topic")
				oldProjectChanged, oldTopicChanged := projectFlag.Changed, topicFlag.Changed
				t.Cleanup(func() {
					reviewProjectName, reviewTopic = oldProject, oldTopic
					projectFlag.Changed, topicFlag.Changed = oldProjectChanged, oldTopicChanged
				})
				project, topic := "Saved project", "Saved topic"
				sess := &review.Session{Meta: review.SessionEntry{Project: &project, Topic: &topic}}
				applyHeaderFlags(sess)
				if *sess.Meta.Project != project || *sess.Meta.Topic != topic {
					t.Fatal("omitted flags replaced saved labels")
				}
				if err := cmd.Flags().Set("project", value); err != nil {
					t.Fatal(err)
				}
				applyHeaderFlags(sess)
				if *sess.Meta.Project != value || *sess.Meta.Topic != topic {
					t.Fatal("project override affected the topic or was ignored")
				}
				if err := cmd.Flags().Set("topic", value); err != nil {
					t.Fatal(err)
				}
				applyHeaderFlags(sess)
				if *sess.Meta.Project != value || *sess.Meta.Topic != value {
					t.Fatal("topic override affected the project or was ignored")
				}
			})
		}
	}
}
