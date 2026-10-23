package cli

import (
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
	root := filepath.Join(t.TempDir(), "project")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_SYSTEM", os.DevNull)
	worktree := filepath.Join(t.TempDir(), "different-name")
	for _, args := range [][]string{
		{"init", "-q"}, {"config", "user.name", "Test"}, {"config", "user.email", "test@example.com"},
		{"commit", "--allow-empty", "-qm", "base"}, {"worktree", "add", "--detach", worktree},
	} {
		if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			t.Fatalf("%s: %v", out, err)
		}
	}
	for _, dir := range []string{root, worktree} {
		if got := reviewProject(dir); got != "project" {
			t.Errorf("reviewProject(%q) = %q, want project", dir, got)
		}
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
