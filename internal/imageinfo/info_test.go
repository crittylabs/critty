package imageinfo

import (
	"bytes"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"strings"
	"testing"
)

func TestImageInfo(t *testing.T) {
	for _, format := range []string{"PNG", "JPEG", "GIF", "animated GIF"} {
		t.Run(format, func(t *testing.T) {
			var b bytes.Buffer
			img := image.NewRGBA(image.Rect(0, 0, 3, 2))
			var err error
			switch format {
			case "PNG":
				err = png.Encode(&b, img)
			case "JPEG":
				err = jpeg.Encode(&b, img, nil)
			default:
				palette := color.Palette{color.Black, color.White}
				frame := image.NewPaletted(img.Bounds(), palette)
				g := &gif.GIF{Image: []*image.Paletted{frame}, Delay: []int{1}}
				if format == "animated GIF" {
					g.Image = append(g.Image, image.NewPaletted(img.Bounds(), color.Palette{color.White, color.Black}))
					g.Delay = append(g.Delay, 1)
				}
				err = gif.EncodeAll(&b, g)
			}
			if err != nil {
				t.Fatal(err)
			}
			data := b.Bytes()
			if strings.Contains(format, "GIF") {
				// An image separator inside an extension is not another frame.
				data = append(append(bytes.Clone(data[:len(data)-1]), 0x21, 0xfe, 2, 0x2c, 0x2c, 0), 0x3b)
			}
			info, err := Read(data)
			if err != nil || info.Width != 3 || info.Height != 2 || info.Format != strings.TrimPrefix(format, "animated ") || info.Animated != (format == "animated GIF") {
				t.Fatalf("info = %+v, error = %v", info, err)
			}
			if !strings.Contains(info.String(), "3×2") || strings.Contains(info.String(), "animated") != info.Animated {
				t.Fatalf("description = %q", info.String())
			}
			if strings.Contains(format, "GIF") {
				for end := range len(data) {
					Read(data[:end]) // Truncated metadata must remain safe to inspect.
				}
			}
		})
	}
}
