package clipboard

import (
	"context"
	"encoding/json"
	"errors"
	"mime"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/atotto/clipboard"
	"golang.org/x/net/html"
)

// ReadText reads plain text, discarding formatting from rich clipboard content.
func ReadText() (string, error) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		return clipboard.ReadAll()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if runtime.GOOS == "linux" {
		return readLinuxText(ctx)
	}
	data, err := run(ctx, "osascript", "-l", "JavaScript", "-e", macTextScript)
	if err != nil {
		return "", err
	}
	return decodeMacText(data)
}

func decodeMacText(data []byte) (string, error) {
	var content struct {
		Text string `json:"text"`
		HTML string `json:"html"`
	}
	if err := json.Unmarshal(data, &content); err != nil {
		return "", err
	}
	if content.HTML == "" {
		return content.Text, nil
	}
	return htmlText(content.HTML)
}

func readLinuxText(ctx context.Context) (string, error) {
	name, args := "xclip", []string{"-selection", "clipboard", "-t", "TARGETS", "-o"}
	if os.Getenv("WAYLAND_DISPLAY") != "" {
		if _, err := exec.LookPath("wl-paste"); err == nil {
			name, args = "wl-paste", []string{"--list-types"}
		}
	}
	if _, err := exec.LookPath(name); err != nil {
		return clipboard.ReadAll() // Keep xsel, Termux and WSL text backends.
	}
	types, err := run(ctx, name, args...)
	if err != nil {
		return "", err
	}
	selected, rank := "", 4
	for _, offered := range strings.Split(string(types), "\n") {
		offered = strings.TrimSpace(offered)
		candidate := 4
		mediaType, params, err := mime.ParseMediaType(offered)
		if err == nil && (params["charset"] == "" || strings.EqualFold(params["charset"], "utf-8") || strings.EqualFold(params["charset"], "us-ascii")) {
			switch mediaType {
			case "text/plain":
				candidate = 2
				if strings.EqualFold(params["charset"], "utf-8") {
					candidate = 0
				}
			case "text/html":
				candidate = 3
			}
		}
		if offered == "UTF8_STRING" {
			candidate = 1
		}
		if candidate < rank {
			selected, rank = offered, candidate
		}
	}
	if selected == "" {
		return "", errors.New("clipboard has no supported text format")
	}
	args = []string{"-selection", "clipboard", "-t", selected, "-o"}
	if name == "wl-paste" {
		args = []string{"--no-newline", "--type", selected}
	}
	data, err := run(ctx, name, args...)
	if err != nil {
		return "", err
	}
	if rank == 3 {
		return htmlText(string(data))
	}
	return string(data), nil
}

func htmlText(source string) (string, error) {
	root, err := html.Parse(strings.NewReader(source))
	if err != nil {
		return "", err
	}
	var text strings.Builder
	newline := func() {
		if text.Len() > 0 && !strings.HasSuffix(text.String(), "\n") {
			text.WriteByte('\n')
		}
	}
	var visit func(*html.Node)
	visit = func(node *html.Node) {
		if node.Type == html.TextNode {
			text.WriteString(node.Data)
			return
		}
		if node.Type == html.ElementNode {
			switch node.Data {
			case "head", "script", "style", "template":
				return
			case "td", "th":
				if text.Len() > 0 && !strings.HasSuffix(text.String(), "\n") {
					text.WriteByte('\t')
				}
			case "br":
				text.WriteByte('\n')
				return
			case "p", "div", "li", "tr", "h1", "h2", "h3", "h4", "h5", "h6", "blockquote", "pre":
				newline()
				defer newline()
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			visit(child)
		}
	}
	visit(root)
	return strings.TrimSuffix(text.String(), "\n"), nil
}

// AppKit converts RTF without rendering HTML or fetching its external resources.
// JSON preserves clipboard whitespace across osascript's output framing.
const macTextScript = `ObjC.import('AppKit');
function run() {
  const pb = $.NSPasteboard.generalPasteboard;
  const text = pb.stringForType($.NSPasteboardTypeString);
  if (text && !text.isNil()) return JSON.stringify({text: ObjC.unwrap(text)});
  const rtf = pb.dataForType($.NSPasteboardTypeRTF);
  if (rtf && !rtf.isNil()) {
    const attributed = $.NSAttributedString.alloc.initWithRTFDocumentAttributes(rtf, null);
    if (attributed && !attributed.isNil()) return JSON.stringify({text: ObjC.unwrap(attributed.string)});
  }
  const html = pb.stringForType($.NSPasteboardTypeHTML);
  if (html && !html.isNil()) return JSON.stringify({html: ObjC.unwrap(html)});
  return JSON.stringify({text: ''});
}`
