package clipboard

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestMacClipboardBridge(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("AppKit requires macOS")
	}
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 2, 3))); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "image with spaces.png")
	if err := os.WriteFile(path, b.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	quotedPath, err := json.Marshal(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name, setup string
		wantImage   bool
		wantText    string
	}{
		{"empty", "", false, ""},
		{"text", `pb.setStringForType(' 日本語🙂\ntext ', $.NSPasteboardTypeString);`, false, " 日本語🙂\ntext "},
		{"plain preferred", `pb.setStringForType('plain', $.NSPasteboardTypeString); pb.setStringForType('<b>rich</b>', $.NSPasteboardTypeHTML);`, false, "plain"},
		{"rtf", `pb.setStringForType('{\\rtf1\\ansi bold \\b text\\b0}', $.NSPasteboardTypeRTF);`, false, "bold text"},
		{"html", `pb.setStringForType('<p><b>Bold</b> &amp; <a href="https://example.invalid">link</a><br>次</p><p>end</p><script>ignored</script>', $.NSPasteboardTypeHTML);`, false, "Bold & link\n次\nend"},
		{"html table", `pb.setStringForType('<table><tr><td>A</td><td>B</td></tr><tr><td>C</td><td>D</td></tr></table>', $.NSPasteboardTypeHTML);`, false, "A\tB\nC\tD"},
		{"bitmap", `pb.setDataForType($.NSData.alloc.initWithBase64EncodedStringOptions('` + base64.StdEncoding.EncodeToString(b.Bytes()) + `', 0), 'public.png');`, true, ""},
		{"file", `pb.writeObjects($.NSArray.arrayWithObject($.NSURL.fileURLWithPath(` + string(quotedPath) + `)));`, true, ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			// Use a private pasteboard; never read or overwrite the user's clipboard.
			script := strings.Replace(macImageScript, "const pb = $.NSPasteboard.generalPasteboard;", "const pb = $.NSPasteboard.pasteboardWithUniqueName;\n"+tt.setup, 1)
			out, err := run(context.Background(), "osascript", "-l", "JavaScript", "-e", script)
			if err != nil {
				t.Fatal(err)
			}
			textScript := strings.Replace(macTextScript, "const pb = $.NSPasteboard.generalPasteboard;", "const pb = $.NSPasteboard.pasteboardWithUniqueName;\n"+tt.setup, 1)
			textData, err := run(context.Background(), "osascript", "-l", "JavaScript", "-e", textScript)
			if err != nil {
				t.Fatal(err)
			}
			text, err := decodeMacText(textData)
			if err != nil || text != tt.wantText {
				t.Fatalf("plain text = %q, %v; want %q", text, err, tt.wantText)
			}
			if !tt.wantImage {
				if len(bytes.TrimSpace(out)) != 0 {
					t.Fatal("text clipboard became an image")
				}
				return
			}
			data, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(out)))
			if err != nil {
				t.Fatal(err)
			}
			img, err := png.Decode(bytes.NewReader(data))
			if err != nil {
				t.Fatal(err)
			}
			if img.Bounds().Dx() != 2 || img.Bounds().Dy() != 3 {
				t.Fatalf("image bounds = %v", img.Bounds())
			}
		})
	}
}

func TestLimitedClipboardOutput(t *testing.T) {
	var b limitedBuffer
	if _, err := io.Copy(&b, bytes.NewReader(make([]byte, 8<<20))); err == nil {
		t.Fatal("unbounded clipboard output")
	}
}

func TestLinuxClipboardFormats(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires a POSIX shell")
	}
	for _, wayland := range []bool{false, true} {
		t.Run(map[bool]string{false: "x11", true: "wayland"}[wayland], func(t *testing.T) {
			dir := t.TempDir()
			name := "xclip"
			t.Setenv("WAYLAND_DISPLAY", "")
			if wayland {
				name = "wl-paste"
				t.Setenv("WAYLAND_DISPLAY", "test-display")
			}
			// A command fixture verifies both format negotiation and exact read arguments.
			script := `#!/bin/sh
case "$*" in
'--list-types'|'-selection clipboard -t TARGETS -o') printf '%s\n' "$TEST_CLIPBOARD_TYPES" ;;
'--no-newline --type image/jpeg'|'-selection clipboard -t image/jpeg -o') printf 'image bytes' ;;
'--no-newline --type text/plain; charset=UTF-8'|'-selection clipboard -t text/plain; charset=UTF-8 -o'|'--no-newline --type text/plain'|'-selection clipboard -t text/plain -o'|'--no-newline --type UTF8_STRING'|'-selection clipboard -t UTF8_STRING -o')
  if [ "$1" = '--no-newline' ]; then selected="$3"; else selected="$4"; fi
  [ "$selected" = "$TEST_CLIPBOARD_TEXT_TARGET" ] || exit 1
  printf ' 日本語🙂\nsecond line \n' ;;
'--no-newline --type text/html'|'-selection clipboard -t text/html -o') printf '<p><b>Bold</b> &amp; text</p><p>次</p>' ;;
*) exit 1 ;;
esac
`
			t.Setenv("TEST_CLIPBOARD_TYPES", "text/html\ntext/plain\nimage/jpeg")
			if err := os.WriteFile(filepath.Join(dir, name), []byte(script), 0o700); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", dir)
			data, err := readLinuxImage(context.Background())
			if err != nil || string(data) != "image bytes" {
				t.Fatalf("clipboard = %q, %v", data, err)
			}
			for _, tt := range []struct {
				name, formats, target, want string
				wantError                   bool
			}{
				{"UTF-8 preferred", "text/html\ntext/plain\nUTF8_STRING\ntext/plain; charset=UTF-8", "text/plain; charset=UTF-8", " 日本語🙂\nsecond line \n", false},
				{"plain preferred", "text/html\ntext/plain", "text/plain", " 日本語🙂\nsecond line \n", false},
				{"X11 UTF-8", "text/html\nUTF8_STRING", "UTF8_STRING", " 日本語🙂\nsecond line \n", false},
				{"HTML fallback", "text/html", "text/html", "Bold & text\n次", false},
				{"unsupported", "application/rtf", "", "", true},
			} {
				t.Run(tt.name, func(t *testing.T) {
					t.Setenv("TEST_CLIPBOARD_TYPES", tt.formats)
					t.Setenv("TEST_CLIPBOARD_TEXT_TARGET", tt.target)
					if _, err := readLinuxImage(context.Background()); err != ErrNoImage {
						t.Fatalf("text clipboard: %v", err)
					}
					text, err := readLinuxText(context.Background())
					if (err != nil) != tt.wantError || text != tt.want {
						t.Fatalf("text = %q, %v; want %q, error %v", text, err, tt.want, tt.wantError)
					}
				})
			}
		})
	}
}
