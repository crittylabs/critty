package review

import (
	"slices"
	"strings"
	"testing"
)

func listFixture() CritJSON {
	cj := NewCritJSON()
	cj.ReviewComments = []Comment{
		{ID: "r_1", Scope: "review", Body: "overall"},
		{ID: "r_2", Scope: "review", Body: "resolved overall", Resolved: true},
	}
	cj.Files["b.go"] = CritJSONFile{Comments: []Comment{
		{ID: "c_b9", StartLine: 9, EndLine: 9, Body: "late line"},
		{ID: "c_bf", Scope: "file", Body: "file-level"},
		{ID: "c_b2", StartLine: 2, EndLine: 4, Body: "early range", Drifted: true},
	}}
	cj.Files["a.go"] = CritJSONFile{Comments: []Comment{
		{ID: "c_a5", StartLine: 5, EndLine: 5, Body: "a comment", Resolved: true,
			Replies: []Reply{{ID: "rp_1", Author: "AI", Body: "fixed"}}},
	}}
	return cj
}

func TestListCommentsOrdering(t *testing.T) {
	cj := listFixture()
	entries := cj.ListComments(false)
	var ids []string
	for _, e := range entries {
		ids = append(ids, e.ID)
	}
	want := []string{"r_1", "r_2", "c_a5", "c_bf", "c_b2", "c_b9"}
	if !slices.Equal(ids, want) {
		t.Errorf("order = %v, want %v", ids, want)
	}

	unresolved := cj.ListComments(true)
	if len(unresolved) != 4 {
		t.Errorf("unresolved count = %d, want 4", len(unresolved))
	}
}

func TestFormatCommentsText(t *testing.T) {
	cj := listFixture()
	text := FormatCommentsText(cj.ListComments(false), false)

	for _, want := range []string{
		"6 comments:",
		"[r_1] review",
		"[c_b2] line b.go:2-4 (drifted)",
		"[c_b9] line b.go:9",
		"[c_bf] file b.go",
		"  body:   overall",
		"  replies:",
		"    - [rp_1] AI: fixed",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("output missing %q:\n%s", want, text)
		}
	}

	if got := FormatCommentsText(nil, true); got != "No unresolved comments." {
		t.Errorf("empty unresolved = %q", got)
	}
	if got := FormatCommentsText(nil, false); got != "No comments." {
		t.Errorf("empty all = %q", got)
	}
}

func TestEncodeCommentsJSONNormalizesNil(t *testing.T) {
	data, err := EncodeCommentsJSON(nil)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(data)) != "[]" {
		t.Errorf("nil should encode as [], got %s", data)
	}
}
