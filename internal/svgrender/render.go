// Package svgrender converts SVG to bounded PNG previews with optional CLIs.
package svgrender

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/xml"
	"fmt"
	"image/png"
	"io"
	"math"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/crittylabs/critty/internal/rendercache"

	"github.com/crittylabs/critty/internal/imageinfo"
	"github.com/crittylabs/critty/internal/renderfont"
)

const maxBytes = 5 << 20
const maxSide = 2048

type command func(context.Context, string, string, []byte, ...string) error
type result struct {
	data []byte
	err  error
}

type Renderer struct {
	mu    sync.Mutex
	tools []string
	fonts *renderfont.Selector
	run   command
	cache rendercache.Cache[[32]byte, result]
}

// New discovers converters in priority order. No executable is installed.
func New() *Renderer {
	r := &Renderer{run: runCommand, fonts: renderfont.New()}
	for _, name := range []string{"rsvg-convert", "merman-cli", "resvg"} {
		if path, err := exec.LookPath(name); err == nil {
			r.tools = append(r.tools, path)
		}
	}
	return r
}

func (r *Renderer) Render(ctx context.Context, source []byte) ([]byte, error) {
	if len(source) > maxBytes {
		return nil, fmt.Errorf("SVG exceeds 5 MiB")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	key := sha256.Sum256(source)
	if cached, ok := r.cache.Get(key); ok {
		return cached.data, cached.err
	}
	data, err := r.render(ctx, source)
	if ctx.Err() == nil {
		r.cache.Put(key, result{data, err}, len(data))
	}
	return data, err
}

func (r *Renderer) render(ctx context.Context, source []byte) ([]byte, error) {
	width, height, err := dimensions(source)
	if err != nil {
		return nil, err
	}
	dir, err := os.MkdirTemp("", "critty-svg-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	out := filepath.Join(dir, "preview.png")
	w, h := strconv.Itoa(width), strconv.Itoa(height)
	for _, tool := range r.tools {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if err := os.Remove(out); err != nil && !os.IsNotExist(err) {
			return nil, err
		}
		var args []string
		switch strings.TrimSuffix(filepath.Base(tool), ".exe") {
		case "rsvg-convert":
			args = []string{"--format", "png", "--width", w, "--height", h, "--keep-aspect-ratio", "--output", out, "-"}
		case "merman-cli":
			args = []string{"render", "-", "--input-kind", "svg", "--format", "png", "--output", out, "--quiet", "--operation-timeout-ms", "3000", "--raster-fit-width", w, "--raster-fit-height", h, "--raster-max-width", "2048", "--raster-max-height", "2048", "--raster-max-pixels", "4194304"}
			for _, font := range r.fonts.Files(ctx, string(source)) {
				args = append(args, "--font-file", font)
			}
		case "resvg":
			args = []string{"--width", w, "--height", h, "-", out}
		}
		if err := r.run(ctx, tool, dir, source, args...); err != nil {
			continue
		}
		if data, err := readPNG(out); err == nil {
			return data, nil
		}
	}
	return nil, fmt.Errorf("SVG converters unavailable or failed")
}

// XML parsing validates the complete document and locates its intrinsic size.
// Resource references stay within the document; converters never receive a
// source filename that could establish a base directory for external files.
func dimensions(source []byte) (int, int, error) {
	d := xml.NewDecoder(bytes.NewReader(source))
	depth, roots := 0, 0
	width, height := 0.0, 0.0
	var viewBox []string
	for {
		token, err := d.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return 0, 0, err
		}
		switch token := token.(type) {
		case xml.StartElement:
			if depth == 0 {
				roots++
				if roots != 1 || token.Name.Local != "svg" || token.Name.Space != "" && token.Name.Space != "http://www.w3.org/2000/svg" {
					return 0, 0, fmt.Errorf("not an SVG document")
				}
				for _, a := range token.Attr {
					switch a.Name.Local {
					case "width":
						width = length(a.Value)
					case "height":
						height = length(a.Value)
					case "viewBox":
						viewBox = strings.FieldsFunc(a.Value, func(c rune) bool { return c == ',' || c == ' ' || c == '\t' || c == '\r' || c == '\n' })
					}
				}
			}
			depth++
			if depth > 128 {
				return 0, 0, fmt.Errorf("SVG nesting exceeds 128 levels")
			}
			for _, a := range token.Attr {
				if a.Name.Local == "href" && token.Name.Local != "a" {
					v := strings.TrimSpace(a.Value)
					if v != "" && !strings.HasPrefix(v, "#") && !embeddedRaster(v) {
						return 0, 0, fmt.Errorf("external SVG resources are not supported")
					}
				}
				if a.Name.Space == "http://www.w3.org/XML/1998/namespace" && a.Name.Local == "base" {
					return 0, 0, fmt.Errorf("SVG base URLs are not supported")
				}
			}
		case xml.EndElement:
			depth--
		case xml.Directive:
			return 0, 0, fmt.Errorf("SVG DTDs are not supported")
		case xml.ProcInst:
			if token.Target != "xml" {
				return 0, 0, fmt.Errorf("SVG processing instructions are not supported")
			}
		case xml.CharData:
			if depth == 0 && strings.TrimSpace(string(token)) != "" {
				return 0, 0, fmt.Errorf("invalid SVG document")
			}
		}
	}
	if roots != 1 || depth != 0 {
		return 0, 0, fmt.Errorf("invalid SVG document")
	}
	if len(viewBox) == 4 {
		vw, _ := strconv.ParseFloat(viewBox[2], 64)
		vh, _ := strconv.ParseFloat(viewBox[3], 64)
		if vw > 0 && vh > 0 && !math.IsInf(vw, 0) && !math.IsInf(vh, 0) {
			if width <= 0 && height <= 0 {
				width, height = vw, vh
			} else if width <= 0 {
				width = height * vw / vh
			} else if height <= 0 {
				height = width * vh / vw
			}
		}
	}
	if width <= 0 {
		width = 300
	}
	if height <= 0 {
		height = 150
	}
	if math.IsInf(width, 0) || math.IsInf(height, 0) || math.IsNaN(width) || math.IsNaN(height) {
		return 0, 0, fmt.Errorf("invalid SVG dimensions")
	}
	scale := min(1.0, maxSide/width, maxSide/height)
	return max(1, int(math.Ceil(width*scale))), max(1, int(math.Ceil(height*scale))), nil
}

func embeddedRaster(value string) bool {
	if !strings.HasPrefix(value, "data:") {
		return false
	}
	header, encoded, ok := strings.Cut(strings.TrimPrefix(value, "data:"), ",")
	if !ok {
		return false
	}
	decoded, err := url.PathUnescape(encoded)
	if err != nil {
		return false
	}
	var data []byte
	if strings.HasSuffix(header, ";base64") {
		data, err = base64.StdEncoding.DecodeString(decoded)
	} else {
		data = []byte(decoded)
	}
	if err != nil || len(data) > maxBytes {
		return false
	}
	info, err := imageinfo.Read(data)
	return err == nil && info.Width > 0 && info.Height > 0 && info.Width <= imageinfo.MaxPixels/info.Height
}

func length(value string) float64 {
	value = strings.TrimSpace(value)
	factor := 1.0
	for _, unit := range []struct {
		name  string
		scale float64
	}{{"px", 1}, {"pt", 96.0 / 72}, {"pc", 16}, {"in", 96}, {"cm", 96.0 / 2.54}, {"mm", 96.0 / 25.4}, {"Q", 96.0 / 101.6}} {
		if strings.HasSuffix(value, unit.name) {
			value = strings.TrimSuffix(value, unit.name)
			factor = unit.scale
			break
		}
	}
	n, err := strconv.ParseFloat(value, 64)
	if err != nil || math.IsNaN(n) || math.IsInf(n, 0) || n <= 0 || n > 1e9 {
		return 0
	}
	return n * factor
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
		return nil, fmt.Errorf("SVG preview exceeds 5 MiB")
	}
	cfg, err := png.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > maxSide || cfg.Height > maxSide {
		return nil, fmt.Errorf("SVG preview exceeds 2048 pixels per side")
	}
	if _, err := png.Decode(bytes.NewReader(data)); err != nil {
		return nil, err
	}
	return data, nil
}

func runCommand(ctx context.Context, executable, dir string, input []byte, args ...string) error {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, executable, args...)
	cmd.Dir = dir
	cmd.Stdin = bytes.NewReader(input)
	cmd.WaitDelay = 100 * time.Millisecond
	return cmd.Run()
}
