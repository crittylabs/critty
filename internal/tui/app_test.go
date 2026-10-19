package tui

import (
	"encoding/json"
	"fmt"
	"image/color"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"

	"github.com/crittylabs/critty/internal/document"
	gitpkg "github.com/crittylabs/critty/internal/git"
	"github.com/crittylabs/critty/internal/review"
)

func TestReviewHost(t *testing.T) {
	for _, host := range []string{"", "tmux", "Herdr"} {
		t.Run(host, func(t *testing.T) {
			t.Setenv("CRITTY_HOST", host)
			if got := reviewHost(); got != host {
				t.Fatalf("reviewHost() = %q, want %q", got, host)
			}
		})
	}
}

func renderedCells(s string) ([]rune, []uv.Style) {
	var runes []rune
	var styles []uv.Style
	var pen uv.Style
	parser := ansi.NewParser()
	parser.SetHandler(ansi.Handler{
		Print: func(r rune) {
			runes = append(runes, r)
			styles = append(styles, pen)
		},
		HandleCsi: func(cmd ansi.Cmd, params ansi.Params) {
			if cmd == 'm' {
				uv.ReadStyle(params, &pen)
			}
		},
	})
	for i := range len(s) {
		parser.Advance(s[i])
	}
	return runes, styles
}

func sameColor(a, b color.Color) bool {
	return a != nil && b != nil && color.RGBAModel.Convert(a) == color.RGBAModel.Convert(b)
}

func TestReviewBarProjectAndTopic(t *testing.T) {
	app := setupAppWithDoc(t, "first\nsecond\n")
	app.project, app.topic = "critty", "header-project-topic"
	app.width = 120
	bar := ansi.Strip(app.renderReviewBar())
	if !strings.HasPrefix(bar, ">_ critty  critty · header-project-topic — review the document.") {
		t.Fatalf("bar = %q", bar)
	}
	app.topic = "\x1b[31m" + strings.Repeat("長いトピック", 20) + "\n\t"
	for _, width := range []int{20, 60, 100} {
		app.width = width
		bar := app.renderReviewBar()
		if lipgloss.Width(bar) != width || lipgloss.Height(bar) != 1 {
			t.Fatalf("width %d: bar does not fit on one line: %q", width, bar)
		}
		if !strings.HasSuffix(ansi.Strip(bar), " Submit q ") {
			t.Fatalf("width %d: Submit button was truncated: %q", width, bar)
		}
	}
}

func TestTabFilenameEmphasis(t *testing.T) {
	app := setupAppWithDoc(t, "line\n")
	app.tabs[0].path = "internal/.github/x/--/日本 file.go"
	app.tabs[0].changedLines = map[int]bool{1: true}
	app.tabs[0].deletedAfter = map[int][]gitpkg.DeletedLine{0: {{OldLineNum: 1, Content: "old"}}}
	for _, active := range []int{0, -1} {
		app.activeTab = active
		for _, tt := range []struct {
			name  string
			state *fileReview
			want  string
		}{
			{name: "no state"},
			{name: "no comments", state: &fileReview{}},
			{name: "line comment", state: &fileReview{Comments: []review.Comment{{Scope: "line"}}}, want: "💬"},
			{name: "resolved", state: &fileReview{Comments: []review.Comment{{Resolved: true}}}},
			{name: "file comment", state: &fileReview{Comments: []review.Comment{{Scope: "file"}}}, want: "💬"},
		} {
			t.Run(fmt.Sprintf("active=%d/%s", active, tt.name), func(t *testing.T) {
				app.tabs[0].state = tt.state
				rendered := app.renderTab(app.tabLabels(), 0, true)
				var underlined, bold, italic strings.Builder
				runes, styles := renderedCells(rendered)
				for i, r := range runes {
					pen := styles[i]
					if (r == '(' || r == ')') && !sameColor(pen.Fg, muted) {
						t.Errorf("delimiter %q foreground = %v, want %v", r, pen.Fg, muted)
					}
					if pen.Underline != uv.UnderlineNone {
						underlined.WriteRune(r)
					}
					if pen.Attrs&uv.AttrBold != 0 {
						bold.WriteRune(r)
					}
					if pen.Attrs&uv.AttrItalic != 0 {
						italic.WriteRune(r)
					}
				}
				if got := underlined.String(); got != "" {
					t.Fatalf("tab label still uses underlining: %q", got)
				}
				if got := strings.Contains(ansi.Strip(rendered), "💬"); got != (tt.want != "") {
					t.Fatalf("tab bubble presence = %t, want %t", got, tt.want != "")
				}
				wantBold := ""
				if active == 0 {
					wantBold = "日本 file.go"
				}
				if got := bold.String(); got != wantBold {
					t.Fatalf("bold text = %q, want %q", got, wantBold)
				}
				if got := italic.String(); got != "i.g" {
					t.Fatalf("italic text = %q, want %q", got, "i.g")
				}
				if !strings.Contains(ansi.Strip(rendered), "i/.g/x/--/日本 file.go (+1 -1)") {
					t.Fatalf("tab label changed: %q", rendered)
				}
			})
		}
	}
}

func TestRenderHeaderPathBackground(t *testing.T) {
	app := setupAppWithDoc(t, "first\nsecond\n")
	app.filePath = "path/to/file"
	app.width = 80
	_, header, _ := strings.Cut(app.renderHeader(), "\n")
	if !strings.Contains(strings.Join(strings.Fields(ansi.Strip(header)), " "), "path/to/file L0/3 0/0 💬") {
		t.Fatalf("path and status should retain their order: %q", header)
	}
	runes, styles := renderedCells(header)
	plain := string(runes)
	runeIndex := func(text string) int {
		i := strings.Index(plain, text)
		if i < 0 {
			t.Fatalf("header missing %q: %q", text, plain)
		}
		return len([]rune(plain[:i]))
	}
	gapStart, gapEnd := runeIndex("L0/3")+len("L0/3"), runeIndex("0/0")
	for i := gapStart; i < gapEnd-1; i++ {
		if runes[i] != ' ' || !sameColor(styles[i].Bg, accent) {
			t.Fatalf("header gap must contain spaces with the accent background: %+v", styles[i])
		}
	}
	if runes[gapEnd-1] != ' ' || !sameColor(styles[gapEnd-1].Bg, buttonBackground) {
		t.Fatalf("summary must have one space inside its background: %+v", styles[gapEnd-1])
	}
	for _, part := range []struct {
		text string
		fg   color.Color
		bg   color.Color
	}{
		{"path/to/file", lipgloss.BrightWhite, accent},
		{"L0/3", lipgloss.Cyan, accent},
		{"0/0", success, buttonBackground},
	} {
		start := runeIndex(part.text)
		for _, style := range styles[start : start+len(part.text)] {
			if !sameColor(style.Fg, part.fg) || !sameColor(style.Bg, part.bg) {
				t.Fatalf("wrong colors in %q: %+v", part.text, style)
			}
		}
	}
}

