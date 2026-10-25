package tui

import (
	"bytes"
	"go/ast"
	"go/format"
	"go/parser"
	"go/scanner"
	"go/token"
	"strings"

	"github.com/crittylabs/critty/internal/document"
)

type goOutlineCache struct {
	doc     *document.Document
	content string
	partial bool
	rows    []outlineHeading
}

func (t *FileTab) ensureGoOutline() *goOutlineCache {
	if c := t.goOutline; c != nil && c.doc == t.doc && c.content == t.doc.Content && c.partial == (t.doc.Known != nil) {
		return c
	}
	c := &goOutlineCache{doc: t.doc, content: t.doc.Content, partial: t.doc.Known != nil}
	t.goOutline = c
	c.rows = parseOutlineDocument(t.doc, parseGoOutline)
	return c
}

func parseGoOutline(source string, offset int) []outlineHeading {
	fset := token.NewFileSet()
	// Standalone diff fragments may not contain the package clause.
	var scan scanner.Scanner
	scan.Init(fset.AddFile("", -1, len(source)), []byte(source), nil, 0)
	_, first, _ := scan.Scan()
	if first != token.PACKAGE {
		source = "package outline\n" + source
		offset--
	}
	// ParseFile returns a partial AST on syntax errors. Keep declarations that
	// it recovered, without resolving identifiers or loading other packages.
	file, _ := parser.ParseFile(fset, "", source, parser.SkipObjectResolution|parser.AllErrors)
	if file == nil {
		return nil
	}
	var rows []outlineHeading
	appendEntry := func(pos token.Pos, name *ast.Ident, label string) {
		if name == nil || name.Name == "_" || name.Name == "" {
			return
		}
		line := fset.PositionFor(pos, false).Line + offset
		if line > 0 {
			rows = append(rows, outlineHeading{line: line, level: 1, label: label})
		}
	}
	for _, decl := range file.Decls {
		switch decl := decl.(type) {
		case *ast.FuncDecl:
			if decl.Name == nil {
				continue
			}
			label := "func "
			if decl.Recv != nil && len(decl.Recv.List) > 0 {
				var receiver bytes.Buffer
				if err := format.Node(&receiver, fset, decl.Recv.List[0].Type); err != nil {
					continue
				}
				label += "(" + strings.Join(strings.Fields(receiver.String()), " ") + ") "
			}
			appendEntry(decl.Pos(), decl.Name, label+decl.Name.Name)
		case *ast.GenDecl:
			if decl.Tok != token.TYPE {
				continue
			}
			for _, spec := range decl.Specs {
				if spec, ok := spec.(*ast.TypeSpec); ok && spec.Name != nil {
					appendEntry(spec.Pos(), spec.Name, "type "+spec.Name.Name)
				}
			}
		}
	}
	return rows
}
