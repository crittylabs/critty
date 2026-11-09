// Package mermaidrender renders Mermaid through optional native CLIs.
package mermaidrender

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image/png"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/crittylabs/critty/internal/rendercache"

	"github.com/crittylabs/critty/internal/renderfont"
)

type command func(context.Context, string, string, string, ...string) ([]byte, error)

type result struct {
	data []byte
	err  error
}

// Renderer keeps subprocesses and a small result cache within one viewer.
type Renderer struct {
	mu           sync.Mutex
	merman, mmdr string
	fontSelector *renderfont.Selector
	fonts        []string
	run          command
	cache        rendercache.Cache[string, result]
}

// Find resolves a global command override, auto discovery, or explicit off.
func Find(setting, name string) (string, error) {
	if setting == "off" {
		return "", nil
	}
	if setting != "" && setting != "auto" {
		return exec.LookPath(setting)
	}
	path, _ := exec.LookPath(name)
	return path, nil
}

func New(merman, mmdr string, fonts []string) *Renderer {
	merman, _ = Find(merman, "merman-cli")
	mmdr, _ = Find(mmdr, "mmdr")
	return &Renderer{merman: merman, mmdr: mmdr, fontSelector: renderfont.New(), fonts: append([]string(nil), fonts...), run: runCommand}
}

func (r *Renderer) Available() bool {
	return r != nil && (r.merman != "" || r.mmdr != "")
}

// Render tries Merman, then mmdr, accepting only a bounded, decodable PNG.
func (r *Renderer) Render(ctx context.Context, source string, dark bool) ([]byte, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := fmt.Sprintf("%t:%s", dark, source)
	if cached, ok := r.cache.Get(key); ok {
		return cached.data, cached.err
	}
	data, err := r.render(ctx, source, dark)
	if ctx.Err() == nil {
		r.cache.Put(key, result{data, err}, len(data))
	}
	return data, err
}

func (r *Renderer) render(ctx context.Context, source string, dark bool) ([]byte, error) {
	if source == "" || len(source) > 64<<10 {
		return nil, fmt.Errorf("invalid Mermaid size")
	}
	dir, err := os.MkdirTemp("", "critty-mermaid-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	theme := "default"
	if dark {
		theme = "dark"
	}
	fonts := r.selectFonts(ctx, source)
	out := filepath.Join(dir, "diagram.png")
	if r.merman != "" {
		args := []string{"render", "-", "--format", "png", "--output", out, "--theme", theme, "--background", "transparent", "--quiet", "--operation-timeout-ms", "3000", "--raster-max-width", "4096", "--raster-max-height", "4096", "--raster-max-pixels", "16777216"}
		for _, font := range fonts {
			args = append(args, "--font-file", font)
		}
		if _, err := r.run(ctx, r.merman, dir, source, args...); err == nil {
			if data, err := readPNG(out); err == nil {
				return data, nil
			}
		}
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if err := os.Remove(out); err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	if r.mmdr == "" {
		return nil, fmt.Errorf("Mermaid renderer unavailable or failed")
	}
	// mmdr has no raster budget flags. Check its natural dimensions before
	// allowing the PNG allocation, using the same source, theme, and fonts.
	args := []string{"--input", "-", "--theme", theme}
	if len(fonts) > 0 && r.fontSelector.Available() {
		// Resolve an installed family matching the source character set.
		// mmdr accepts families through configuration, not explicit font files.
		if family := r.fontSelector.Family(ctx, source); family != "" {
			data, err := json.Marshal(map[string]any{"themeVariables": map[string]string{"fontFamily": family}})
			if err != nil {
				return nil, err
			}
			config := filepath.Join(dir, "config.json")
			if err := os.WriteFile(config, data, 0o600); err != nil {
				return nil, err
			}
			args = append(args, "--configFile", config)
		}
	}
	size, err := r.run(ctx, r.mmdr, dir, source, append(append([]string(nil), args...), "--size")...)
	if err != nil {
		return nil, err
	}
	var bounds struct{ Width, Height float64 }
	if err := json.Unmarshal(size, &bounds); err != nil || !validSize(bounds.Width, bounds.Height) {
		return nil, fmt.Errorf("invalid Mermaid dimensions")
	}
	if _, err := r.run(ctx, r.mmdr, dir, source, append(args, "--outputFormat", "png", "--output", out)...); err != nil {
		return nil, err
	}
	return readPNG(out)
}

func validSize(width, height float64) bool {
	return !math.IsNaN(width) && !math.IsNaN(height) && width > 0 && height > 0 && width <= 4096 && height <= 4096 && math.Ceil(width)*math.Ceil(height) <= 16<<20
}

func (r *Renderer) selectFonts(ctx context.Context, source string) []string {
	if len(r.fonts) > 0 {
		return r.fonts
	}
	return r.fontSelector.Files(ctx, source)
}

func readPNG(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, (5<<20)+1))
	if err != nil {
		return nil, err
	}
	if len(data) > 5<<20 {
		return nil, fmt.Errorf("Mermaid image exceeds 5 MiB")
	}
	cfg, err := png.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	if !validSize(float64(cfg.Width), float64(cfg.Height)) {
		return nil, fmt.Errorf("invalid Mermaid dimensions")
	}
	if _, err := png.Decode(bytes.NewReader(data)); err != nil {
		return nil, err
	}
	return data, nil
}

type boundedOutput []byte

func (out *boundedOutput) Write(data []byte) (int, error) {
	n := min(len(data), (16<<10)-len(*out))
	*out = append(*out, data[:n]...)
	if n < len(data) {
		return n, io.ErrShortWrite
	}
	return n, nil
}

func runCommand(ctx context.Context, executable, dir, input string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, executable, args...)
	cmd.Dir = dir
	cmd.Stdin = strings.NewReader(input)
	cmd.WaitDelay = 100 * time.Millisecond
	var output boundedOutput
	cmd.Stdout = &output
	err := cmd.Run()
	return output, err
}