func TestRenderHeaderStaysOnOneLineWithLongPath(t *testing.T) {
	tests := []struct {
		name  string
		setup func(*AppModel)
		want  string
	}{
		{name: "document", want: "L0/3 0/0 💬"},
		{name: "deleted", setup: func(app *AppModel) { app.tabs[0].cursorSide = "old" }, want: "L0 (deleted) 0/0 💬"},
		{name: "selection", setup: func(app *AppModel) { app.tabs[0].selecting = true }, want: "VISUAL  L0-0"},
		{name: "loading", setup: func(app *AppModel) { app.tabs[0].doc = nil }, want: "0/0 💬"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := setupAppWithDoc(t, "first\nsecond\n")
			app.width = 40
			app.filePath = strings.Repeat("long/", 20) + "file.md"
			if tt.setup != nil {
				tt.setup(&app)
			}

			header := app.renderHeader()
			plainHeader := ansi.Strip(header)
			if got := lipgloss.Height(header); got != 2 {
				t.Errorf("header height = %d, want bar and file line: %q", got, plainHeader)
			}
			if !strings.Contains(plainHeader, "…") {
				t.Errorf("header = %q, want truncated path", plainHeader)
			}
			if !strings.Contains(strings.Join(strings.Fields(plainHeader), " "), strings.Join(strings.Fields(tt.want), " ")) {
				t.Errorf("header = %q, want status %q", plainHeader, tt.want)
			}
		})
	}
}

func TestViewEnablesMouseHoverReporting(t *testing.T) {
	app := setupAppWithDoc(t, "line\n")
	app.width = 80
	app.height = 20

	if got := app.View().MouseMode; got != tea.MouseModeAllMotion {
		t.Fatalf("mouse mode = %v, want all-motion reporting", got)
	}
}

func TestKeyboardCommentsDeletedLineRange(t *testing.T) {
	app := setupAppWithDoc(t, "first\nfourth\n")
	app.width = 100
	app.contentViewport.SetWidth(75)
	app.commentViewport.SetWidth(25)
	app.commentViewport.SetHeight(10)
	app.tabs[0].deletedAfter = map[int][]gitpkg.DeletedLine{
		1: {
			{OldLineNum: 2, Content: "second"},
			{OldLineNum: 3, Content: "third"},
		},
	}
	app.tabs[0].cursorLine = 1
	app.rebuildContent()

	app = pressKey(app, 'j')
	if app.tab().cursorLine != 2 || app.tab().cursorSide != "old" {
		t.Fatalf("cursor = %d/%s, want deleted line 2", app.tab().cursorLine, app.tab().cursorSide)
	}
	app = pressKey(app, 'v')
	app = pressKey(app, 'j')
	app = pressKey(app, tea.KeyEnter)
	if app.modal != commentModal || app.canSuggest() {
		t.Fatalf("modal = %v, can suggest = %t; want deleted-line comment without Suggest", app.modal, app.canSuggest())
	}
	app.modalTextarea.SetValue("restore this logic")
	app.modalSubmit()

	if len(app.tab().state.Comments) != 1 {
		t.Fatalf("comments = %+v, want one", app.tab().state.Comments)
	}
	c := app.tab().state.Comments[0]
	if c.Side != "old" || c.StartLine != 2 || c.EndLine != 3 || c.Anchor != "second\nthird" {
		t.Fatalf("comment = %+v, want old lines 2-3 with deleted anchor", c)
	}
}

func TestLineCommentCritJSONCompatibility(t *testing.T) {
	tests := []struct {
		name    string
		side    string
		start   int
		end     int
		anchor  string
		deleted map[int][]gitpkg.DeletedLine
		changed map[int]bool
	}{
		{name: "added line", start: 1, end: 1, anchor: "first", changed: map[int]bool{1: true}},
		{name: "added range", start: 1, end: 2, anchor: "first\nsecond", changed: map[int]bool{1: true, 2: true}},
		{name: "deleted line", side: "old", start: 2, end: 2, anchor: "removed", deleted: map[int][]gitpkg.DeletedLine{
			1: {{OldLineNum: 2, Content: "removed"}},
		}},
		{name: "deleted range", side: "old", start: 2, end: 3, anchor: "removed\nalso removed", deleted: map[int][]gitpkg.DeletedLine{
			1: {
				{OldLineNum: 2, Content: "removed"},
				{OldLineNum: 3, Content: "also removed"},
			},
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := setupAppWithDoc(t, "first\nsecond\n")
			app.tabs[0].deletedAfter = tt.deleted
			app.tabs[0].changedLines = tt.changed
			app.tabs[0].cursorLine = tt.end
			app.tabs[0].cursorSide = tt.side
			if tt.start != tt.end {
				app.tabs[0].selecting = true
				app.tabs[0].selectAnchor = tt.start
				app.tabs[0].selectSide = tt.side
			}
			app.openLineComment()
			app.modalTextarea.SetValue("review note")
			app.modalSubmit()

			data, err := json.Marshal(app.tab().state.Comments[0])
			if err != nil {
				t.Fatalf("marshaling comment: %v", err)
			}
			var wire map[string]any
			if err := json.Unmarshal(data, &wire); err != nil {
				t.Fatalf("unmarshaling comment: %v", err)
			}
			if got := int(wire["start_line"].(float64)); got != tt.start {
				t.Errorf("start_line = %d, want %d", got, tt.start)
			}
			if got := int(wire["end_line"].(float64)); got != tt.end {
				t.Errorf("end_line = %d, want %d", got, tt.end)
			}
			if got := wire["anchor"]; got != tt.anchor {
				t.Errorf("anchor = %q, want %q", got, tt.anchor)
			}
			gotSide, hasSide := wire["side"]
			if tt.side == "" && hasSide {
				t.Errorf("new-side JSON contains side = %q, want omitted", gotSide)
			}
			if tt.side == "old" && gotSide != "old" {
				t.Errorf("old-side JSON side = %q, want old", gotSide)
			}
		})
	}
}

func TestOldSideCommentRendersAfterDeletedLine(t *testing.T) {
	app := setupAppWithDoc(t, "first\nthird\n")
	app.width = 100
	app.contentViewport.SetWidth(75)
	app.contentViewport.SetHeight(12)
	app.commentViewport.SetWidth(25)
	app.commentViewport.SetHeight(10)
	app.tabs[0].deletedAfter = map[int][]gitpkg.DeletedLine{
		1: {{OldLineNum: 2, Content: "second"}},
	}
	app.tabs[0].state.Comments = []review.Comment{{
		ID: "c_old", Side: "old", StartLine: 2, EndLine: 2, Body: "deleted comment",
	}}
	app.updateCommentSidebar()
	app.rebuildContent()

	r := app.contentLayout.oldRanges[2]
	if r.end-r.start < 2 {
		t.Fatalf("deleted line occupies %d row(s), want inline annotation", r.end-r.start)
	}
	if got := ansi.Strip(app.contentViewport.View()); !strings.Contains(got, "deleted comment") {
		t.Fatalf("content = %q, want old-side annotation", got)
	}
	if got := ansi.Strip(app.commentViewport.View()); !strings.Contains(got, "L2 (deleted)") {
		t.Fatalf("sidebar = %q, want deleted-side label", got)
	}
}

