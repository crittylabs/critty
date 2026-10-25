// Package imageinfo reads image metadata without decoding pixel data.
package imageinfo

import (
	"bytes"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"strings"

	_ "golang.org/x/image/webp"
)

type Info struct {
	Width, Height int
	Format        string
	Animated      bool
}

func Read(data []byte) (Info, error) {
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return Info{}, err
	}
	info := Info{Width: cfg.Width, Height: cfg.Height, Format: strings.ToUpper(format)}
	if format == "gif" {
		info.Animated = animatedGIF(data)
	}
	return info, nil
}

func (i Info) String() string {
	animation := ""
	if i.Animated {
		animation = ", animated"
	}
	return fmt.Sprintf("%s (%d×%d%s)", i.Format, i.Width, i.Height, animation)
}

// Follow GIF89a block boundaries rather than decoding all animation frames.
// Only a second complete image block is needed to identify animation.
func animatedGIF(data []byte) bool {
	if len(data) < 13 {
		return false
	}
	pos := 13
	skip := func(n int) bool {
		if n > len(data)-pos {
			return false
		}
		pos += n
		return true
	}
	colorTable := func(flags byte) bool {
		return flags&0x80 == 0 || skip(3<<(1+(flags&7)))
	}
	subBlocks := func() bool {
		for pos < len(data) {
			n := int(data[pos])
			pos++
			if n == 0 {
				return true
			}
			if !skip(n) {
				return false
			}
		}
		return false
	}
	if !colorTable(data[10]) {
		return false
	}
	frames := 0
	for pos < len(data) {
		block := data[pos]
		pos++
		switch block {
		case 0x21: // Extension label followed by data sub-blocks.
			if !skip(1) || !subBlocks() {
				return false
			}
		case 0x2c: // Image descriptor, optional palette, and LZW data.
			if !skip(9) || !colorTable(data[pos-1]) || !skip(1) || !subBlocks() {
				return false
			}
			frames++
			if frames == 2 {
				return true
			}
		default: // Trailer or malformed input.
			return false
		}
	}
	return false
}
