package document

import (
	"crypto/sha256"
	"fmt"
	"os"
	"strings"
)

type Document struct {
	Path    string
	Content string
	Lines   []string
	Hash    string
	Known   map[int]bool // nil for complete documents
}

func (d *Document) HasLine(n int) bool {
	return n >= 1 && n <= len(d.Lines) && (d.Known == nil || d.Known[n])
}

func Load(path string) (*Document, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading document: %w", err)
	}

	return FromContent(path, data), nil
}

// FromContent builds a document from content supplied by another source.
func FromContent(path string, data []byte) *Document {
	content := string(data)
	hash := fmt.Sprintf("%x", sha256.Sum256(data))
	lines := strings.Split(content, "\n")

	return &Document{
		Path:    path,
		Content: content,
		Lines:   lines,
		Hash:    hash,
	}
}

func (d *Document) LineAt(n int) string {
	if n < 1 || n > len(d.Lines) {
		return ""
	}
	return d.Lines[n-1]
}

func (d *Document) LineCount() int {
	return len(d.Lines)
}

// SourceLineCount excludes the cursor row after a final newline and the
// sole cursor row of an empty document.
func (d *Document) SourceLineCount() int {
	count := d.LineCount()
	if count > 0 && d.Known == nil && d.Lines[count-1] == "" &&
		(d.Content == "" || strings.HasSuffix(d.Content, "\n")) {
		return count - 1
	}
	return count
}
