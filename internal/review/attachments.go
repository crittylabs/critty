package review

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
)

const MaxAttachmentBytes = 5 << 20

// SaveAttachment stores an image beside review.json and returns its Markdown path.
func (s *Session) SaveAttachment(data []byte) (string, error) {
	if len(data) == 0 || len(data) > MaxAttachmentBytes {
		return "", fmt.Errorf("image must be between 1 byte and 5 MiB")
	}
	ext := map[string]string{
		"image/png": ".png", "image/jpeg": ".jpg", "image/gif": ".gif", "image/webp": ".webp",
	}[http.DetectContentType(data)]
	if ext == "" {
		return "", fmt.Errorf("unsupported image type")
	}
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return "", err
	}
	id[6] = (id[6] & 0x0f) | 0x40
	id[8] = (id[8] & 0x3f) | 0x80
	name := fmt.Sprintf("%x-%x-%x-%x-%x%s", id[:4], id[4:6], id[6:8], id[8:10], id[10:], ext)
	return s.saveAttachment(data, name)
}

func (s *Session) saveAttachment(data []byte, name string) (string, error) {
	if s.Dir == "" && !s.InMemory() {
		return "", fmt.Errorf("no review directory")
	}
	if s.InMemory() && s.Dir == "" {
		dir, err := os.MkdirTemp("", "critty-demo-*")
		if err != nil {
			return "", err
		}
		s.Dir, s.memoryDir = dir, dir
	}
	dir := filepath.Join(s.Dir, "attachments")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	f, err := os.OpenFile(filepath.Join(dir, name), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if os.IsExist(err) {
		existing, readErr := os.ReadFile(filepath.Join(dir, name))
		if readErr == nil && bytes.Equal(existing, data) {
			return "attachments/" + name, nil
		}
	}
	if err != nil {
		return "", err
	}
	_, writeErr := f.Write(data)
	closeErr := f.Close()
	if writeErr != nil || closeErr != nil {
		_ = os.Remove(f.Name())
		if writeErr != nil {
			return "", writeErr
		}
		return "", closeErr
	}
	return "attachments/" + filepath.Base(f.Name()), nil
}

// HasAttachments errs on the side of retaining images when the directory cannot
// be read.  Approval cleanup must not remove an unread attachment.
func (s *Session) HasAttachments() bool {
	entries, err := os.ReadDir(filepath.Join(s.Dir, "attachments"))
	return len(entries) > 0 || (err != nil && !os.IsNotExist(err))
}
