package review

import (
	"os"
	"strings"
	"testing"
)

func TestSlugify(t *testing.T) {
	tests := []struct{ in, want string }{
		{"My Great Plan!", "my-great-plan"},
		{"  spaced   out  ", "spaced-out"},
		{"日本語のみ", ""},
	}
	for _, tt := range tests {
		if got := Slugify(tt.in); got != tt.want {
			t.Errorf("Slugify(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestResolveSlugFromHeading(t *testing.T) {
	slug := ResolveSlug([]byte("intro\n\n# Auth Refactor Plan\n\nbody\n"))
	if !strings.HasPrefix(slug, "auth-refactor-plan-") {
		t.Errorf("slug = %q", slug)
	}
	fallback := ResolveSlug([]byte("no heading at all\n"))
	if !strings.HasPrefix(fallback, "plan-") {
		t.Errorf("fallback slug = %q", fallback)
	}
}

func TestSavePlanVersion(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	v1, err := SavePlanVersionAt(PlanStorageDir("my-plan"), []byte("first\n"))
	if err != nil || v1 != 1 {
		t.Fatalf("v1 = %d, err = %v", v1, err)
	}
	v2, err := SavePlanVersionAt(PlanStorageDir("my-plan"), []byte("second\n"))
	if err != nil || v2 != 2 {
		t.Fatalf("v2 = %d, err = %v", v2, err)
	}

	current, err := os.ReadFile(PlanCurrentPath("my-plan"))
	if err != nil || string(current) != "second\n" {
		t.Errorf("current.md = %q, err = %v", current, err)
	}
	first, err := os.ReadFile(PlanStorageDir("my-plan") + "/v001.md")
	if err != nil || string(first) != "first\n" {
		t.Errorf("v001.md = %q, err = %v", first, err)
	}
}

func TestOpenPlanSessionKeyStability(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Chdir(t.TempDir())

	a, err := OpenPlanSession("my-plan")
	if err != nil {
		t.Fatal(err)
	}
	b, err := OpenPlanSession("my-plan")
	if err != nil {
		t.Fatal(err)
	}
	if a.Key != b.Key {
		t.Errorf("keys differ: %s vs %s", a.Key, b.Key)
	}
	if !strings.Contains(a.Dir, "plans/my-plan/.crit") {
		t.Errorf("unexpected review dir: %s", a.Dir)
	}
}
