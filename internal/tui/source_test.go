package tui

import (
	"os"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	gitpkg "github.com/crittylabs/critty/internal/git"
	"github.com/crittylabs/critty/internal/review"
)

func TestRangeReviewUsesCommittedContentAcrossRounds(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_SYSTEM", os.DevNull)
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	runGitIn(t, dir, "init", "-q")
	runGitIn(t, dir, "config", "user.name", "Test")
	runGitIn(t, dir, "config", "user.email", "test@example.com")
	runGitIn(t, dir, "commit", "--allow-empty", "-qm", "base")
	if err := os.WriteFile("file.txt", []byte("committed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGitIn(t, dir, "add", ".")
	runGitIn(t, dir, "commit", "-qm", "next")
	if err := os.WriteFile("file.txt", []byte("working\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	source, err := gitpkg.ResolveRange("HEAD~1..HEAD")
	if err != nil {
		t.Fatal(err)
	}
	files, err := source.Files()
	if err != nil {
		t.Fatal(err)
	}
	sess, err := review.OpenCodeSessionWithArgs("", []string{"--scope", source.Range})
	if err != nil {
		t.Fatal(err)
	}
	app := NewCodeReviewApp(files, source.Base, AppConfig{Session: sess, Source: &source})
	updated, _ := app.Update(docRenderedMsg{})
	app = updated.(AppModel)
	app.width, app.height = 100, 30
	app.recalculateLayout()
	for i := 0; i < 2; i++ {
		if app.tab().doc.Content != "committed\n" || app.reviewScopeLabel() != "Range: HEAD~1..HEAD" {
			t.Fatal("range source changed")
		}
		app = restartRound(t, app)
	}
}

func TestEmptyReviewCanFinishByMouse(t *testing.T) {
	app := NewCodeReviewApp(nil, "HEAD", AppConfig{Staged: true})
	app.width, app.height = 100, 24
	app.recalculateLayout()
	app.rebuildContent()
	app.updateCommentSidebar()
	if !strings.Contains(app.View().Content, "No changes") {
		t.Fatal("missing empty state")
	}
	app.openFinishModal()
	_, regions := app.renderEmptyReview()
	for _, region := range regions {
		if region.action.focus != 0 {
			continue
		}
		_, cmd := app.handleMouseClick(tea.MouseClickMsg{X: region.rect.left, Y: region.rect.top, Button: tea.MouseLeft})
		if cmd == nil {
			t.Fatal("approve click did not finish")
		}
		if _, ok := cmd().(tea.QuitMsg); !ok {
			t.Fatal("approve click did not quit")
		}
		return
	}
	t.Fatal("approve button missing")
}
