package tui

import (
	"time"

	tea "charm.land/bubbletea/v2"
)

const noticeDuration = 2 * time.Second

type footerNotice struct {
	text       string
	generation uint64
}

type noticeExpiredMsg uint64

func (m *AppModel) flashNotice(text string) tea.Cmd {
	m.notice.text = text
	m.notice.generation++
	generation := m.notice.generation
	m.relayoutNotice()
	return tea.Tick(noticeDuration, func(time.Time) tea.Msg { return noticeExpiredMsg(generation) })
}

func (m *AppModel) expireNotice(msg noticeExpiredMsg) {
	if uint64(msg) != m.notice.generation {
		return
	}
	m.notice.text = ""
	m.relayoutNotice()
}

func (m *AppModel) relayoutNotice() {
	attention := m.captureDiffAttention()
	offset := m.contentViewport.YOffset()
	m.recalculateLayout()
	m.rebuildContent()
	m.contentViewport.SetYOffset(offset)
	m.restoreDiffAttention(attention)
}
