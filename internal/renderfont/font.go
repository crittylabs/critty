// Package renderfont selects installed fonts for optional native renderers.
package renderfont

import (
	"context"
	"io"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

type Selector struct {
	path  string
	query func(context.Context, string, string, string) ([]byte, error)
}

func New() *Selector {
	path, _ := exec.LookPath("fc-match")
	return &Selector{path: path, query: query}
}

func (s *Selector) Available() bool { return s != nil && s.path != "" }

func (s *Selector) Files(ctx context.Context, source string) []string {
	if !s.Available() {
		return nil
	}
	var fonts []string
	for _, pattern := range []string{"sans-serif", pattern(source)} {
		data, err := s.query(ctx, s.path, "%{file}\n", pattern)
		font := strings.TrimSpace(string(data))
		if err != nil || font == "" || !filepath.IsAbs(font) || strings.ContainsAny(font, "\r\n") {
			continue
		}
		if len(fonts) == 0 || fonts[0] != font {
			fonts = append(fonts, font)
		}
	}
	return fonts
}

func (s *Selector) Family(ctx context.Context, source string) string {
	if !s.Available() {
		return ""
	}
	data, err := s.query(ctx, s.path, "%{family[0]}", pattern(source))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func pattern(source string) string {
	chars := make(map[rune]bool)
	for _, ch := range source {
		if ch > ' ' {
			chars[ch] = true
		}
	}
	var values []string
	for ch := range chars {
		values = append(values, strconv.FormatInt(int64(ch), 16))
	}
	sort.Strings(values)
	return ":charset=" + strings.Join(values, " ")
}

type output []byte

func (out *output) Write(data []byte) (int, error) {
	n := min(len(data), (16<<10)-len(*out))
	*out = append(*out, data[:n]...)
	if n < len(data) {
		return n, io.ErrShortWrite
	}
	return n, nil
}

func query(ctx context.Context, path, format, pattern string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, "--format", format, pattern)
	cmd.WaitDelay = 100 * time.Millisecond
	var out output
	cmd.Stdout = &out
	err := cmd.Run()
	return out, err
}
