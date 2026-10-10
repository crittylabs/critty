// Package form interprets task lists in attached Markdown documents.
package form

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mattn/go-shellwords"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	extast "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/text"
)

type Span struct {
	Start int `json:"start"`
	End   int `json:"end"`
}

type Item struct {
	ID         string `json:"id"`
	Value      string `json:"value,omitempty"`
	Choice     bool   `json:"choice,omitempty"`
	Explicit   bool   `json:"explicit,omitempty"`
	Required   bool   `json:"required,omitempty"`
	Consent    bool   `json:"consent,omitempty"`
	Initial    bool   `json:"initial,omitempty"`
	Label      string `json:"label"`
	Definition string `json:"definition"`
	Line       int    `json:"line"`
	EndLine    int    `json:"end_line"`
	Offset     int    `json:"offset"`
	Metadata   []Span `json:"metadata,omitempty"`
}

type Answer struct {
	Value    any    `json:"value"`
	Human    bool   `json:"human"`
	Author   string `json:"author,omitempty"`
	Revision string `json:"review_revision,omitempty"`
	Round    int    `json:"round,omitempty"`
}

// Parse uses the Markdown AST so examples, escaped lists, and quotations are inert.
func Parse(source string) ([]Item, error) {
	data := []byte(source)
	root := goldmark.New(goldmark.WithExtensions(extension.TaskList)).Parser().Parse(text.NewReader(data))
	var items []Item
	err := ast.Walk(root, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		checkbox, ok := n.(*extast.TaskCheckBox)
		if !entering || !ok {
			return ast.WalkContinue, nil
		}
		for p := n.Parent(); p != nil; p = p.Parent() {
			if _, quoted := p.(*ast.Blockquote); quoted {
				return ast.WalkContinue, nil
			}
		}
		block := n.Parent()
		start := block.Lines().At(0).Start
		item := Item{Line: strings.Count(source[:start], "\n") + 1, Offset: start, Initial: checkbox.IsChecked}
		end := block.Lines().At(block.Lines().Len() - 1).Stop
		for end > start && (source[end-1] == '\n' || source[end-1] == '\r') {
			end--
		}
		item.EndLine = strings.Count(source[:end], "\n") + 1
		var label strings.Builder
		for i := 0; i < block.Lines().Len(); i++ {
			segment := block.Lines().At(i)
			label.Write(segment.Value(data))
		}
		err := ast.Walk(block, func(child ast.Node, enter bool) (ast.WalkStatus, error) {
			html, ok := child.(*ast.RawHTML)
			if !enter || !ok {
				return ast.WalkContinue, nil
			}
			var raw strings.Builder
			for i := 0; i < html.Segments.Len(); i++ {
				seg := html.Segments.At(i)
				raw.Write(seg.Value(data))
			}
			comment := strings.TrimSpace(raw.String())
			if !strings.HasPrefix(comment, "<!--") || !strings.HasSuffix(comment, "-->") {
				return ast.WalkContinue, nil
			}
			body := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(comment, "<!--"), "-->"))
			if !strings.HasPrefix(body, "form:") {
				return ast.WalkContinue, nil
			}
			if len(item.Metadata) > 0 {
				return ast.WalkStop, fmt.Errorf("line %d: multiple form annotations", item.Line)
			}
			if err := attributes(&item, strings.TrimPrefix(body, "form:")); err != nil {
				return ast.WalkStop, fmt.Errorf("line %d: %w", item.Line, err)
			}
			for i := 0; i < html.Segments.Len(); i++ {
				seg := html.Segments.At(i)
				item.Metadata = append(item.Metadata, Span{Start: seg.Start, End: seg.Stop})
			}
			labelText := label.String()
			label.Reset()
			label.WriteString(strings.Replace(labelText, raw.String(), "", 1))
			return ast.WalkContinue, nil
		})
		if err != nil {
			return ast.WalkStop, err
		}
		item.Label = strings.TrimSpace(strings.TrimSpace(label.String())[3:])
		var definition strings.Builder
		if err := ast.Walk(block.Parent(), func(child ast.Node, enter bool) (ast.WalkStatus, error) {
			if !enter {
				return ast.WalkContinue, nil
			}
			if _, nested := child.(*ast.List); nested {
				return ast.WalkSkipChildren, nil
			}
			if child.Type() == ast.TypeBlock {
				for i := 0; i < child.Lines().Len(); i++ {
					segment := child.Lines().At(i)
					definition.Write(segment.Value(data))
				}
			}
			return ast.WalkContinue, nil
		}); err != nil {
			return ast.WalkStop, err
		}
		item.Definition = definition.String()
		if item.ID == "" {
			if item.Required || item.Consent || item.Choice {
				return ast.WalkStop, fmt.Errorf("line %d: form requires an explicit id", item.Line)
			}
			item.ID = fmt.Sprintf("auto:%d", item.Line)
		}
		items = append(items, item)
		return ast.WalkContinue, nil
	})
	if err != nil {
		return nil, err
	}
	groups := map[string][]Item{}
	for _, item := range items {
		if item.Consent && item.Choice {
			return nil, fmt.Errorf("line %d: consent cannot have a value", item.Line)
		}
		for _, old := range groups[item.ID] {
			if !old.Choice || !item.Choice {
				return nil, fmt.Errorf("line %d: duplicate boolean id %q", item.Line, item.ID)
			}
			if old.Value == item.Value || old.Required != item.Required || (old.Initial && item.Initial) {
				return nil, fmt.Errorf("line %d: inconsistent choices for %q", item.Line, item.ID)
			}
		}
		groups[item.ID] = append(groups[item.ID], item)
	}
	return items, nil
}