func TestDeletedFileRendersSelectableLines(t *testing.T) {
	app := NewApp("deleted.go", AppConfig{})
	app.tabs[0].doc = &document.Document{Path: "deleted.go"}
	app.tabs[0].isDeleted = true
	app.tabs[0].deletedAfter = map[int][]gitpkg.DeletedLine{
		0: {
			{OldLineNum: 1, Content: "package deleted"},
			{OldLineNum: 2, Content: "var removed = true"},
		},
	}
	app.tabs[0].cursorLine = 1
	app.tabs[0].cursorSide = "old"
	app.contentViewport.SetWidth(75)
	app.contentViewport.SetHeight(10)
	app.rebuildContent()

	content := ansi.Strip(app.contentViewport.View())
	if !strings.Contains(content, "package deleted") || !strings.Contains(content, "var removed = true") {
		t.Fatalf("content = %q, want deleted file contents", content)
	}
	app = pressKey(app, 'j')
	if app.tab().cursorLine != 2 || app.tab().cursorSide != "old" {
		t.Fatalf("cursor = %d/%s, want deleted line 2", app.tab().cursorLine, app.tab().cursorSide)
	}
}

func TestReviewFrameFitsTerminalWithSidebar(t *testing.T) {
	for _, width := range []int{60, 120} {
		for _, hidden := range []bool{false, true} {
			t.Run(fmt.Sprintf("width=%d/hidden=%t", width, hidden), func(t *testing.T) {
				app := setupAppWithDoc(t, strings.Repeat("source\n", 30))
				app.width, app.height, app.multiFile = width, 30, true
				app.hideComments = hidden
				app.tab().state.Comments = []review.Comment{{ID: "file", Scope: "file", Body: "comment"}}
				app.recalculateLayout()
				app.updateCommentSidebar()
				app.rebuildContent()
				rows := strings.Split(ansi.Strip(app.View().Content), "\n")
				_, top, contentRight, bottom := app.contentBounds()
				paneRight := app.contentPaneWidth() - 1
				for y := top; y < bottom; y++ {
					if lipgloss.Width(rows[y]) != width || ansi.Cut(rows[y], width-1, width) != "│" {
						t.Fatalf("row %d: width=%d, want %d with right border at last column; %q", y, lipgloss.Width(rows[y]), width, rows[y])
					}
					if ansi.Cut(rows[y], paneRight, paneRight+1) != "│" {
						t.Fatalf("row %d: want content pane border at column %d; %q", y, paneRight, rows[y])
					}
				}
				left, _, right, _ := app.commentBounds()
				if hidden {
					if left != contentRight || right != paneRight {
						t.Fatalf("gutter bounds [%d,%d), want [%d,%d)", left, right, contentRight, paneRight)
					}
					return
				}
				if left != paneRight+2 || right != width-1 {
					t.Fatalf("sidebar bounds [%d,%d), want [%d,%d) inside the comment pane", left, right, paneRight+2, width-1)
				}
				if got := ansi.Cut(rows[top-1], paneRight+1, width); (!strings.HasPrefix(got, "│") && !strings.HasPrefix(got, "├")) || !strings.HasSuffix(got, "╮") {
					t.Fatalf("comment pane tab row = %q, want its tab joined to the top border", got)
				}
				if got := ansi.Cut(rows[bottom], paneRight-1, width); got != "─╯╰"+strings.Repeat("─", width-paneRight-3)+"╯" {
					t.Fatalf("bottom row = %q, want both panes closed side by side", got)
				}
			})
		}
	}
}

func TestHelpModalShowsAllShortcutGroupsAndCloses(t *testing.T) {
	app := setupAppWithDoc(t, "first\nsecond\n")
	app.width = 80
	app.height = 24

	app = pressAltKey(app, '?')
	if app.modal != helpModal {
		t.Fatalf("? opened modal %v, want help modal", app.modal)
	}

	background := lipgloss.NewStyle().Width(app.width).Height(app.height).Render("")
	rendered := app.renderWithModal(background)
	help := app.renderHelp(app.modalInnerWidth())
	for _, want := range []string{
		"Keyboard Help", "General", "Navigation", "Code review", "Selection and dialogs",
		"↑/↓,j/k", "PgUp/PgDn", "Home/End,g/G,</>", "TAB/S-TAB", "C-s", "C-PgUp/PgDn", "a/r/c", "M-p",
		"Incremental search", "C-s / C-r", "C-d / C-w", "DEL / C-y", "M-s i/c/w/r/m", "terminal paste",
		"C-: ctrl+, M-: meta/alt+, S-: shift+", "RET: enter", "DEL: backspace",
	} {
		if !strings.Contains(rendered+help, want) {
			t.Errorf("help modal does not contain %q", want)
		}
	}
	if width := lipgloss.Width(rendered); width > app.width {
		t.Errorf("help width = %d, terminal width = %d", width, app.width)
	}
	if height := lipgloss.Height(rendered); height > app.height {
		t.Errorf("help height = %d, terminal height = %d", height, app.height)
	}

	app = pressKey(app, tea.KeyEscape)
	if app.modal != noModal {
		t.Fatalf("esc left modal %v open", app.modal)
	}

	app = pressAltKey(app, '?')
	app = pressAltKey(app, '?')
	if app.modal != noModal {
		t.Fatalf("second ? left modal %v open", app.modal)
	}
}

