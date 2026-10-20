package review

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// NewMemorySession keeps a disposable review entirely in this process.
func NewMemorySession() *Session {
	return &Session{CJ: NewCritJSON(), memory: map[string][]byte{}}
}

func (s *Session) InMemory() bool { return s.memory != nil }

func (s *Session) readFile(name string) ([]byte, error) {
	if s.memory != nil {
		data, ok := s.memory[name]
		if !ok {
			return nil, os.ErrNotExist
		}
		return data, nil
	}
	return os.ReadFile(filepath.Join(s.Dir, name))
}

func (s *Session) writeJSON(name string, value any) error {
	if s.memory != nil {
		data, err := json.Marshal(value)
		if err == nil {
			s.memory[name] = data
		}
		return err
	}
	if err := os.MkdirAll(s.Dir, 0o700); err != nil {
		return err
	}
	return WriteJSON(filepath.Join(s.Dir, name), value)
}

// Close removes temporary images belonging to a disposable review.
func (s *Session) Close() error {
	if s.memoryDir == "" {
		return nil
	}
	return os.RemoveAll(s.memoryDir)
}
