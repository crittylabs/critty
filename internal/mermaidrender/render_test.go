package mermaidrender

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestRenderFallbackAndCache(t *testing.T) {
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, image.NewNRGBA(image.Rect(0, 0, 40, 20))); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, primary, fallback string
		wantError               bool
		calls                   []string
	}{
		{"primary", "valid", "valid", false, []string{"merman"}},
		{"failed primary", "error", "valid", false, []string{"merman", "mmdr-size", "mmdr"}},
		{"corrupt primary", "corrupt", "valid", false, []string{"merman", "mmdr-size", "mmdr"}},
		{"oversized fallback", "error", "huge", true, []string{"merman", "mmdr-size"}},
		{"both fail", "error", "error", true, []string{"merman", "mmdr-size", "mmdr"}},
		{"stale output", "stale", "empty", true, []string{"merman", "mmdr-size", "mmdr"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls []string
			r := &Renderer{merman: "merman", mmdr: "mmdr", cache: make(map[string]result)}
			r.run = func(_ context.Context, exe, dir, input string, args ...string) ([]byte, error) {
				if input != "flowchart LR\n A-->B\n" {
					t.Fatalf("source changed: %q", input)
				}
				if slices.Contains(args, "--size") {
					calls = append(calls, "mmdr-size")
					if tc.fallback == "huge" {
						return []byte(`{"width":5000,"height":20}`), nil
					}
					return []byte(`{"width":40,"height":20}`), nil
				}
				calls = append(calls, exe)
				mode := tc.fallback
				if exe == "merman" {
					mode = tc.primary
					if !slices.Contains(args, "dark") || slices.Contains(args, "--allow-network") {
						t.Fatal(args)
					}
				}
				if mode == "error" {
					return nil, fmt.Errorf("failed")
				}
				if mode == "empty" {
					return nil, nil
				}
				data := encoded.Bytes()
				if mode == "corrupt" {
					data = []byte("not a PNG")
				}
				if err := os.WriteFile(filepath.Join(dir, "diagram.png"), data, 0600); err != nil {
					t.Fatal(err)
				}
				if mode == "stale" {
					return nil, fmt.Errorf("failed after writing")
				}
				return nil, nil
			}
			for range 2 {
				data, err := r.Render(context.Background(), "flowchart LR\n A-->B\n", true)
				if (err != nil) != tc.wantError {
					t.Fatalf("data=%d err=%v", len(data), err)
				}
			}
			if !slices.Equal(calls, tc.calls) {
				t.Fatalf("calls=%v want=%v", calls, tc.calls)
			}
		})
	}
}

func TestFontsAndDiscovery(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	if r := New("off", "off", nil); r.Available() {
		t.Fatal("disabled renderers available")
	}
	if path, err := Find("auto", "merman-cli"); path != "" || err != nil {
		t.Fatalf("missing auto: %q %v", path, err)
	}
	if _, err := Find("missing-wrapper", "merman-cli"); err == nil {
		t.Fatal("missing override accepted")
	}
	r := &Renderer{fontconfig: "fc-match"}
	var patterns []string
	r.run = func(_ context.Context, exe, dir, input string, args ...string) ([]byte, error) {
		patterns = append(patterns, args[len(args)-1])
		return []byte("/fonts/sans.ttf\n"), nil
	}
	if fonts := r.selectFonts(context.Background(), "A\u03bbA"); len(fonts) != 1 {
		t.Fatalf("fonts=%v", fonts)
	}
	if !slices.Equal(patterns, []string{"sans-serif", ":charset=3bb 41"}) {
		t.Fatalf("patterns=%v", patterns)
	}
	r.fonts = []string{"/fonts/custom.ttf"}
	if fonts := r.selectFonts(context.Background(), "text"); !slices.Equal(fonts, r.fonts) || len(patterns) != 2 {
		t.Fatal(fonts, patterns)
	}
}

func TestInputAndDimensionLimits(t *testing.T) {
	r := &Renderer{cache: make(map[string]result)}
	if _, err := r.Render(context.Background(), strings.Repeat("x", (64<<10)+1), false); err == nil {
		t.Fatal("oversized source accepted")
	}
	for _, size := range [][2]float64{{0, 1}, {1, 0}, {4097, 1}, {1, 4097}} {
		if validSize(size[0], size[1]) {
			t.Fatal(size)
		}
	}
}
