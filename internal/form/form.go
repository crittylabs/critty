// Package form interprets task lists in attached Markdown documents.
package form

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/mattn/go-shellwords"
	"github.com/yuin/goldmark/v2/ast"
	"github.com/yuin/goldmark/v2/extension"
	"github.com/yuin/goldmark/v2/parser"
	"github.com/yuin/goldmark/v2/text"
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
	// Tags and SelectAll are sorted sets, so reordering keeps fingerprints.
	Tags      []string `json:"tags,omitempty"`
	SelectAll []string `json:"select_all,omitempty"`
	// Text makes a choice accept a note, or another item a text field.
	Text bool `json:"text,omitempty"`
}

type Answer struct {
	Value    any    `json:"value"`
	Human    bool   `json:"human"`
	Author   string `json:"author,omitempty"`
	Revision string `json:"review_revision,omitempty"`
	Round    int    `json:"round,omitempty"`
	// Via names the select-all item whose human operation set this answer.
	Via string `json:"via,omitempty"`
	// Text holds the note on a choice with text, kept as a draft while
	// another choice is selected.
	Text string `json:"text,omitempty"`
}

// Parse uses the Markdown AST so examples, escaped lists, and quotations are inert.
func Parse(source string) ([]Item, error) {
	data := []byte(source)
	root := parser.New(parser.WithExtensions(extension.TaskListItemParser)).Parse(data)
	var items []Item
	err := ast.Walk(root, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		status, ok := extension.TaskStatusOf(n)
		if !entering || !ok {
			return ast.WalkContinue, nil
		}
		for p := n.Parent(); p != nil; p = p.Parent() {
			if _, quoted := p.(*ast.Blockquote); quoted {
				return ast.WalkContinue, nil
			}
		}
		block := n.FirstChild()
		segments := block.(ast.BlockNode).Source()
		start := segments[0].Start
		item := Item{Line: strings.Count(source[:start], "\n") + 1, Offset: start, Initial: status == extension.TaskStatusCompleted}
		end := segments[len(segments)-1].Stop
		for end > start && (source[end-1] == '\n' || source[end-1] == '\r') {
			end--
		}
		item.EndLine = strings.Count(source[:end], "\n") + 1
		var label strings.Builder
		label.Write(text.NewLines(segments).Bytes(data))
		err := ast.Walk(block, func(child ast.Node, enter bool) (ast.WalkStatus, error) {
			html, ok := child.(*ast.RawHTML)
			if !enter || !ok {
				return ast.WalkContinue, nil
			}
			raw := html.Value.Str(data)
			comment := strings.TrimSpace(raw)
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
			for _, seg := range html.Value.Indices() {
				item.Metadata = append(item.Metadata, Span{Start: seg.Start, End: seg.Stop})
			}
			labelText := label.String()
			label.Reset()
			label.WriteString(strings.Replace(labelText, raw, "", 1))
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
			if block, ok := child.(ast.BlockNode); ok {
				segments := block.Source()
				switch block := block.(type) {
				case *ast.CodeBlock:
					segments = block.Value.Segments()
				case *ast.HTMLBlock:
					segments = block.Value.Segments()
				}
				definition.Write(text.NewLines(segments).Bytes(data))
			}
			return ast.WalkContinue, nil
		}); err != nil {
			return ast.WalkStop, err
		}
		item.Definition = definition.String()
		if item.SelectAll != nil && (item.Required || item.Consent || item.Choice || item.Text) {
			return ast.WalkStop, fmt.Errorf("line %d: select-all cannot be required, consent, text, or have a value", item.Line)
		}
		if item.Text && (item.Consent || !item.Choice && item.Tags != nil) {
			return ast.WalkStop, fmt.Errorf("line %d: text fields cannot be consent or have tags", item.Line)
		}
		if item.ID == "" {
			if item.Required || item.Consent || item.Choice || item.SelectAll != nil {
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
	if _, err := Targets(items); err != nil {
		return nil, err
	}
	return items, nil
}

func tagList(key, value string) ([]string, error) {
	seen := map[string]bool{}
	var tags []string
	for tag := range strings.SplitSeq(value, ",") {
		tag = strings.Trim(tag, " \t\n\r\f\v")
		if !validTag(tag) {
			return nil, fmt.Errorf("invalid tag %q in %s", tag, key)
		}
		if !seen[tag] {
			seen[tag] = true
			tags = append(tags, tag)
		}
	}
	slices.Sort(tags)
	return tags, nil
}

func validTag(tag string) bool {
	return tag != "" && strings.IndexFunc(tag, func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("_-.", r))
	}) < 0
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
		case key == "text" && !assigned:
			item.Text = true
		case key == "tags" && assigned:
			if item.Tags, err = tagList(key, value); err != nil {
				return err
			}
		case key == "select-all" && assigned:
			if item.SelectAll, err = tagList(key, value); err != nil {
				return err
			}
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
		if item.SelectAll != nil {
			continue
		}
		if _, exists := answers[item.ID]; !exists {
			var value any = false
			if item.Choice || item.Text {
				value = ""
			}
			answers[item.ID] = Answer{Value: value}
		}
		if item.Initial && !item.Consent && !item.Required && (item.Choice || !item.Text) {
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
	switch {
	case item.Choice:
		return answer.Value == item.Value
	case item.Text:
		value, ok := answer.Value.(string)
		return ok && value != ""
	}
	return answer.Value == true
}

// IsTextField reports whether item is a text field rather than a choice
// accepting a note.
func IsTextField(item Item) bool {
	return item.Text && !item.Choice
}

func Satisfied(item Item, answer Answer) bool {
	if !answer.Human {
		return false
	}
	if item.Choice || item.Text {
		value, ok := answer.Value.(string)
		return ok && value != ""
	}
	return answer.Value == true
}

type Heading struct {
	Line int
	Text string
}

// Headings lists Markdown headings in source order so form items can be
// grouped by the section that asks them.
func Headings(source string) ([]Heading, error) {
	data := []byte(source)
	var headings []Heading
	err := ast.Walk(parser.New().Parse(data), func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		heading, ok := n.(*ast.Heading)
		if !entering || !ok {
			return ast.WalkContinue, nil
		}
		segments := heading.Source()
		if len(segments) == 0 {
			return ast.WalkSkipChildren, nil
		}
		title := strings.TrimSpace(string(text.NewLines(segments).Bytes(data)))
		headings = append(headings, Heading{Line: strings.Count(source[:segments[0].Start], "\n") + 1, Text: title})
		return ast.WalkSkipChildren, nil
	})
	return headings, err
}
