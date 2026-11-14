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
	"github.com/crittylabs/critty/internal/icons"
)

type goOutlineCache struct {
	doc     *document.Document
	content string
	partial bool
	rows    []outlineSymbol
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

func parseGoOutline(source string, offset int) []outlineSymbol {
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
	var rows []outlineSymbol
	appendEntry := func(pos token.Pos, name *ast.Ident, label, short string, kind icons.Kind) {
		if name == nil || name.Name == "_" || name.Name == "" {
			return
		}
		line := fset.PositionFor(pos, false).Line + offset
		if line > 0 {
			rows = append(rows, outlineSymbol{outlineHeading{line: line, level: 1, label: label}, kind, short})
		}
	}
	for _, decl := range file.Decls {
		switch decl := decl.(type) {
		case *ast.FuncDecl:
			if decl.Name == nil {
				continue
			}
			name := decl.Name.Name
			kind := icons.Function
			if decl.Recv != nil && len(decl.Recv.List) > 0 {
				kind = icons.Method
				var receiver bytes.Buffer
				if err := format.Node(&receiver, fset, decl.Recv.List[0].Type); err != nil {
					continue
				}
				name = "(" + strings.Join(strings.Fields(receiver.String()), " ") + ") " + name
			}
			appendEntry(decl.Pos(), decl.Name, "func "+name, name, kind)
		case *ast.GenDecl:
			if decl.Tok != token.TYPE {
				continue
			}
			for _, spec := range decl.Specs {
				if spec, ok := spec.(*ast.TypeSpec); ok && spec.Name != nil {
					kind := icons.Class
					switch spec.Type.(type) {
					case *ast.StructType:
						kind = icons.Struct
					case *ast.InterfaceType:
						kind = icons.Interface
					}
					appendEntry(spec.Pos(), spec.Name, "type "+spec.Name.Name, spec.Name.Name, kind)
				}
			}
		}
	}
	return rows
}
