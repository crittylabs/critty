package tui

import "strings"

// sourceLineCursor scans each prefix once when offsets arrive in source order.
type sourceLineCursor struct {
	text                string
	offset, line, start int
}

// position returns a one-based line and zero-based byte column.  Earlier
// offsets restart the scan, so callers need not guarantee traversal order.
// The offset must be between zero and len(s.text), inclusive.
func (s *sourceLineCursor) position(offset int) (line, column int) {
	if s.line == 0 || offset < s.offset {
		s.offset, s.line, s.start = 0, 1, 0
	}
	prefix := s.text[s.offset:offset]
	if count := strings.Count(prefix, "\n"); count > 0 {
		s.line += count
		s.start = s.offset + strings.LastIndexByte(prefix, '\n') + 1
	}
	s.offset = offset
	return s.line, offset - s.start
}
