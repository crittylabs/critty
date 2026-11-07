package svgrender

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestConversionFallbackAndCache(t *testing.T) {
	var valid bytes.Buffer
	if err := png.Encode(&valid, image.NewNRGBA(image.Rect(0, 0, 40, 20))); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		modes  []string
		want   []string
		failed bool
	}{
		{"first", []string{"valid", "valid", "valid"}, []string{"rsvg-convert"}, false},
		{"second", []string{"error", "valid", "valid"}, []string{"rsvg-convert", "merman-cli"}, false},
		{"third", []string{"corrupt", "error", "valid"}, []string{"rsvg-convert", "merman-cli", "resvg"}, false},
		{"all fail", []string{"error", "corrupt", "error"}, []string{"rsvg-convert", "merman-cli", "resvg"}, true},
		{"stale", []string{"stale", "empty", "empty"}, []string{"rsvg-convert", "merman-cli", "resvg"}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls []string
			r := &Renderer{tools: []string{"rsvg-convert", "merman-cli", "resvg"}, cache: make(map[[32]byte]result)}
			r.run = func(_ context.Context, exe, dir string, input []byte, args ...string) error {
				index := len(calls)
				calls = append(calls, exe)
				if !bytes.Contains(input, []byte("<svg")) || !slices.Contains(args, "-") || !slices.Contains(args, "40") || !slices.Contains(args, "20") {
					t.Fatal(args)
				}
				mode := tc.modes[index]
				if mode == "error" {
					return fmt.Errorf("failed")
				}
				if mode == "empty" {
					return nil
				}
				data := valid.Bytes()
				if mode == "corrupt" {
					data = []byte("not PNG")
				}
				if err := os.WriteFile(filepath.Join(dir, "preview.png"), data, 0600); err != nil {
					t.Fatal(err)
				}
				if mode == "stale" {
					return fmt.Errorf("failed after writing")
				}
				return nil
			}
			for range 2 {
				_, err := r.Render(context.Background(), []byte(`<svg width="40" height="20"/>`))
				if (err != nil) != tc.failed {
					t.Fatal(err)
				}
			}
			if !slices.Equal(calls, tc.want) {
				t.Fatalf("calls=%v", calls)
			}
		})
	}
}

func TestSVGDimensionsAndValidation(t *testing.T) {
	for _, tc := range []struct {
		name, source  string
		width, height int
	}{
		{"pixels", `<svg width="40px" height="20"/>`, 40, 20},
		{"physical", `<svg width="1in" height="72pt"/>`, 96, 96},
		{"viewBox", `<svg viewBox="0,0,200,100"/>`, 200, 100},
		{"ratio", `<svg width="40" viewBox="0 0 200 100"/>`, 40, 20},
		{"percent", `<svg width="100%" height="100%" viewBox="0 0 200 100"/>`, 200, 100},
		{"large", `<svg width="8192" height="4096"/>`, 2048, 1024},
		{"default", `<svg/>`, 300, 150},
		{"internal references", `<svg><defs><path id="p"/></defs><use href="#p"/><a href="https://example.org"><rect/></a></svg>`, 300, 150},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w, h, err := dimensions([]byte(tc.source))
			if err != nil || w != tc.width || h != tc.height {
				t.Fatalf("%dx%d %v", w, h, err)
			}
		})
	}
	for _, source := range []string{
		`<html/>`, `<svg>`, `<svg/><svg/>`, `<svg/>text`, `<svg xmlns="urn:other"/>`,
		`<!DOCTYPE svg SYSTEM "file:///missing"><svg/>`,
		`<?xml-stylesheet href="file:///missing"?><svg/>`,
		`<svg><image href="/missing.png"/></svg>`,
		`<svg xmlns:xlink="http://www.w3.org/1999/xlink"><image xlink:href="https://example.org/image.png"/></svg>`,
		`<svg xml:base="file:///"><use href="#p"/></svg>`,
		strings.Repeat("<svg>", 129) + strings.Repeat("</svg>", 129),
	} {
		if _, _, err := dimensions([]byte(source)); err == nil {
			t.Fatalf("invalid SVG accepted: %s", source)
		}
	}
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, image.NewNRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	source := `<svg><image href="data:image/png;base64,` + base64.StdEncoding.EncodeToString(encoded.Bytes()) + `"/></svg>`
	if _, _, err := dimensions([]byte(source)); err != nil {
		t.Fatal(err)
	}
}

func TestDiscoveryAndCancellation(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	if tools := New().tools; len(tools) != 0 {
		t.Fatal(tools)
	}
	r := New()
	if _, err := r.Render(context.Background(), make([]byte, maxBytes+1)); err == nil {
		t.Fatal("oversized SVG accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	r.tools = []string{"rsvg-convert", "merman-cli"}
	calls := 0
	r.run = func(context.Context, string, string, []byte, ...string) error { calls++; cancel(); return ctx.Err() }
	if _, err := r.Render(ctx, []byte("<svg/>")); err == nil || calls != 1 || len(r.cache) != 0 {
		t.Fatal(err, calls, len(r.cache))
	}
}
