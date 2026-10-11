package review

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

// AttachedDocument keeps the source identity separate from its comment path.
type AttachedDocument struct {
	ID       string `json:"id"`
	Source   string `json:"source"`
	Name     string `json:"name"`
	Content  string `json:"content"`
	Revision string `json:"revision"`
	Removed  bool   `json:"removed,omitempty"`
}

func (d AttachedDocument) Path() string {
	return "attachments/" + d.ID + "/" + filepath.Base(d.Source)
}

type DocumentAttachments struct {
	Version   int                `json:"version"`
	Documents []AttachedDocument `json:"documents"`
}

// ReadAttachedDocuments validates all inputs before changing session state.
func ReadAttachedDocuments(paths []string) ([]AttachedDocument, error) {
	docs := make([]AttachedDocument, 0, len(paths))
	seen := map[string]bool{}
	for _, path := range paths {
		abs, err := filepath.Abs(path)
		if err != nil {
			return nil, err
		}
		abs, err = filepath.EvalSymlinks(abs)
		if err != nil {
			return nil, fmt.Errorf("attachment %s: %w", path, err)
		}
		if seen[abs] {
			return nil, fmt.Errorf("duplicate attachment: %s", path)
		}
		seen[abs] = true
		info, err := os.Stat(abs)
		if err != nil {
			return nil, err
		}
		if !info.Mode().IsRegular() {
			return nil, fmt.Errorf("attachment is not a regular file: %s", path)
		}
		data, err := os.ReadFile(abs)
		if err != nil {
			return nil, err
		}
		if !utf8.Valid(data) || strings.ContainsRune(string(data), '\x00') {
			return nil, fmt.Errorf("attachment is not UTF-8 text: %s", path)
		}
		identity := sha256.Sum256([]byte(abs))
		revision := sha256.Sum256(data)
		docs = append(docs, AttachedDocument{ID: fmt.Sprintf("a_%x", identity), Source: abs,
			Content: string(data), Revision: fmt.Sprintf("sha256:%x", revision)})
	}
	setDocumentNames(docs)
	for _, d := range docs {
		if _, err := ParseDocumentForm(d); err != nil {
			return nil, err
		}
	}
	return docs, nil
}

func setDocumentNames(docs []AttachedDocument) {
	depths := make([]int, len(docs))
	parts := make([][]string, len(docs))
	for i, d := range docs {
		parts[i] = strings.Split(filepath.ToSlash(d.Source), "/")
		depths[i] = 1
	}
	for {
		counts := map[string]int{}
		for i := range docs {
			docs[i].Name = strings.Join(parts[i][len(parts[i])-depths[i]:], "/")
			counts[docs[i].Name]++
		}
		changed := false
		for i, d := range docs {
			if counts[d.Name] > 1 && depths[i] < len(parts[i]) {
				depths[i]++
				changed = true
			}
		}
		if !changed {
			return
		}
	}
}

func (s *Session) LoadDocuments() (DocumentAttachments, error) {
	var attachments DocumentAttachments
	data, err := os.ReadFile(filepath.Join(s.Dir, "documents.json"))
	if os.IsNotExist(err) {
		return attachments, nil
	}
	if err != nil {
		return attachments, err
	}
	if err := json.Unmarshal(data, &attachments); err != nil {
		return attachments, fmt.Errorf("reading document attachments: %w", err)
	}
	if attachments.Version != 1 {
		return attachments, fmt.Errorf("unsupported document attachment version: %d", attachments.Version)
	}
	return attachments, nil
}

// ReplaceDocuments retains removed snapshots so their comments remain readable.
// Callers hold the session run lock while replacing review inputs.
func (s *Session) ReplaceDocuments(docs []AttachedDocument) error {
	old, err := s.LoadDocuments()
	if err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, d := range docs {
		seen[d.ID] = true
	}
	for _, d := range old.Documents {
		if !seen[d.ID] {
			d.Removed = true
			docs = append(docs, d)
		}
	}
	data, err := json.MarshalIndent(DocumentAttachments{Version: 1, Documents: docs}, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(s.Dir, 0o700); err != nil {
		return err
	}
	f, err := os.CreateTemp(s.Dir, "documents-*.tmp")
	if err != nil {
		return err
	}
	defer func() {
		if err := os.Remove(f.Name()); err != nil && !os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "critty: removing temporary document snapshot: %v\n", err)
		}
	}()
	_, writeErr := f.Write(append(data, '\n'))
	closeErr := f.Close()
	if writeErr != nil {
		return writeErr
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(f.Name(), filepath.Join(s.Dir, "documents.json"))
}