func attributes(item *Item, source string) error {
	parser := shellwords.NewParser()
	parser.ParseEnv, parser.ParseBacktick = false, false
	words, err := parser.Parse(source)
	if err != nil {
		return fmt.Errorf("invalid form annotation: %w", err)
	}
	if parser.Position != -1 {
		return fmt.Errorf("invalid form annotation: shell operator")
	}
	seen := map[string]bool{}
	for _, word := range words {
		key, value, assigned := strings.Cut(word, "=")
		if seen[key] {
			return fmt.Errorf("duplicate form attribute %q", key)
		}
		seen[key] = true
		switch {
		case key == "id" && assigned && value != "" && !strings.HasPrefix(value, "auto:"):
			item.ID, item.Explicit = value, true
		case key == "value" && assigned && value != "":
			item.Value, item.Choice = value, true
		case key == "required" && !assigned:
			item.Required = true
		case key == "consent" && !assigned:
			item.Consent = true
		default:
			return fmt.Errorf("invalid form attribute %q", word)
		}
	}
	return nil
}

// Fingerprint excludes source positions but includes the complete ordered group.
func Fingerprint(items []Item, id string) (string, error) {
	var definitions []Item
	for _, item := range items {
		if item.ID == id {
			item.Line, item.EndLine, item.Offset, item.Metadata = 0, 0, 0, nil
			definitions = append(definitions, item)
		}
	}
	data, err := json.Marshal(definitions)
	return fmt.Sprintf("%x", sha256.Sum256(data)), err
}

func Defaults(items []Item) map[string]Answer {
	answers := map[string]Answer{}
	for _, item := range items {
		if _, exists := answers[item.ID]; !exists {
			var value any = false
			if item.Choice {
				value = ""
			}
			answers[item.ID] = Answer{Value: value}
		}
		if item.Initial && !item.Consent && !item.Required {
			var value any = true
			if item.Choice {
				value = item.Value
			}
			answers[item.ID] = Answer{Value: value}
		}
	}
	return answers
}

func Selected(item Item, answer Answer) bool {
	if item.Choice {
		return answer.Value == item.Value
	}
	return answer.Value == true
}

func Satisfied(item Item, answer Answer) bool {
	if !answer.Human {
		return false
	}
	if item.Choice {
		value, ok := answer.Value.(string)
		return ok && value != ""
	}
	return answer.Value == true
}
