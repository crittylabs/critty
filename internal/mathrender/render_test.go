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
		display                           bool
		want                              []string
	}{
		{name: "single line", formula: `\frac{1}{2}`, want: []string{"ratex"}},
		{name: "multiline", formula: "a% comment\n+b", want: []string{"latex", "dvipng"}},
		{name: "display environment", formula: " \\begin{eqnarray*}\na &=& b \\\\\n&=& c\n\\end{eqnarray*} ", display: true, want: []string{"latex", "dvipng"}},
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
					want := tc.formula + "\n$"
					if tc.display {
						want = "\\begin{document}\n" + tc.formula + "\n\\end{document}"
					}
					if !strings.Contains(string(data), want) {
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

func TestNormalizePNGKeepsThinRulesVisible(t *testing.T) {
	for _, rgb := range []uint8{0, 255} {
		t.Run(fmt.Sprintf("foreground %d", rgb), func(t *testing.T) {
			source := image.NewNRGBA(image.Rect(0, 0, 80, 40))
			ink := color.NRGBA{R: rgb, G: rgb, B: rgb, A: 255}
			// Keep the full glyph height while placing two thin equality rules
			// at different subpixel positions in the four-times raster.
			source.SetNRGBA(0, 0, ink)
			source.SetNRGBA(0, 39, ink)
			for x := 16; x < 80; x++ {
				source.SetNRGBA(x, 11, ink)
				source.SetNRGBA(x, 24, ink)
			}
			var data bytes.Buffer
			if err := png.Encode(&data, source); err != nil {
				t.Fatal(err)
			}
			dataOut, err := normalizePNG(data.Bytes(), false)
			if err != nil {
				t.Fatal(err)
			}
			out, err := png.Decode(bytes.NewReader(dataOut))
			if err != nil {
				t.Fatal(err)
			}
			for _, band := range [][2]int{{3, 8}, {8, 13}} {
				peak := uint32(0)
				for y := band[0]; y < band[1]; y++ {
					_, _, _, a := out.At(12, y).RGBA()
					peak = max(peak, a>>8)
				}
				if peak < 100 {
					t.Fatalf("thin rule in rows %v faded to alpha %d", band, peak)
				}
			}
		})
	}
}

func TestInlineMathHasNoSidePadding(t *testing.T) {
	source := image.NewNRGBA(image.Rect(0, 0, 16, 16))
	for y := range 16 {
		for x := range 16 {
			source.SetNRGBA(x, y, color.NRGBA{A: 255})
		}
	}
	var data bytes.Buffer
	if err := png.Encode(&data, source); err != nil {
		t.Fatal(err)
	}
	result, err := normalizePNG(data.Bytes(), true)
	if err != nil {
		t.Fatal(err)
	}
	out, err := png.Decode(bytes.NewReader(result))
	if err != nil {
		t.Fatal(err)
	}
	for _, x := range []int{0, out.Bounds().Max.X - 1} {
		_, _, _, alpha := out.At(x, out.Bounds().Dy()/2).RGBA()
		if alpha == 0 {
			t.Fatalf("inline math has transparent side padding at x=%d", x)
		}
	}
}
