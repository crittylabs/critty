package mathrender

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestFindRaTeX(t *testing.T) {
	compatible := "Read formulas from stdin. Write PNGs. --dpr --inline"
	for _, tc := range []struct {
		name, setting, help string
		found, want, fail   bool
	}{
		{"absent", "", "", false, false, false},
		{"auto", "", compatible, true, true, false},
		{"explicit auto", "auto", compatible, true, true, false},
		{"off", "off", compatible, true, false, false},
		{"unrelated render", "", "Manage cloud deployments", true, false, true},
		{"explicit command", "render", "Custom wrapper", true, true, false},
		{"missing override", "missing-render", "", false, false, true},
		{"oversized help", "", strings.Repeat("x", 17<<10), true, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bin := t.TempDir()
			t.Setenv("PATH", bin)
			if tc.found {
				script := "#!/bin/sh\nprintf '%s\\n' '" + tc.help + "'\n"
				if err := os.WriteFile(filepath.Join(bin, "render"), []byte(script), 0700); err != nil {
					t.Fatal(err)
				}
			}
			path, err := FindRaTeX(tc.setting)
			if (path != "") != tc.want || (err != nil) != tc.fail {
				t.Fatalf("path=%q, err=%v", path, err)
			}
			if path != "" && path != filepath.Join(bin, "render") {
				t.Fatalf("unexpected executable %q", path)
			}
			if !tc.fail {
				if r := New(tc.setting); r.Available() != tc.want {
					t.Fatalf("renderer availability does not match discovery: %+v", r)
				}
			}
		})
	}
}

func TestRaTeXOffKeepsLatexFallback(t *testing.T) {
	bin := t.TempDir()
	t.Setenv("PATH", bin)
	for _, name := range []string{"render", "latex", "dvipng"} {
		if err := os.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\nexit 99\n"), 0700); err != nil {
			t.Fatal(err)
		}
	}
	r := New("off")
	if r.ratex != "" || !r.Available() || r.latex == "" || r.dvipng == "" {
		t.Fatalf("off disabled the LaTeX fallback: %+v", r)
	}
}

func TestRendererFallback(t *testing.T) {
	for _, tc := range []struct {
		name, formula                     string
		ratexFailure, invalidPNG, noLatex bool
		want                              []string
	}{
		{name: "single line", formula: `\frac{1}{2}`, want: []string{"ratex"}},
		{name: "multiline", formula: "a% comment\n+b", want: []string{"latex", "dvipng"}},
		{name: "failed RaTeX", formula: "x", ratexFailure: true, want: []string{"ratex", "latex", "dvipng"}},
		{name: "invalid RaTeX PNG", formula: "x", invalidPNG: true, want: []string{"ratex", "latex", "dvipng"}},
		{name: "missing fallback", formula: "x", ratexFailure: true, noLatex: true, want: []string{"ratex"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls []string
			var imageData bytes.Buffer
			im := image.NewRGBA(image.Rect(0, 0, 8, 8))
			for y := range 8 {
				for x := range 8 {
					im.SetRGBA(x, y, color.RGBA{A: 255})
				}
			}
			if err := png.Encode(&imageData, im); err != nil {
				t.Fatal(err)
			}
			r := &Renderer{ratex: "ratex", latex: "latex", dvipng: "dvipng", cache: make(map[string]result)}
			if tc.noLatex {
				r.latex, r.dvipng = "", ""
			}
			r.run = func(_ context.Context, executable, dir, input string, args ...string) error {
				calls = append(calls, executable)
				if executable == "ratex" {
					if input != tc.formula+"\n" {
						t.Fatalf("input changed: %q", input)
					}
					if tc.ratexFailure {
						return fmt.Errorf("unsupported")
					}
					out := filepath.Join(dir, "ratex")
					if err := os.Mkdir(out, 0o700); err != nil {
						return err
					}
					data := imageData.Bytes()
					if tc.invalidPNG {
						data = []byte("invalid")
					}
					return os.WriteFile(filepath.Join(out, "0.png"), data, 0o600)
				}
				if executable == "latex" {
					data, err := os.ReadFile(filepath.Join(dir, "formula.tex"))
					if err != nil {
						return err
					}
					if !strings.Contains(string(data), tc.formula+"\n$") {
						t.Fatalf("TeX changed or closing delimiter swallowed by comment: %s", data)
					}
					if !strings.Contains(strings.Join(args, " "), "-no-shell-escape") {
						t.Fatal("shell escape not disabled")
					}
					return nil
				}
				return os.WriteFile(filepath.Join(dir, "formula.png"), imageData.Bytes(), 0o600)
			}
			for range 2 {
				data, err := r.Render(context.Background(), tc.formula, false, false)
				if (err != nil) != tc.noLatex {
					t.Fatalf("data=%d err=%v", len(data), err)
				}
			}
			if !slices.Equal(calls, tc.want) {
				t.Fatalf("calls %v, want %v (including cache hit)", calls, tc.want)
			}
		})
	}
}
