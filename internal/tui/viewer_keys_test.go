package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestViewerSelectionAndLineKeys(t *testing.T) {
	for _, readOnly := range []bool{false, true} {
		for _, toggle := range []tea.KeyPressMsg{
			{Code: tea.KeySpace, Mod: tea.ModCtrl},
			{Code: '@', Mod: tea.ModCtrl},
			{Code: '@', BaseCode: '2', Mod: tea.ModCtrl | tea.ModShift},
		} {
			t.Run(fmt.Sprintf("view=%v/%s", readOnly, toggle.String()), func(t *testing.T) {
				app := setupPrivateAppWithDoc(t, "first\n**second**\nthird\n")
				app.readOnly = readOnly
				app.tab().cursorLine = 1
				app, _ = updateApp(app, tea.WindowSizeMsg{Width: 100, Height: 30})
				app = ctrlKey(app, 'n')
				if app.tab().cursorLine != 2 {
					t.Fatal("C-n did not move to the next line")
				}
				app, _ = updateApp(app, toggle)
				if !app.tab().selecting || app.tab().selectAnchor != 2 {
					t.Fatal("selection toggle did not anchor the current line")
				}
				app = ctrlKey(app, 'n')
				app = ctrlKey(app, 'p')
				if app.tab().cursorLine != 2 || !app.tab().selecting {
					t.Fatal("C-p did not retain line selection")
				}
				app = ctrlKey(app, 'n')
				app, cmd := pressKeyCmd(app, 'Y')
				assertCopiedText(t, app, cmd, "**second**\nthird")
				app, _ = updateApp(app, toggle)
				if app.tab().selecting || app.tab().cursorLine != 3 {
					t.Fatal("selection toggle did not retain the cursor when cancelling")
				}
			})
		}
	}
}

func TestViewerHorizontalKeys(t *testing.T) {
	content := "| ID | Description | Notes |\n| --- | --- | --- |\n| 1 | " + strings.Repeat("word ", 20) + " | " + strings.Repeat("note ", 20) + " |"
	for _, readOnly := range []bool{false, true} {
		t.Run(fmt.Sprint(readOnly), func(t *testing.T) {
			app := setupPrivateAppWithDoc(t, content+"\n\n"+content)
			app.readOnly, app.viewMode = readOnly, true
			app, _ = updateApp(app, tea.WindowSizeMsg{Width: 70, Height: 40})
			app.tab().cursorLine = 3
			block := app.horizontalBlockAtLine(3)
			if block == nil || block.window.maxOffset() == 0 {
				t.Fatal("fixture has no horizontal overflow")
			}
			step := min(block.window.maxOffset(), max(1, block.window.width/2))
			app = ctrlKey(app, 'f')
			if app.horizontalBlockAtLine(3).window.offset != step {
				t.Fatal("C-f did not use the normal horizontal scroll distance")
			}
			app = ctrlKey(app, 'b')
			if app.horizontalBlockAtLine(3).window.offset != 0 {
				t.Fatal("C-b did not return to the left edge")
			}
			for _, pair := range [][2]tea.KeyPressMsg{
				{{Code: 'e', Mod: tea.ModCtrl}, {Code: 'a', Mod: tea.ModCtrl}},
				{{Code: '$'}, {Code: '^'}},
			} {
				app, _ = updateApp(app, pair[0])
				block = app.horizontalBlockAtLine(3)
				if block.window.offset != block.window.maxOffset() {
					t.Fatalf("%s did not reach the right edge", pair[0])
				}
				app, _ = updateApp(app, pair[1])
				if app.horizontalBlockAtLine(3).window.offset != 0 {
					t.Fatalf("%s did not reach the left edge", pair[1])
				}
			}
			if app.tab().cursorLine != 3 || app.contentViewport.YOffset() != 0 || app.horizontalBlockAtLine(7).window.offset != 0 {
				t.Fatal("horizontal keys changed the cursor, vertical scroll, or another table")
			}
		})
	}
}

func TestViewerKeysPreserveInputEditing(t *testing.T) {
	app := setupPrivateAppWithDoc(t, "first\nsecond\nthird")
	app.tab().cursorLine = 2
	app = pressKey(app, 'f')
	app.modalTextarea.SetValue("alpha beta")
	app = ctrlKey(app, 'a')
	app = ctrlKey(app, 'f')
	app = ctrlKey(app, 'b')
	app, _ = updateApp(app, tea.KeyPressMsg{Code: 'Y', Text: "Y"})
	app = ctrlKey(app, 'e')
	app, _ = updateApp(app, tea.KeyPressMsg{Code: '$', Text: "$"})
	if app.modalTextarea.Value() != "Yalpha beta$" || app.tab().cursorLine != 2 || app.tab().selecting || len(app.killRing.entries) != 0 {
		t.Fatal("viewer keys intercepted textarea editing")
	}
	app.modal = noModal
	app.startISearch(true)
	app.startISearchEdit()
	app.isearchInput.SetValue("alpha")
	app = ctrlKey(app, 'a')
	app = ctrlKey(app, 'f')
	app = ctrlKey(app, 'b')
	app, _ = updateApp(app, tea.KeyPressMsg{Code: 'Y', Text: "Y"})
	app = ctrlKey(app, 'e')
	app, _ = updateApp(app, tea.KeyPressMsg{Code: '^', Text: "^"})
	if app.isearchInput.Value() != "Yalpha^" || len(app.killRing.entries) != 0 {
		t.Fatal("viewer keys intercepted line-editor input")
	}
}