func TestHelpModalScrollsToBottom(t *testing.T) {
	app := newISearchApp(t, 1)
	app.version = "v1.0.0"
	app, _ = updateApp(app, tea.WindowSizeMsg{Width: 80, Height: 24})
	app = pressAltKey(app, '?')
	app = pressKey(app, tea.KeyEnd)
	if app.helpOffset == 0 || !strings.Contains(ansi.Strip(app.View().Content), "pending/completed item") {
		t.Fatalf("end did not show the bottom of keyboard help (offset %d):\n%s", app.helpOffset, ansi.Strip(app.View().Content))
	}
	if !strings.Contains(ansi.Strip(app.View().Content), "C-: ctrl+, M-: meta/alt+, S-: shift+") {
		t.Fatal("modifier legend scrolled out of keyboard help")
	}
	app = pressKey(app, tea.KeyHome)
	if app.helpOffset != 0 {
		t.Fatal("home did not restore the top of keyboard help")
	}
	app = pressKey(app, tea.KeyPgDown)
	if app.helpOffset == 0 {
		t.Fatal("Page Down did not scroll keyboard help")
	}
	app = pressKey(app, tea.KeyPgUp)
	if app.helpOffset != 0 {
		t.Fatal("Page Up did not restore the top of keyboard help")
	}
	app = ctrlKey(app, 'd')
	if app.helpOffset == 0 {
		t.Fatal("ctrl+d did not scroll keyboard help")
	}
	app = ctrlKey(app, 'u')
	if app.helpOffset != 0 {
		t.Fatal("ctrl+u did not restore the top of keyboard help")
	}
	app, _ = updateApp(app, tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	if app.helpOffset == 0 {
		t.Fatal("mouse wheel did not scroll keyboard help")
	}
	for _, region := range app.modalMouseRegions() {
		if region.action.scrollable && region.action.scrollOffset > 0 {
			app = clickMouse(app, region.rect.left, region.rect.top)
			break
		}
	}
	if !strings.Contains(ansi.Strip(app.View().Content), "M-s i/c/w/r/m") {
		t.Fatal("Down button did not reveal search mode shortcuts")
	}
	app, _ = updateApp(app, tea.WindowSizeMsg{Width: 80, Height: 40})
	view := app.View()
	if !strings.Contains(ansi.Strip(view.Content), "Keyboard Help") || lipgloss.Height(view.Content) > 40 {
		t.Fatal("resizing did not fit the help content to the larger terminal")
	}
	app = pressKey(app, tea.KeyHome)
	if !strings.Contains(ansi.Strip(app.View().Content), "General") {
		t.Fatal("the top of help is unreachable after resizing")
	}
	for _, dark := range []bool{true, false} {
		initAdaptiveStyles(dark)
		for _, size := range []tea.WindowSizeMsg{{Width: 80, Height: 24}, {Width: 40, Height: 12}} {
			app, _ = updateApp(app, size)
			app = pressKey(app, tea.KeyEnd)
			view := app.View()
			if !strings.Contains(ansi.Strip(view.Content), "critty v1.0.0") || lipgloss.Height(view.Content) > size.Height {
				t.Fatal("help version missing or dialog clipped after scrolling and resizing")
			}
			footer, controls := app.helpFooter(app.modalInnerWidth(), 1, 0)
			rows := strings.Split(ansi.Strip(footer), "\n")
			if !strings.Contains(rows[len(rows)-1], "Up") || !strings.Contains(rows[len(rows)-1], "Down") {
				t.Fatal("help scroll controls are not on the bottom row")
			}
			if size.Width == 80 && !strings.Contains(rows[len(rows)-1], "critty v1.0.0") {
				t.Fatal("help version is not beside the scroll controls")
			}
			for _, control := range controls {
				if control.rect.top != len(rows)-1 {
					t.Fatal("help control hit area does not match the bottom row")
				}
			}
			for _, region := range app.modalMouseRegions() {
				if (region.action.close || region.action.scrollable) && (region.rect.top < 0 || region.rect.bottom > size.Height) {
					t.Fatal("help control hidden by version footer")
				}
			}
		}
	}
	initAdaptiveStyles(true)
}

func TestDocRenderedMsg_LoadsExistingComments(t *testing.T) {
	testFile := filepath.Join(t.TempDir(), "test.go")
	if err := os.WriteFile(testFile, []byte("package main\n\nfunc main() {}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	sess := setupSession(t, testFile)
	comment := testComment()
	comment.Anchor = "package main"
	sess.SetFileComments(testFile, "", []review.Comment{comment})
	if err := sess.Save(); err != nil {
		t.Fatal(err)
	}

	app := NewApp(testFile, AppConfig{Session: sess, Author: "Tester"})
	app.tabs = []FileTab{{path: testFile}}
	app, _ = updateApp(app, docRenderedMsg{})

	if state := app.tabs[0].state; state == nil || len(state.Comments) != 1 || state.Comments[0].ID != comment.ID || state.Comments[0].Body != comment.Body {
		t.Fatalf("state = %+v, want the saved comment", state)
	}
}

func TestResolveKey_TogglesSelectedComment(t *testing.T) {
	app, _ := newFinishTestApp(t, []review.Comment{testComment()})
	app.commentViewport.SetWidth(40)
	app.commentViewport.SetHeight(20)
	app.focused = commentPane
	app.updateCommentSidebar()

	app = pressKey(app, 'r')

	comment := app.tabs[0].state.Comments[0]
	if !comment.Resolved || comment.ResolvedRound != 1 {
		t.Fatalf("expected resolved comment in round 1, got %+v", comment)
	}
	if len(app.tabs[0].sidebarItems) != 1 || !app.tabs[0].sidebarItems[0].resolved {
		t.Fatalf("resolved comment lost its sidebar header: %+v", app.tabs[0].sidebarItems)
	}
	if got := app.commentViewport.View(); !strings.Contains(got, comment.Body) {
		t.Fatalf("resolved sidebar message = %q", got)
	}

	// The inline annotation remains available so the thread can be reopened.
	app.focused = contentPane
	app.tabs[0].doc = &document.Document{Path: "test.go", Content: "line", Lines: []string{"line"}}
	app.tabs[0].cursorLine = comment.EndAt()
	app.tabs[0].cursorOnAnnotation = true
	app.tabs[0].cursorAnnoIdx = 0
	app = pressKey(app, 'r')

	comment = app.tabs[0].state.Comments[0]
	if comment.Resolved || comment.ResolvedRound != 0 {
		t.Fatalf("expected unresolved comment with cleared round, got %+v", comment)
	}
}

func TestFileCommentShortcutCreatesInlineFileComment(t *testing.T) {
	app := setupAppWithDoc(t, "first\nsecond\n")
	app.contentViewport.SetWidth(80)
	app.contentViewport.SetHeight(12)
	app.commentViewport.SetWidth(40)
	app.commentViewport.SetHeight(20)

	app = pressKey(app, 'f')
	if app.modal != fileCommentModal {
		t.Fatalf("f opened modal %v, want file comment modal", app.modal)
	}

	app.modalTextarea.SetValue("applies to the whole file")
	app.modalSubmit()

	comments := app.tabs[0].state.Comments
	if len(comments) != 1 {
		t.Fatalf("comments = %+v, want one file comment", comments)
	}
	comment := comments[0]
	if comment.Scope != "file" || comment.StartLine != 0 || comment.EndLine != 0 || comment.Anchor != "" {
		t.Fatalf("file comment has line metadata: %+v", comment)
	}
	if got := app.session.FileComments(app.tab().path); len(got) != 1 || got[0].Scope != "file" {
		t.Fatalf("persisted comments = %+v, want one file comment", got)
	}
	if got := app.commentViewport.View(); !strings.Contains(got, "File") || !strings.Contains(got, comment.Body) {
		t.Fatalf("sidebar = %q, want file comment", got)
	}
	if targets := app.commentTargets(0); len(targets) != 1 || targets[0].scope != "file" {
		t.Fatalf("navigation targets = %+v, want one file-scoped target", targets)
	}
	if got := app.contentViewport.View(); !strings.Contains(got, comment.Body) {
		t.Fatalf("file comment missing inline: %q", got)
	}
}

func TestFileCommentShortcutRepliesToExistingThread(t *testing.T) {
	for _, resolved := range []bool{false, true} {
		for _, ownReply := range []bool{false, true} {
			for _, hidden := range []bool{false, true} {
				t.Run(fmt.Sprintf("resolved=%t/ownReply=%t/hidden=%t", resolved, ownReply, hidden), func(t *testing.T) {
					app := setupAppWithDoc(t, "first\nsecond\n")
					thread := review.Comment{
						ID: "file-thread", Scope: "file", Body: "original", Author: app.author,
						ReviewRound: app.reviewRound(), Resolved: resolved,
					}
					if ownReply {
						thread.Replies = []review.Reply{{ID: "own-reply", Body: "previous reply", Author: app.author, ReviewRound: app.reviewRound()}}
					}
					app.tab().state.Comments = []review.Comment{
						{ID: "line", Scope: "line", StartLine: 1, EndLine: 1, Body: "line comment"},
						thread,
						{ID: "other-file-thread", Scope: "file", Body: "another thread"},
					}
					app.hideComments = hidden
					app.updateCommentSidebar()
					app = pressKey(app, 'f')
					if app.modal != replyModal || app.editingID != thread.ID || app.editingReplyID != "" {
						t.Fatalf("f opened modal %v for %q/%q, want a new reply to %q", app.modal, app.editingID, app.editingReplyID, thread.ID)
					}
					if app.modalTextarea.Value() != "" || !app.modalTextarea.Focused() || app.modalInitial != "" {
						t.Fatal("reply editor should be empty and focused")
					}
					app.modalTextarea.SetValue("new reply")
					app.modalSubmit()
					comments := app.session.FileComments(app.tab().path)
					if len(comments) != 3 {
						t.Fatalf("got %d threads, want 3", len(comments))
					}
					got := comments[1]
					if got.Body != thread.Body || got.Resolved != thread.Resolved || len(got.Replies) != len(thread.Replies)+1 || got.Replies[len(got.Replies)-1].Body != "new reply" {
						t.Fatalf("persisted thread = %+v, want original body and appended reply", got)
					}
					if ownReply && got.Replies[0].Body != "previous reply" {
						t.Fatal("existing reply was overwritten")
					}
				})
			}
		}
	}
}

func TestRoundStart_ReloadsCommentsAndAdvancesRound(t *testing.T) {
	app, _ := newFinishTestApp(t, []review.Comment{testComment()})
	if err := os.WriteFile("test.go", []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	app.persist()
	// Load the document so the next round has previous content to carry
	// comments forward from; docRenderedMsg reloads state from the session.
	updated0, _ := app.Update(docRenderedMsg{})
	app = updated0.(AppModel)

	// Simulate the agent replying via the CLI while the TUI waits.
	other, err := review.OpenDocSession("", "test.go")
	if err != nil {
		t.Fatal(err)
	}
	if err := other.AppendReply("c_test01", "fixed it", "AI", "", true, ""); err != nil {
		t.Fatalf("reply: %v", err)
	}
	if err := other.Save(); err != nil {
		t.Fatal(err)
	}

	app = restartRound(t, app)

	if app.session.CJ.ReviewRound != 2 {
		t.Errorf("expected round 2, got %d", app.session.CJ.ReviewRound)
	}
	comments := app.tabs[0].state.Comments
	if len(comments) != 1 || len(comments[0].Replies) != 1 {
		t.Fatalf("expected reloaded comment with reply, got %+v", comments)
	}
	if !comments[0].Resolved {
		t.Error("expected reloaded comment to be resolved")
	}
	if !comments[0].CarriedForward || comments[0].ID != "c_test01" {
		t.Errorf("expected a carried-forward comment with the same ID, got %+v", comments[0])
	}
}

func TestRoundStartRefreshesCodeReviewTabs(t *testing.T) {
	dir := newGitRepo(t)
	if err := os.WriteFile("existing.go", []byte("package existing\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGitIn(t, dir, "add", "existing.go")
	runGitIn(t, dir, "commit", "-q", "-m", "initial")
	if err := os.WriteFile("existing.go", []byte("package existing\n\nvar changed = true\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	files, err := gitpkg.ReviewSource{Scope: "all"}.Files()
	if err != nil {
		t.Fatal(err)
	}
	app := NewCodeReviewApp(files, "HEAD", AppConfig{})
	if len(app.tabs) != 1 {
		t.Fatalf("initial tabs = %d, want 1", len(app.tabs))
	}

	newFiles := []string{"go.mod", "internal/cli/process_darwin.go", "internal/cli/process_linux.go", "internal/cli/process_other.go"}
	for _, path := range newFiles {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("new file\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		runGitIn(t, dir, "add", path)
	}

	app = restartRound(t, app)
	want := []string{"internal/cli/process_darwin.go", "internal/cli/process_linux.go", "internal/cli/process_other.go", "existing.go", "go.mod"}
	if len(app.tabs) != len(want) {
		t.Fatalf("refreshed tabs = %d, want %d", len(app.tabs), len(want))
	}
	for i, path := range want {
		if app.tabs[i].path != path {
			t.Errorf("tab %d = %q, want %q", i, app.tabs[i].path, path)
		}
	}
}

func TestStagedRoundUsesIndexDocument(t *testing.T) {
	dir := newGitRepo(t)
	path := filepath.Join(dir, "partial.go")
	if err := os.WriteFile(path, []byte("package base\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGitIn(t, dir, "add", "partial.go")
	runGitIn(t, dir, "commit", "-q", "-m", "initial")
	if err := os.WriteFile(path, []byte("package staged\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGitIn(t, dir, "add", "partial.go")
	if err := os.WriteFile(path, []byte("package unstaged\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	files, err := gitpkg.ReviewSource{Scope: "staged"}.Files()
	if err != nil {
		t.Fatal(err)
	}
	sess, err := review.OpenCodeSession("")
	if err != nil {
		t.Fatal(err)
	}
	app := NewCodeReviewApp(files, "HEAD", AppConfig{Session: sess, Staged: true})
	updated, _ := app.Update(docRenderedMsg{})
	app = updated.(AppModel)
	if got := app.tab().doc.Content; got != "package staged\n" {
		t.Fatalf("initial document = %q, want indexed content", got)
	}

	if err := os.WriteFile(path, []byte("package nextstaged\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGitIn(t, dir, "add", "partial.go")
	if err := os.WriteFile(path, []byte("package nextunstaged\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	app = restartRound(t, app)
	if got := app.tab().doc.Content; got != "package nextstaged\n" {
		t.Fatalf("next-round document = %q, want indexed content", got)
	}
}

func TestRevertedAdditionShowsPlaceholderAndKeepsComments(t *testing.T) {
	dir := newGitRepo(t)
	if err := os.WriteFile(filepath.Join(dir, "keep.txt"), []byte("keep\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGitIn(t, dir, "add", "keep.txt")
	runGitIn(t, dir, "commit", "-q", "-m", "initial")
	if err := os.WriteFile(filepath.Join(dir, "added.go"), []byte("package added\nvar x = 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGitIn(t, dir, "add", "added.go")

	files, err := gitpkg.ReviewSource{Scope: "all"}.Files()
	if err != nil {
		t.Fatal(err)
	}
	sess, err := review.OpenCodeSession("")
	if err != nil {
		t.Fatal(err)
	}
	app := NewCodeReviewApp(files, "HEAD", AppConfig{Session: sess, Author: "Tester"})
	updated, _ := app.Update(docRenderedMsg{})
	app = updated.(AppModel)
	app.width, app.height = 100, 30
	app.recalculateLayout()
	for i := range app.tabs {
		if app.tabs[i].path == "added.go" {
			app.activeTab = i
			app.tabs[i].state.Comments = append(app.tabs[i].state.Comments, review.Comment{
				ID: "c_1", StartLine: 2, EndLine: 2, Body: "why one?", Anchor: "var x = 1", Author: "Tester",
			})
		}
	}
	app.persist()

	runGitIn(t, dir, "rm", "-q", "-f", "added.go")
	app = restartRound(t, app)

	if app.tab().path != "added.go" || !app.tab().outsideChanges {
		t.Fatalf("active tab = %q (outside changes: %t), want added.go kept outside the changes", app.tab().path, app.tab().outsideChanges)
	}
	content := ansi.Strip(app.contentViewport.View())
	if !strings.Contains(content, "no longer part of the changes") || !strings.Contains(content, "Original text:") {
		t.Fatalf("content = %q, want placeholder and original comment context", content)
	}
	if !strings.Contains(content, "💬") {
		t.Fatalf("content = %q, want a marker for the kept comment", content)
	}
	app.jumpToComment(1, true)
	if got := ansi.Strip(app.contentViewport.View()); !strings.Contains(got, "why one?") || !strings.Contains(got, "var x = 1") {
		t.Fatalf("opened thread = %q, want the comment and its original text", got)
	}
	app.persist()
	fresh, err := review.OpenSessionAt(sess.Key, sess.Dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := fresh.CJ.Files["added.go"].Comments; len(got) != 1 || got[0].Body != "why one?" {
		t.Fatalf("persisted comments = %+v, want the kept comment", got)
	}
}

func TestCommentSidebarCollapsesResolvedFileComments(t *testing.T) {
	fileComment := review.Comment{ID: "c_file", Scope: "file", Body: "split this file", Author: "Reviewer", Resolved: true, CreatedAt: review.Now()}
	lineComment := testComment()

	app, _ := newFinishTestApp(t, []review.Comment{lineComment, fileComment})
	app.commentViewport.SetWidth(40)
	app.commentViewport.SetHeight(20)
	app.focused = contentPane
	app.updateCommentSidebar()

	items := app.tabs[0].sidebarItems
	if len(items) != 2 || items[0].id != fileComment.ID || !items[0].resolved || items[1].id != lineComment.ID {
		t.Fatalf("sidebar items = %+v, want the resolved file comment before the line comment", items)
	}
	rendered := ansi.Strip(app.commentViewport.View())
	if !strings.Contains(rendered, "☑︎ Resolved") || strings.Contains(rendered, fileComment.Body) || !strings.Contains(rendered, lineComment.Body) {
		t.Fatalf("sidebar = %q, want a collapsed resolved header without the body beside the open thread", rendered)
	}

	app.tabs[0].sidebarCursor = 0
	app.focused = commentPane
	app = pressKey(app, 'r')

	if app.tabs[0].state.Comments[1].Resolved {
		t.Fatal("expected r to reopen the resolved file comment")
	}
	rendered = ansi.Strip(app.commentViewport.View())
	if !strings.Contains(rendered, fileComment.Body) {
		t.Fatalf("sidebar = %q, want the reopened body", rendered)
	}
}

func TestModalCloseButtonAndOutsideClickDismiss(t *testing.T) {
	open := map[string]func(AppModel) AppModel{
		"comment": func(app AppModel) AppModel { return pressKey(app, tea.KeyEnter) },
		"finish":  func(app AppModel) AppModel { return pressKey(app, 'q') },
		"help":    func(app AppModel) AppModel { return pressAltKey(app, '?') },
	}
	for name, openModal := range open {
		for _, via := range []string{"x", "outside"} {
			t.Run(name+"/"+via, func(t *testing.T) {
				app := setupAppWithDoc(t, "first\nsecond\n")
				app.width, app.height = 100, 30
				app.recalculateLayout()
				app = openModal(app)
				if app.modal == noModal {
					t.Fatal("modal did not open")
				}
				_, layout := app.renderReviewScreen()
				var x, y int
				found := false
				for _, region := range layout.modalRegions {
					if via == "x" && region.action.close {
						row := strings.Split(app.View().Content, "\n")[region.rect.top]
						if got := ansi.Strip(ansi.Cut(row, region.rect.left, region.rect.right)); got != "x" {
							t.Fatalf("close region contains %q, want x", got)
						}
						x, y, found = region.rect.left, region.rect.top, true
					}
					if via == "outside" && region.action.dialog {
						x, y, found = region.rect.right+1, region.rect.top, true
					}
				}
				if !found {
					t.Fatalf("no %s target for %s modal", via, name)
				}
				if app = clickMouse(app, x, y); app.modal != noModal {
					t.Fatalf("modal = %v after clicking %s, want closed", app.modal, via)
				}
			})
		}
	}
}

func TestResolveKeysStayOrAdvance(t *testing.T) {
	app := newCommentNavigationTestApp()
	app.multiFile = true
	app.width, app.height = 100, 30
	app.recalculateLayout()
	app.tabs[0].cursorLine = 2
	app.tabs[0].cursorOnAnnotation = true
	app.rebuildContent()
	app.updateCommentSidebar()

	app = pressKey(app, 'r')
	if c := app.tabs[0].state.Comments[0]; !c.Resolved || c.ID != "first-a" {
		t.Fatalf("r did not resolve first-a: %+v", c)
	}
	if app.focused != contentPane || !app.tab().cursorOnAnnotation || app.selectedCommentID() != "first-a" {
		t.Fatalf("r moved away: focus = %v, selected = %q", app.focused, app.selectedCommentID())
	}
	app = pressKey(app, 'r')
	if app.tabs[0].state.Comments[0].Resolved {
		t.Fatal("second r did not reopen the thread")
	}

	app = pressKey(app, 'R')
	if !app.tabs[0].state.Comments[0].Resolved {
		t.Fatal("R did not resolve first-a")
	}
	if got := app.selectedCommentID(); got != "first-b" {
		t.Fatalf("R selected %q, want the next unresolved thread first-b", got)
	}

	app.tabs[0].state.Comments[2].Resolved = true
	app.tabs[0].cursorLine, app.tabs[0].cursorOnAnnotation, app.tabs[0].cursorAnnoIdx = 2, true, 0
	if got := app.selectedCommentID(); got != "first-a" {
		t.Fatalf("setup selected %q, want first-a", got)
	}
	app = pressKey(app, 'R')
	if !app.tabs[0].state.Comments[0].Resolved {
		t.Fatal("R reopened an already resolved thread")
	}
	if got := app.selectedCommentID(); got != "first-b" {
		t.Fatalf("R from a resolved thread selected %q, want first-b", got)
	}
}

func TestFillTabRowJoinsFlushTabToPaneBorder(t *testing.T) {
	border := lipgloss.RoundedBorder()
	style := lipgloss.NewStyle().Foreground(accent)
	for _, tc := range []struct {
		name  string
		last  lipgloss.Style
		wantR string
	}{
		{"active", activeTabStyle, "│"},
		{"inactive", inactiveTabStyle, "┤"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			row := lipgloss.JoinHorizontal(lipgloss.Top, inactiveTabStyle.Render("one"), tc.last.Render("two"))
			width := lipgloss.Width(row)
			lines := strings.Split(fillTabRow(row, width, border, style), "\n")
			if len(lines) != 3 {
				t.Fatalf("tab row has %d lines, want 3", len(lines))
			}
			for i, line := range lines {
				if got := lipgloss.Width(line); got != width {
					t.Fatalf("line %d width = %d, want %d", i, got, width)
				}
			}
			bottom := ansi.Strip(lines[2])
			if !strings.HasSuffix(bottom, tc.wantR) {
				t.Fatalf("bottom line = %q, want it to end with %q", bottom, tc.wantR)
			}
			if !strings.HasSuffix(lines[2], "\x1b[m") {
				t.Fatalf("bottom line %q does not reset its style", lines[2])
			}
			if got := fillTabRow(row, width+3, border, style); !strings.HasSuffix(ansi.Strip(got), "──╮") {
				t.Fatalf("wider row = %q, want the top border filler", ansi.Strip(got))
			}
		})
	}
}

func TestReviewBarHostsSubmitButton(t *testing.T) {
	app := setupAppWithDoc(t, "first\nsecond\n")
	app.width, app.height, app.host = 100, 24, "tmux"
	app.recalculateLayout()

	rows := strings.Split(app.renderHeader(), "\n")
	if len(rows) != 2 {
		t.Fatalf("header has %d rows, want bar and file line", len(rows))
	}
	bar := rows[0]
	if got := lipgloss.Width(bar); got != app.width {
		t.Fatalf("bar width = %d, want %d", got, app.width)
	}
	plain := ansi.Strip(bar)
	if !strings.HasPrefix(plain, ">_ critty  review the document on tmux.") || !strings.HasSuffix(plain, " Submit q ") {
		t.Fatalf("bar = %q, want the subject on the left and Submit on the right", plain)
	}
	for _, part := range []string{
		renderBrand(),
		reviewBarStyle.Render(" "),
		reviewBarStyle.Render("review the "),
		reviewBarStyle.Render("document on tmux."),
	} {
		if !strings.Contains(bar, part) {
			t.Fatalf("bar = %q, want it to contain %q", bar, part)
		}
	}
	initAdaptiveStyles(true)
	for _, tc := range []struct {
		scope  string
		staged bool
		bg     lipgloss.Style
	}{{"staged", true, diffAddedTextBg}, {"staged", false, diffAddedTextBg}, {"unstaged", false, diffDeletedTextBg}} {
		app.multiFile, app.filePath, app.staged = true, "", tc.staged
		app.source = nil
		if !tc.staged {
			app.source = &gitpkg.ReviewSource{Scope: tc.scope}
		}
		bar := app.renderReviewBar()
		if plain := ansi.Strip(bar); !strings.Contains(plain, "review the "+tc.scope+" changes on tmux.") {
			t.Fatalf("%s bar = %q, want the scope before the noun", tc.scope, plain)
		}
		if want := tc.bg.Foreground(lipgloss.BrightWhite).Render(tc.scope); !strings.Contains(bar, want) {
			t.Fatalf("%s bar = %q, want the scope styled as %q", tc.scope, bar, want)
		}
	}
	app.staged = false
	app.source = &gitpkg.ReviewSource{Scope: "range", Range: "main..HEAD"}
	if plain := ansi.Strip(app.renderReviewBar()); !strings.Contains(plain, "review the changes on tmux.") {
		t.Fatalf("range bar = %q, want no scope word", plain)
	}
	app.multiFile, app.filePath, app.source = false, "", nil
	if label := modalBtnFocusedLabel.Bold(true).Render("Submit "); !strings.Contains(bar, label) {
		t.Fatalf("bar = %q, want a bold button label %q", bar, label)
	}
	if !strings.Contains(bar, modalBtnFocusedKey.Render("q")) {
		t.Fatalf("bar = %q, want the q key styled as a button key", bar)
	}
	footer := ansi.Strip(app.renderFooter())
	if strings.Contains(footer, "Approve") || strings.Contains(footer, "Submit") {
		t.Fatalf("footer = %q, want no finish button", footer)
	}
	if label := modalBtnFocusedLabel.Bold(true).Render("Help "); !strings.Contains(app.renderFooter(), label) {
		t.Fatalf("footer = %q, want a bold Help label %q", app.renderFooter(), label)
	}

	rect, ok := app.finishButtonRect()
	if !ok || rect.top != 0 {
		t.Fatalf("submit button rect = %+v, %t; want it on the bar row", rect, ok)
	}
	assertRegionContainsRenderedText(t, app, rect, "Submit q")
	app = clickMouse(app, rect.left, rect.top)
	if app.modal != finishModal {
		t.Fatalf("modal = %v, want finish modal", app.modal)
	}

	app.host = ""
	app.ignoreWhitespace, app.hideComments = true, false
	app.multiFile = true
	app.width = 45
	app.recalculateLayout()
	rows = strings.Split(app.renderHeader(), "\n")
	plain = ansi.Strip(rows[0])
	if lipgloss.Width(rows[0]) != app.width || !strings.Contains(plain, "Ignore: WS") || !strings.Contains(plain, "…") || !strings.HasSuffix(plain, " Submit q ") {
		t.Fatalf("narrow bar = %q, want the text truncated ahead of the button", plain)
	}
}

func TestFooterHiddenOnShortTerminals(t *testing.T) {
	t.Setenv("TMUX", "/tmp/tmux-1/default,1,0")
	for _, tc := range []struct {
		height int
		shown  bool
	}{{21, false}, {22, true}, {24, true}} {
		app := setupAppWithDoc(t, "first\nsecond\n")
		app.width, app.height = 100, tc.height
		app.recalculateLayout()
		content := ansi.Strip(app.View().Content)
		rows := strings.Split(content, "\n")
		if len(rows) != tc.height {
			t.Fatalf("height %d: rendered %d rows, want the terminal filled with no spare row", tc.height, len(rows))
		}
		if shown := strings.Contains(content, "Help M-?"); shown != tc.shown {
			t.Fatalf("height %d: footer shown = %t, want %t: %q", tc.height, shown, tc.shown, content)
		}
		last := rows[len(rows)-1]
		if !tc.shown {
			if _, ok := app.footerHelpRect(); ok {
				t.Fatalf("height %d: hidden footer still has a Help button region", tc.height)
			}
			if !strings.HasSuffix(last, "╯") {
				t.Fatalf("height %d: last row = %q, want the pane's bottom border", tc.height, last)
			}
		}
	}
}

func TestDimRenderedDimsBackgrounds(t *testing.T) {
	initAdaptiveStyles(true)
	bar := reviewBarStyle.Render("bar") + diffAddedTextBg.Foreground(lipgloss.BrightWhite).Render("staged") + "plain"
	dimmed := dimRendered(bar, 14, 1)
	_, styles := renderedCells(dimmed)
	if len(styles) < 14 {
		t.Fatalf("parsed %d cells, want 14: %q", len(styles), dimmed)
	}
	for i, style := range styles[:14] {
		if !sameColor(style.Fg, color.RGBA{R: 0x55, G: 0x55, B: 0x55, A: 255}) {
			t.Fatalf("cell %d fg = %v, want dimmed gray", i, style.Fg)
		}
		hasBg := i < 9
		if (style.Bg != nil) != hasBg {
			t.Fatalf("cell %d bg = %v, want background presence %t", i, style.Bg, hasBg)
		}
		if hasBg && !sameColor(style.Bg, color.RGBA{R: 0x2a, G: 0x2a, B: 0x2a, A: 255}) {
			t.Fatalf("cell %d bg = %v, want dimmed dark gray", i, style.Bg)
		}
	}

	initAdaptiveStyles(false)
	t.Cleanup(func() { initAdaptiveStyles(true) })
	if light := dimRendered(bar, 14, 1); !strings.Contains(light, "170;170;170") || !strings.Contains(light, "221;221;221") {
		t.Fatalf("light dimming = %q, want pale gray text on a paler background", light)
	}
}

func TestLightBackgroundDarkensTextOnTerminalBackground(t *testing.T) {
	initAdaptiveStyles(false)
	t.Cleanup(func() { initAdaptiveStyles(true) })
	app := setupAppWithDoc(t, "first\nsecond\n")
	app.width, app.height, app.multiFile = 100, 24, true
	app.recalculateLayout()
	for name, style := range map[string]lipgloss.Style{
		"active tab": activeTabStyle, "help description": helpDescriptionStyle,
		"context box": contextBoxStyle, "self author": app.commentAuthorStyle(app.author),
	} {
		if got := style.GetForeground(); !sameColor(got, lipgloss.Black) {
			t.Errorf("%s foreground = %v on a light background, want black", name, got)
		}
	}
	for name, style := range map[string]lipgloss.Style{
		"inactive tab": inactiveTabStyle, "footer description": footerDescStyle, "comment": commentStyle,
	} {
		if got := style.GetForeground(); !sameColor(got, lipgloss.BrightBlack) {
			t.Errorf("%s foreground = %v on a light background, want bright black", name, got)
		}
	}
	if tab := app.renderTab(app.tabLabels(), 0, true); !strings.Contains(tab, "\x1b[1;30m") && !strings.Contains(tab, "\x1b[30m") && !strings.Contains(tab, ";30m") {
		t.Errorf("active tab = %q, want a black label", tab)
	}
	initAdaptiveStyles(true)
	if got := activeTabStyle.GetForeground(); !sameColor(got, lipgloss.BrightWhite) {
		t.Errorf("active tab foreground = %v on a dark background, want bright white", got)
	}

	app, _ = updateApp(app, tea.BackgroundColorMsg{Color: color.White})
	if got := app.modalTextarea.Styles().Focused.CursorLine.GetBackground(); !sameColor(got, lipgloss.Color("255")) {
		t.Errorf("textarea cursor line background = %v on a light background, want near white", got)
	}
	if terminalIsDark {
		t.Error("terminalIsDark is still true after a light background probe")
	}
}

func TestAccessKeyUnderline(t *testing.T) {
	resolve, _ := renderCommentHeader("", false, false, 40)
	resolved, _ := renderCommentHeader("", true, false, 40)
	for _, tt := range []struct {
		name, rendered, want string
	}{
		{"resolve", resolve, "R"},
		{"resolved", resolved, "R"},
		{"images", renderAccessKeyBarButton("Images: on"), "I"},
		{"edit dialog", renderResolution(true, "Resolved", false), ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var underlined strings.Builder
			runes, styles := renderedCells(tt.rendered)
			for i, r := range runes {
				if styles[i].Underline != uv.UnderlineNone {
					underlined.WriteRune(r)
				}
			}
			if got := underlined.String(); got != tt.want {
				t.Fatalf("underlined text = %q, want %q in %q", got, tt.want, tt.rendered)
			}
		})
	}
}

func TestReadOnlyViewer(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "notes.md")
	content := "# Notes\n\n- [ ] Confirm this change\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	dataRoot := filepath.Join(root, "state")
	app := NewApp(path, AppConfig{ReadOnly: true, Session: &review.Session{Dir: dataRoot}})
	editorMessage(&app, docRenderedMsg{})
	editorMessage(&app, tea.WindowSizeMsg{Width: 100, Height: 30})
	if app.session != nil || app.err != nil {
		t.Fatalf("read-only initialization: %v, %v", app.session, app.err)
	}
	for _, msg := range []tea.KeyPressMsg{{Code: tea.KeyEnter}, {Code: 'f'}, {Code: 'e', Mod: tea.ModAlt}, {Code: tea.KeySpace}} {
		editorMessage(&app, msg)
		if app.modal != noModal || len(app.tab().state.Comments) != 0 {
			t.Fatalf("read-only key opened editor: %v", msg)
		}
	}
	if app.toggleForm(3) {
		t.Fatal("read-only form was changed")
	}
	if err := app.SaveUnsubmittedDraft(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dataRoot); !os.IsNotExist(err) {
		t.Fatalf("session storage exists: %v", err)
	}
	saved, err := os.ReadFile(path)
	if err != nil || string(saved) != content {
		t.Fatalf("source changed: %q, %v", saved, err)
	}
	app.locationError = ""
	plain := ansi.Strip(app.View().Content)
	if !strings.Contains(plain, "Dry run") || !strings.Contains(plain, "Close q") || strings.Contains(plain, "Submit q") {
		t.Fatalf("missing read-only UI: %s", plain)
	}
	editorMessage(&app, tea.KeyPressMsg{Code: '/'})
	if !app.isearch.active {
		t.Fatal("read-only search unavailable")
	}
	editorMessage(&app, tea.KeyPressMsg{Code: tea.KeyEscape})
	rect, ok := app.finishButtonRect()
	if !ok {
		t.Fatal("Close button missing")
	}
	_, cmd := clickMouseCmd(app, rect.left, rect.top)
	if !isQuit(cmd) {
		t.Fatal("Close did not quit the viewer")
	}
	_, cmd = pressKeyCmd(app, 'q')
	if !isQuit(cmd) {
		t.Fatal("q did not quit the viewer")
	}
}
