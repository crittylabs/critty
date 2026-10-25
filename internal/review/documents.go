package review

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/crittylabs/critty/internal/imageinfo"
)

// AttachedDocument keeps the source identity separate from its comment path.
type AttachedDocument struct {
	ID        string `json:"id"`
	Source    string `json:"source"`
	Name      string `json:"name"`
	Content   string `json:"content"`
	Revision  string `json:"revision"`
	Removed   bool   `json:"removed,omitempty"`
	Image     string `json:"image,omitempty"`
	imageData []byte
}

// ImagePath resolves a saved image snapshot, never its mutable source file.
func (d AttachedDocument) ImagePath(base string) (string, error) {
	if d.Image == "" || filepath.ToSlash(filepath.Dir(d.Image)) != "attachments" || filepath.Base(d.Image) == "." {
		return "", fmt.Errorf("invalid image snapshot: %s", d.Image)
	}
	return filepath.Join(base, filepath.FromSlash(d.Image)), nil
}

func (d AttachedDocument) ReadImage(base string) ([]byte, error) {
	if d.imageData != nil {
		return d.imageData, nil
	}
	path, err := d.ImagePath(base)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > MaxAttachmentBytes {
		return nil, fmt.Errorf("invalid image snapshot: %s", d.Image)
	}
	return os.ReadFile(path)
}

func (d AttachedDocument) Path() string {
	return "attachments/" + d.ID + "/" + filepath.Base(d.Source)
}

type DocumentAttachments struct {
	Version   int                `json:"version"`
	Documents []AttachedDocument `json:"documents"`
	// Answers labels submission for rounds that collect form answers.
	Answers bool `json:"answers,omitempty"`
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
		if imageinfo.SupportedPath(abs) && info.Size() > MaxAttachmentBytes {
			return nil, fmt.Errorf("image attachment exceeds 5 MiB: %s", path)
		}
		data, err := os.ReadFile(abs)
		if err != nil {
			return nil, err
		}
		identity := sha256.Sum256([]byte(abs))
		revision := sha256.Sum256(data)
		doc := AttachedDocument{ID: fmt.Sprintf("a_%x", identity), Source: abs,
			Content: string(data), Revision: fmt.Sprintf("sha256:%x", revision)}
		if imageinfo.SupportedPath(abs) {
			meta, err := imageinfo.Read(data)
			if err != nil {
				return nil, fmt.Errorf("image attachment %s: %w", path, err)
			}
			if meta.Width <= 0 || meta.Height <= 0 || meta.Width > imageinfo.MaxPixels/meta.Height {
				return nil, fmt.Errorf("image attachment exceeds 16 megapixels: %s", path)
			}
			ext := strings.ToLower(meta.Format)
			if ext == "jpeg" {
				ext = "jpg"
			}
			doc.Image = fmt.Sprintf("attachments/%x.%s", revision, ext)
			doc.Content, doc.imageData = meta.String(), data
		} else if !utf8.Valid(data) || strings.ContainsRune(string(data), '\x00') {
			return nil, fmt.Errorf("attachment is not UTF-8 text: %s", path)
		}
		docs = append(docs, doc)
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
	data, err := s.readFile("documents.json")
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
	for _, d := range docs {
		if d.imageData != nil {
			if _, err := d.ImagePath(s.Dir); err != nil {
				return err
			}
			if _, err := s.saveAttachment(d.imageData, filepath.Base(d.Image)); err != nil {
				return err
			}
		}
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
	return s.saveDocuments(DocumentAttachments{Version: 1, Documents: docs, Answers: old.Answers})
}

// SetAnswerMode records whether later rounds submit feedback as answers.
func (s *Session) SetAnswerMode(on bool) error {
	attachments, err := s.LoadDocuments()
	if err != nil {
		return err
	}
	attachments.Version, attachments.Answers = 1, on
	if attachments.Documents == nil {
		attachments.Documents = []AttachedDocument{}
	}
	return s.saveDocuments(attachments)
}

func (s *Session) saveDocuments(attachments DocumentAttachments) error {
	return s.writeJSON("documents.json", attachments)
}
