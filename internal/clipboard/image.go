// Package clipboard reads images without requiring cgo in release binaries.
package clipboard

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/crittylabs/critty/internal/review"
)

var ErrNoImage = errors.New("no image on clipboard")

// ReadImage prefers copied image files on macOS, then native image data, as
// Codex CLI does.  Linux requests image data from the active display clipboard.
func ReadImage() ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	switch runtime.GOOS {
	case "darwin":
		data, err := run(ctx, "osascript", "-l", "JavaScript", "-e", macImageScript)
		if err != nil {
			return nil, err
		}
		if len(bytes.TrimSpace(data)) == 0 {
			return nil, ErrNoImage
		}
		return base64.StdEncoding.DecodeString(strings.TrimSpace(string(data)))
	case "linux":
		return readLinuxImage(ctx)
	default:
		return nil, ErrNoImage
	}
}

func readLinuxImage(ctx context.Context) ([]byte, error) {
	name, args := "xclip", []string{"-selection", "clipboard", "-t", "TARGETS", "-o"}
	if os.Getenv("WAYLAND_DISPLAY") != "" {
		name, args = "wl-paste", []string{"--list-types"}
	}
	types, err := run(ctx, name, args...)
	if err != nil {
		return nil, ErrNoImage // Allow the existing text clipboard fallback.
	}
	for _, mime := range []string{"image/png", "image/jpeg", "image/gif", "image/webp"} {
		for _, offered := range strings.Fields(string(types)) {
			if offered != mime {
				continue
			}
			if name == "wl-paste" {
				return run(ctx, name, "--no-newline", "--type", mime)
			}
			return run(ctx, name, "-selection", "clipboard", "-t", mime, "-o")
		}
	}
	return nil, ErrNoImage
}

type limitedBuffer struct {
	buffer bytes.Buffer
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	// Allow Base64 expansion for the macOS bridge.
	if b.buffer.Len()+len(p) > (review.MaxAttachmentBytes+2)/3*4+1024 {
		return 0, fmt.Errorf("clipboard image exceeds 5 MiB")
	}
	return b.buffer.Write(p)
}

func run(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.WaitDelay = 200 * time.Millisecond
	var out limitedBuffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("reading image with %s: %w", name, err)
	}
	return out.buffer.Bytes(), nil
}

// AppKit accepts both Finder file URLs and bitmap clipboard representations.
// Only image bytes cross stdout; paths and clipboard text are never executed.
const macImageScript = `ObjC.import('AppKit');
function run() {
  const pb = $.NSPasteboard.generalPasteboard;
  let image = null;
  const urls = pb.readObjectsForClassesOptions($.NSArray.arrayWithObject($.NSURL), $({NSPasteboardURLReadingFileURLsOnlyKey: true}));
  if (urls) {
    for (let i = 0; i < urls.count; i++) {
      const candidate = $.NSImage.alloc.initWithContentsOfURL(urls.objectAtIndex(i));
      if (candidate && !candidate.isNil()) { image = candidate; break; }
    }
  }
  if (!image) image = $.NSImage.alloc.initWithPasteboard(pb);
  if (!image || image.isNil()) return '';
  const bitmap = $.NSBitmapImageRep.imageRepWithData(image.TIFFRepresentation);
  if (!bitmap || bitmap.isNil()) throw Error('cannot decode clipboard image');
  const png = bitmap.representationUsingTypeProperties($.NSPNGFileType, $({}));
  if (!png || png.isNil()) throw Error('cannot encode clipboard image');
  if (png.length > 5242880) throw Error('clipboard image exceeds 5 MiB');
  return ObjC.unwrap(png.base64EncodedStringWithOptions(0));
}`
