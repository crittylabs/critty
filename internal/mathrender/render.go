// Package mathrender turns TeX expressions into bounded PNG previews using optional CLIs.
package mathrender

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/png"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/image/draw"
)

const maxBytes = 5 << 20

type command func(context.Context, string, string, string, ...string) error

// Renderer caches a bounded set of results for one viewer session.
type Renderer struct {
	mu                   sync.Mutex
	ratex, latex, dvipng string
	run                  command
	cache                map[string]result
}

type result struct {
	data []byte
	err  error
}

// New detects optional renderers on PATH unless RaTeX is disabled or overridden.
func New(ratex string) *Renderer {
	switch ratex {
	case "", "auto":
		ratex, _ = FindRaTeX("")
	case "off":
		ratex = ""
	}
	latex, _ := exec.LookPath("latex")
	dvipng, _ := exec.LookPath("dvipng")
	return &Renderer{ratex: ratex, latex: latex, dvipng: dvipng, run: runCommand, cache: make(map[string]result)}
}

// FindRaTeX resolves an override or verifies the PNG CLI advertised by render.
// The help check distinguishes its generic command name from unrelated tools.
func FindRaTeX(setting string) (string, error) {
	if setting == "off" {
		return "", nil
	}
	if setting != "" && setting != "auto" {
		return exec.LookPath(setting)
	}
	path, err := exec.LookPath("render")
	if err != nil {
		return "", nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, "--help")
	cmd.WaitDelay = 100 * time.Millisecond
	var data mathHelpOutput
	cmd.Stdout = &data
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("render help: %w", err)
	}
	for _, feature := range []string{"Read formulas from", "Write PNGs", "--dpr", "--inline"} {
		if !strings.Contains(string(data), feature) {
			return "", fmt.Errorf("render on PATH does not advertise the RaTeX PNG CLI")
		}
	}
	return path, nil
}

type mathHelpOutput []byte

func (out *mathHelpOutput) Write(data []byte) (int, error) {
	n := min(len(data), (16<<10)-len(*out))
	*out = append(*out, data[:n]...)
	if n < len(data) {
		return n, io.ErrShortWrite
	}
	return n, nil
}

func (r *Renderer) Available() bool {
	return r != nil && (r.ratex != "" || r.latex != "" && r.dvipng != "")
}

// Render preserves physical newlines and uses LaTeX when RaTeX is inapplicable
// or fails, including when its output is not a valid bounded PNG.
func (r *Renderer) Render(ctx context.Context, formula string, inline, dark bool) ([]byte, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := fmt.Sprintf("%t:%t:%s", inline, dark, formula)
	if v, ok := r.cache[key]; ok {
		return v.data, v.err
	}
	data, err := r.render(ctx, formula, inline, dark)
	if len(r.cache) >= 16 {
		clear(r.cache)
	}
	r.cache[key] = result{data, err}
	return data, err
}

