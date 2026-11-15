package tui

import "github.com/crittylabs/critty/v2/internal/document"

type documentsLoadedMsg struct {
	documents map[string]*document.Document
}

type errMsg struct {
	err error
}
