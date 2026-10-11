package cli

import (
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/crittylabs/critty/internal/review"
)

var reviewTopic string
var reviewProjectName string
var headerCommands []*cobra.Command

func addHeaderFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(&reviewTopic, "topic", "", "header topic (defaults to the Git review branch; empty hides it)")
	cmd.Flags().StringVar(&reviewProjectName, "project", "", "header project name (defaults to the repository or directory name; empty hides it)")
	headerCommands = append(headerCommands, cmd)
}

func headerFlagSpecified(name string) bool {
	for _, cmd := range headerCommands {
		if cmd.Flags().Changed(name) {
			return true
		}
	}
	return false
}

func reviewProjectLabel(root string, sess *review.Session) string {
	if sess.Meta.Project != nil {
		return *sess.Meta.Project
	}
	return reviewProject(root)
}

func applyHeaderFlags(sess *review.Session) {
	if headerFlagSpecified("topic") {
		topic := reviewTopic
		sess.Meta.Topic = &topic
	}
	if headerFlagSpecified("project") {
		project := reviewProjectName
		sess.Meta.Project = &project
	}
}

func reviewProject(root string) string {
	// The common Git directory identifies the project even in a linked worktree.
	cmd := exec.Command("git", "rev-parse", "--path-format=absolute", "--git-common-dir")
	cmd.Dir = root
	if out, err := cmd.Output(); err == nil {
		common := strings.TrimSpace(string(out))
		if filepath.Base(common) == ".git" {
			return filepath.Base(filepath.Dir(common))
		}
		return strings.TrimSuffix(filepath.Base(common), ".git")
	}
	if root == "" {
		return ""
	}
	return filepath.Base(root)
}

func reviewTopicLabel(sess *review.Session, mode *reviewMode) string {
	if sess.Meta.Topic != nil {
		return *sess.Meta.Topic
	}
	if !mode.code() || mode.patch != nil {
		return ""
	}
	if mode.source != nil && mode.source.Scope == "range" {
		_, head, _ := strings.Cut(mode.source.Range, "..")
		head = strings.TrimPrefix(head, ".")
		if head != "" && head != "HEAD" {
			return head
		}
	}
	if sess.Meta.Branch == "HEAD" {
		return ""
	}
	return sess.Meta.Branch
}
