package tui

import (
	"io"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/yuin/goldmark/v2/ast"
	"golang.org/x/net/html"
)

var keyboardKind = ast.NewNodeKind("Keyboard")

type markdownDecoration uint8

const (
	noDecoration markdownDecoration = iota
	openingDecoration
	closingDecoration
)

type keyboardNode struct {
	ast.BaseInline
	start, contentStart, contentEnd, end int
}

func (n *keyboardNode) Kind() ast.NodeKind        { return keyboardKind }
func (n *keyboardNode) Dump([]byte) *ast.NodeDump { return ast.NewNodeDump(n, nil) }

// Pair only HTML tokens already recognized by the Markdown parser.  Code
// spans, comments and escaped markup remain literal, and HTML attributes
// containing angle brackets are handled by the HTML tokenizer.
func groupKeyboardNodes(parent ast.Node, source []byte) {
	var openings []*ast.RawHTML
	for n := parent.FirstChild(); n != nil; {
		next := n.NextSibling()
		groupKeyboardNodes(n, source)
		if raw, ok := n.(*ast.RawHTML); ok {
			switch keyboardTag(raw.Value.Str(source)) {
			case html.StartTagToken:
				openings = append(openings, raw)
			case html.EndTagToken:
				if len(openings) > 0 {
					open := openings[len(openings)-1]
					openings = openings[:len(openings)-1]
					a, b := open.Value.Indices(), raw.Value.Indices()
					key := &keyboardNode{start: a[0].Start, contentStart: a[len(a)-1].Stop, contentEnd: b[0].Start, end: b[len(b)-1].Stop}
					key.Init(key)
					parent.InsertBefore(open, key)
					for child := open.NextSibling(); child != raw; {
						following := child.NextSibling()
						parent.RemoveChild(child)
						key.AppendChild(child)
						child = following
					}
					parent.RemoveChild(open)
					parent.RemoveChild(raw)
				}
			}
		}
		n = next
	}
}

func keyboardTag(source string) html.TokenType {
	tokenizer := html.NewTokenizer(strings.NewReader(source))
	kind := tokenizer.Next()
	if kind != html.StartTagToken && kind != html.EndTagToken {
		return html.ErrorToken
	}
	if tokenizer.Token().Data != "kbd" || tokenizer.Next() != html.ErrorToken || tokenizer.Err() != io.EOF {
		return html.ErrorToken
	}
	return kind
}

func containsKeyboard(n ast.Node) bool {
	for child := n.FirstChild(); child != nil; child = child.NextSibling() {
		if _, ok := child.(*keyboardNode); ok || containsKeyboard(child) {
			return true
		}
	}
	return false
}

func keyboardStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(mdCodeStyle.GetForeground())
}

func (line *markdownLine) hasKeyboard() bool {
	for _, token := range line.tokens {
		if token.style&markdownKeyboard != 0 {
			return true
		}
	}
	return false
}