func (r *Renderer) render(ctx context.Context, formula string, inline, dark bool) ([]byte, error) {
	if len(formula) == 0 || len(formula) > 16<<10 {
		return nil, fmt.Errorf("invalid math size")
	}
	dir, err := os.MkdirTemp("", "critty-math-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	color, rgb := "#000000", "0 0 0"
	if dark {
		color, rgb = "#ffffff", "1 1 1"
	}
	if r.ratex != "" && !strings.ContainsAny(formula, "\r\n") {
		out := filepath.Join(dir, "ratex")
		args := []string{"--output-dir", out, "--font-size", "32", "--dpr", "4", "--padding", "6", "--color", color, "--background-color", "transparent"}
		if inline {
			args = append(args, "--inline")
		}
		if err := r.run(ctx, r.ratex, dir, formula+"\n", args...); err == nil {
			entries, err := os.ReadDir(out)
			if err == nil && len(entries) == 1 && !entries[0].IsDir() {
				if data, err := readPNG(filepath.Join(out, entries[0].Name())); err == nil {
					if normalized, err := normalizePNG(data, inline); err == nil {
						return normalized, nil
					}
				}
			}
		}
	}
	if r.latex == "" || r.dvipng == "" {
		return nil, fmt.Errorf("math renderer unavailable")
	}
	style := "\\displaystyle "
	if inline {
		style = "\\textstyle "
	}
	body := "$" + style + formula + "\n$"
	if !inline && displayEnvironment(formula) {
		body = formula
	}
	source := "\\documentclass{article}\n\\usepackage{amsmath,amssymb}\n\\pagestyle{empty}\n\\begin{document}\n" + body + "\n\\end{document}\n"
	if err := os.WriteFile(filepath.Join(dir, "formula.tex"), []byte(source), 0o600); err != nil {
		return nil, err
	}
	if err := r.run(ctx, r.latex, dir, "", "-no-shell-escape", "-halt-on-error", "-interaction=nonstopmode", "formula.tex"); err != nil {
		return nil, fmt.Errorf("latex: %w", err)
	}
	if err := r.run(ctx, r.dvipng, dir, "", "--nogs", "-D", "920", "--gamma", "1", "-T", "tight", "-bg", "Transparent", "-fg", "rgb "+rgb, "-o", "formula.png", "formula.dvi"); err != nil {
		return nil, fmt.Errorf("dvipng: %w", err)
	}
	data, err := readPNG(filepath.Join(dir, "formula.png"))
	if err != nil {
		return nil, err
	}
	return normalizePNG(data, inline)
}

// These environments enter display math themselves.  Inner environments such
// as aligned, gathered, and matrices still need the surrounding math mode.
func displayEnvironment(formula string) bool {
	body := strings.TrimSpace(formula)
	for _, name := range []string{"equation", "eqnarray", "align", "alignat", "flalign", "gather", "multline"} {
		if strings.HasPrefix(body, "\\begin{"+name+"}") || strings.HasPrefix(body, "\\begin{"+name+"*}") {
			return true
		}
	}
	return false
}

// Both engines rasterize at four times the target size.  Use one resampler
// and padding instead of mixing their native antialiasing/padding.
func normalizePNG(data []byte, inline bool) ([]byte, error) {
	source, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	bounds := source.Bounds()
	ink := image.Rectangle{}
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			_, _, _, alpha := source.At(x, y).RGBA()
			if alpha != 0 {
				ink = ink.Union(image.Rect(x, y, x+1, y+1))
			}
		}
	}
	if ink.Empty() {
		return nil, fmt.Errorf("empty math image")
	}
	size := image.Pt((ink.Dx()+3)/4, (ink.Dy()+3)/4)
	padding := image.Pt(3, 3)
	if inline {
		padding.X = 0
	}
	out := image.NewNRGBA(image.Rectangle{Max: size.Add(padding.Mul(2))})
	draw.CatmullRom.Scale(out, image.Rectangle{Min: padding, Max: padding.Add(size)}, source, ink, draw.Src, nil)
	// Thin rules can cover less than one pixel after resampling.  Raise their
	// coverage without changing glyph bounds or the light/dark foreground.
	for i := 3; i < len(out.Pix); i += 4 {
		out.Pix[i] = uint8(math.Round(255 * math.Sqrt(float64(out.Pix[i])/255)))
	}
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, out); err != nil {
		return nil, err
	}
	return encoded.Bytes(), nil
}

func readPNG(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxBytes {
		return nil, fmt.Errorf("math image exceeds 5 MiB")
	}
	cfg, err := png.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > (16<<20)/cfg.Height {
		return nil, fmt.Errorf("math image exceeds 16 megapixels")
	}
	if _, err := png.Decode(bytes.NewReader(data)); err != nil {
		return nil, err
	}
	return data, nil
}

func runCommand(ctx context.Context, executable, dir, input string, args ...string) error {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, executable, args...)
	cmd.Dir = dir
	cmd.Stdin = strings.NewReader(input)
	cmd.Env = append(os.Environ(), "openin_any=p", "openout_any=p", "TEXMFOUTPUT="+dir, "TEXMFVAR="+dir, "MKTEXPK=0", "MKTEXTFM=0")
	return cmd.Run()
}
