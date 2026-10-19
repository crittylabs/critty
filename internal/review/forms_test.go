package review

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/crittylabs/critty/internal/form"
)

func TestFormDraftsAndInvalidation(t *testing.T) {
	s, err := OpenSession(t.TempDir(), "forms")
	if err != nil {
		t.Fatal(err)
	}
	docs := []AttachedDocument{{ID: "a", Source: "/form.md", Name: "form.md", Content: "- [ ] Note <!-- form: id=note -->\n- [x] Confirm <!-- form: id=confirm required -->\n- [x] Push <!-- form: id=push consent -->\n", Revision: "one"}}
	input := ReviewInput{Scope: "document", Files: map[string]ReviewedFile{"target.md": {SnapshotFile: SnapshotFile{Content: "one"}}}}
	state, err := s.BeginForms(input, docs)
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Missing()) != 1 || state.Documents[0].Answers["push"].Value != false {
		t.Fatalf("source granted permission: %+v", state)
	}
	for _, id := range []string{"note", "confirm", "push"} {
		state.Documents[0].Answers[id] = form.Answer{Value: true, Human: true, Author: "Human", Revision: state.Revision, Round: 1}
	}
	if err := s.SaveForms(state); err != nil {
		t.Fatal(err)
	}
	resumed, err := s.BeginForms(input, docs)
	if err != nil {
		t.Fatal(err)
	}
	if len(resumed.Missing()) != 0 || !resumed.Documents[0].Answers["push"].Human {
		t.Fatal("lost unchanged drafts")
	}
	input.Files["target.md"] = ReviewedFile{SnapshotFile: SnapshotFile{Content: "two"}}
	changed, err := s.BeginForms(input, docs)
	if err != nil {
		t.Fatal(err)
	}
	if changed.Revision == state.Revision || len(changed.Missing()) != 1 || changed.Documents[0].Answers["push"].Human || !changed.Documents[0].Answers["note"].Human {
		t.Fatalf("incorrect invalidation: %+v", changed)
	}
	for _, mutation := range []func([]AttachedDocument) []AttachedDocument{
		func(d []AttachedDocument) []AttachedDocument {
			return append(d, AttachedDocument{ID: "b", Source: "/second.txt", Name: "second.txt", Content: "context"})
		},
		func(d []AttachedDocument) []AttachedDocument { d[0].Content += "\n"; return d },
	} {
		if err := s.SaveForms(state); err != nil {
			t.Fatal(err)
		}
		updated, err := s.BeginForms(state.Input, mutation(append([]AttachedDocument(nil), docs...)))
		if err != nil {
			t.Fatal(err)
		}
		if len(updated.Missing()) != 1 || updated.Documents[0].Answers["push"].Human {
			t.Fatal("attachment change retained confirmation")
		}
	}
	if err := s.SaveForms(state); err != nil {
		t.Fatal(err)
	}
	docs[0].Content = strings.ReplaceAll(docs[0].Content, "Note", "Changed")
	docs[0].Revision = "two"
	changed, err = s.BeginForms(state.Input, docs)
	if err != nil {
		t.Fatal(err)
	}
	if changed.Documents[0].Answers["note"].Human {
		t.Fatal("changed definition retained answer")
	}
	docs[0].Removed = true
	removed, err := s.BeginForms(state.Input, docs)
	if err != nil || len(removed.Documents) != 0 {
		t.Fatalf("removed form retained: %+v %v", removed, err)
	}
}

func TestSelfContainedFormResults(t *testing.T) {
	s, err := OpenSession(t.TempDir(), "result")
	if err != nil {
		t.Fatal(err)
	}
	docs := []AttachedDocument{{ID: "a", Source: "/consent.md", Name: "consent.md", Content: "- [ ] Publish target <!-- form: id=publish consent -->"}}
	state, err := s.BeginForms(ReviewInput{Files: map[string]ReviewedFile{"target": {SnapshotFile: SnapshotFile{Content: "exact target"}}}}, docs)
	if err != nil {
		t.Fatal(err)
	}
	state.Documents[0].Answers["publish"] = form.Answer{Value: true, Human: true, Author: "Human", Revision: state.Revision, Round: 1}
	for _, approved := range []bool{false, true} {
		result := state.Result(approved, 1)
		if approved && !result.Consents["a"]["publish"] || !approved && len(result.Consents) != 0 {
			t.Fatalf("wrong consent result: %+v", result)
		}
		path := filepath.Join(t.TempDir(), "result.json")
		if err := WriteJSON(path, result); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var read FormResult
		if err := json.Unmarshal(data, &read); err != nil {
			t.Fatal(err)
		}
		if read.Input.Files["target"].Content != "exact target" || read.Documents[0].Content != docs[0].Content || read.Documents[0].Items[0].Label != "Publish target" {
			t.Fatal("result requires deleted session data")
		}
	}
}

func TestSelectAllFormVersion(t *testing.T) {
	s, err := OpenSession(t.TempDir(), "forms")
	if err != nil {
		t.Fatal(err)
	}
	input := ReviewInput{Scope: "document", Files: map[string]ReviewedFile{}}
	plain := []AttachedDocument{{ID: "a", Source: "/form.md", Name: "form.md", Content: "- [ ] Note <!-- form: id=note -->\n", Revision: "one"}}
	state, err := s.BeginForms(input, plain)
	if err != nil {
		t.Fatal(err)
	}
	if state.Version != 1 {
		t.Fatalf("plain form version: %d", state.Version)
	}
	tagged := []AttachedDocument{{ID: "a", Source: "/form.md", Name: "form.md", Content: "- [ ] All <!-- form: id=all select-all=t -->\n- [ ] Note <!-- form: id=note tags=t -->\n", Revision: "two"}}
	state, err = s.BeginForms(input, tagged)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := state.Documents[0].Answers["all"]; state.Version != 2 || ok {
		t.Fatalf("select-all state: %+v", state)
	}
	if loaded, err := s.LoadForms(); err != nil || loaded.Version != 2 {
		t.Fatalf("reload: %v %+v", err, loaded)
	}
}

func TestFormResultOmitsDraftNotes(t *testing.T) {
	items, err := form.Parse("- [ ] Other <!-- form: id=q value=other text -->\n- [ ] Plain <!-- form: id=q value=plain -->\n")
	if err != nil {
		t.Fatal(err)
	}
	answers := map[string]form.Answer{"q": {Value: "plain", Text: "draft", Human: true}}
	state := FormState{Version: 2, Documents: []FormDocument{{Items: items, Answers: answers}}}
	if text := state.Result(false, 1).Documents[0].Answers["q"].Text; text != "" {
		t.Fatalf("exported draft note %q", text)
	}
	if answers["q"].Text != "draft" {
		t.Fatal("result changed the draft")
	}
	answers["q"] = form.Answer{Value: "other", Text: "note", Human: true}
	if text := state.Result(false, 1).Documents[0].Answers["q"].Text; text != "note" {
		t.Fatalf("lost selected note: %q", text)
	}
}
