package renderfont

import (
	"context"
	"fmt"
	"slices"
	"testing"
)

func TestFontSelection(t *testing.T) {
	var patterns []string
	s := &Selector{path: "fc-match", query: func(_ context.Context, path, format, pattern string) ([]byte, error) {
		patterns = append(patterns, pattern)
		if format == "%{family[0]}" {
			return []byte("Sample Sans"), nil
		}
		return []byte("/fonts/sans.ttf\n"), nil
	}}
	if fonts := s.Files(context.Background(), "A\u03bbA"); len(fonts) != 1 || fonts[0] != "/fonts/sans.ttf" {
		t.Fatal(fonts)
	}
	if !slices.Equal(patterns, []string{"sans-serif", ":charset=3bb 41"}) {
		t.Fatal(patterns)
	}
	if family := s.Family(context.Background(), "A"); family != "Sample Sans" {
		t.Fatal(family)
	}
	for _, value := range []string{"relative.ttf", "/fonts/a.ttf\n/fonts/b.ttf", ""} {
		s.query = func(context.Context, string, string, string) ([]byte, error) { return []byte(value), nil }
		if fonts := s.Files(context.Background(), "A"); len(fonts) != 0 {
			t.Fatal(fonts)
		}
	}
	s.query = func(context.Context, string, string, string) ([]byte, error) { return nil, fmt.Errorf("failed") }
	if fonts := s.Files(context.Background(), "A"); len(fonts) != 0 {
		t.Fatal(fonts)
	}
	if family := s.Family(context.Background(), "A"); family != "" {
		t.Fatal(family)
	}
	t.Setenv("PATH", t.TempDir())
	if New().Available() {
		t.Fatal("unexpected Fontconfig")
	}
	var absent *Selector
	if absent.Available() || len(absent.Files(context.Background(), "A")) != 0 {
		t.Fatal("nil selector")
	}
}
