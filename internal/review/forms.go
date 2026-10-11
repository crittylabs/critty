package review

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/crittylabs/critty/internal/form"
	"github.com/crittylabs/critty/internal/git"
)

type ReviewInput struct {
	Scope  string                  `json:"scope"`
	Source *git.ReviewSource       `json:"source,omitempty"`
	Files  map[string]ReviewedFile `json:"files"`
	Diff   string                  `json:"diff,omitempty"`
}

type ReviewedFile struct {
	SnapshotFile
	Status string        `json:"status,omitempty"`
	Binary bool          `json:"binary,omitempty"`
	Digest string        `json:"digest,omitempty"`
	Diff   *git.DiffInfo `json:"diff,omitempty"`
}

type FormDocument struct {
	AttachedDocument
	Items   []form.Item            `json:"definitions"`
	Answers map[string]form.Answer `json:"answers"`
}

type FormState struct {
	Version   int            `json:"version"`
	Revision  string         `json:"review_revision"`
	Input     ReviewInput    `json:"reviewed_target"`
	Documents []FormDocument `json:"attachments"`
}

// FormResult is self-contained, including the exact inputs to each consent.
type FormResult struct {
	FormState
	Session  string                     `json:"session,omitempty"`
	Approved bool                       `json:"approved"`
	Round    int                        `json:"round"`
	Consents map[string]map[string]bool `json:"consents,omitempty"`
}

func ParseDocumentForm(d AttachedDocument) ([]form.Item, error) {
	if d.Removed || !strings.EqualFold(filepath.Ext(d.Source), ".md") && !strings.EqualFold(filepath.Ext(d.Source), ".markdown") {
		return nil, nil
	}
	items, err := form.Parse(d.Content)
	if err != nil {
		return nil, fmt.Errorf("attachment %s: %w", d.Name, err)
	}
	return items, nil
}

func (s *Session) LoadForms() (FormState, error) {
	var state FormState
	data, err := os.ReadFile(filepath.Join(s.Dir, "forms.json"))
	if os.IsNotExist(err) {
		return state, nil
	}
	if err != nil {
		return state, err
	}
	if err := json.Unmarshal(data, &state); err != nil {
		return state, err
	}
	if state.Version != 1 {
		return state, fmt.Errorf("unsupported form version: %d", state.Version)
	}
	return state, nil
}

func (s *Session) BeginForms(input ReviewInput, docs []AttachedDocument) (FormState, error) {
	old, err := s.LoadForms()
	if err != nil {
		return FormState{}, err
	}
	state := FormState{Version: 1, Input: input, Documents: []FormDocument{}}
	var active []AttachedDocument
	for _, d := range docs {
		if d.Removed {
			continue
		}
		active = append(active, d)
		items, err := ParseDocumentForm(d)
		if err != nil {
			return state, err
		}
		state.Documents = append(state.Documents, FormDocument{AttachedDocument: d, Items: items, Answers: form.Defaults(items)})
	}
	data, err := json.Marshal(struct {
		Input ReviewInput
		Docs  []AttachedDocument
	}{input, active})
	if err != nil {
		return state, err
	}
	state.Revision = fmt.Sprintf("sha256:%x", sha256.Sum256(data))
	for i := range state.Documents {
		d := &state.Documents[i]
		for _, prev := range old.Documents {
			if prev.ID != d.ID {
				continue
			}
			for _, item := range d.Items {
				if !item.Explicit && prev.Revision != d.Revision || (item.Required || item.Consent) && old.Revision != state.Revision {
					continue
				}
				current, err := form.Fingerprint(d.Items, item.ID)
				if err != nil {
					return state, err
				}
				previous, err := form.Fingerprint(prev.Items, item.ID)
				if err != nil {
					return state, err
				}
				if current != previous {
					continue
				}
				if answer, ok := prev.Answers[item.ID]; ok {
					d.Answers[item.ID] = answer
				}
			}
		}
	}
	return state, s.SaveForms(state)
}

func (s *Session) SaveForms(state FormState) error {
	if err := os.MkdirAll(s.Dir, 0o700); err != nil {
		return err
	}
	return WriteJSON(filepath.Join(s.Dir, "forms.json"), state)
}

// WriteJSON replaces a result only after its complete contents are written.
func WriteJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".critty-result-*.tmp")
	if err != nil {
		return err
	}
	defer func() {
		if err := os.Remove(f.Name()); err != nil && !os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "critty: removing temporary result: %v\n", err)
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
	return os.Rename(f.Name(), path)
}

func (state *FormState) Document(path string) *FormDocument {
	if state == nil {
		return nil
	}
	for i := range state.Documents {
		if state.Documents[i].Path() == path {
			return &state.Documents[i]
		}
	}
	return nil
}

func (state FormState) HasForms() bool {
	for _, d := range state.Documents {
		if len(d.Items) > 0 {
			return true
		}
	}
	return false
}

func (state FormState) Missing() []string {
	var missing []string
	for _, d := range state.Documents {
		seen := map[string]bool{}
		for _, item := range d.Items {
			if item.Required && !seen[item.ID] && !form.Satisfied(item, d.Answers[item.ID]) {
				missing = append(missing, d.Name+": "+item.ID)
				seen[item.ID] = true
			}
		}
	}
	return missing
}

func (state FormState) Result(approved bool, round int) FormResult {
	approved = approved && len(state.Missing()) == 0
	result := FormResult{FormState: state, Approved: approved, Round: round}
	if approved {
		result.Consents = map[string]map[string]bool{}
		for _, d := range state.Documents {
			for _, item := range d.Items {
				if !item.Consent {
					continue
				}
				if result.Consents[d.ID] == nil {
					result.Consents[d.ID] = map[string]bool{}
				}
				result.Consents[d.ID][item.ID] = form.Satisfied(item, d.Answers[item.ID])
			}
		}
	}
	return result
}

func (s *Session) SaveFormResult(result FormResult) error {
	// Keep each completed round even when a later round replaces the drafts.
	return WriteJSON(filepath.Join(s.Dir, fmt.Sprintf("result-%d.json", result.Round)), result)
}
