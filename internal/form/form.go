// Package form interprets task lists in attached Markdown documents.
package form

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strconv"
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

type InputType string

const (
	InputFile      InputType = "file"
	InputDirectory InputType = "directory"
	InputPath      InputType = "path"
)

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
	// Text adds string input to a control, or makes a standalone input field.
	Text bool `json:"text,omitempty"`
	// Type selects single-line path input; Text remains true for string fields.
	Type     InputType `json:"type,omitempty"`
	Default  string    `json:"default,omitempty"`
	Existing bool      `json:"existing,omitempty"`
	// Checkbox keeps a boolean control alongside an associated input.
	Checkbox bool `json:"checkbox,omitempty"`
	// Rank makes items sharing an ID one field ordered by the reviewer.
	Rank         bool   `json:"rank,omitempty"`
	RankParent   string `json:"rank_parent,omitempty"`
	RankFixed    bool   `json:"rank_fixed,omitempty"`
	RankMaxDepth *int   `json:"rank_max_depth,omitempty"`
	RankMinLevel int    `json:"rank_min_level,omitempty"`
	RankMaxLevel int    `json:"rank_max_level,omitempty"`
}

type Answer struct {
	Value    any    `json:"value"`
	Human    bool   `json:"human"`
	Author   string `json:"author,omitempty"`
	Revision string `json:"review_revision,omitempty"`
	Round    int    `json:"round,omitempty"`
	// Via names the select-all item whose human operation set this answer.
	Via string `json:"via,omitempty"`
	// Text holds the current associated input; unchecked checkbox input
	// remains a draft until that checkbox is selected again.
	Text string `json:"text,omitempty"`
	// Drafts holds per-choice input values while another option is selected.
	Drafts map[string]string `json:"drafts,omitempty"`
	// RankParents maps each ranked child to the value of its parent.
	RankParents map[string]string `json:"rank_parents,omitempty"`
}

// Parse uses the Markdown AST so examples, escaped lists, and quotations are inert.
func Parse(source string) ([]Item, error) {
	data := []byte(source)
	root := parser.New(parser.WithExtensions(extension.TaskListItemParser)).Parse(data)
	var items []Item
	rankItems := map[ast.Node]Item{}
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
		if len(item.Metadata) == 0 {
			return ast.WalkContinue, nil
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
		if item.Rank {
			if !item.Choice || item.Consent || item.Text || item.Tags != nil || item.SelectAll != nil {
				return ast.WalkStop, fmt.Errorf("line %d: rank requires a value and cannot be consent, text, or tagged", item.Line)
			}
			item.Choice = false
			for parent := n.Parent(); parent != nil; parent = parent.Parent() {
				if ranked, ok := rankItems[parent]; ok && ranked.ID == item.ID {
					item.RankParent = ranked.Value
					break
				}
			}
			rankItems[n] = item
		}
		if item.SelectAll != nil && (item.Required || item.Consent || item.Choice || item.Text) {
			return ast.WalkStop, fmt.Errorf("line %d: select-all cannot be required, consent, text, or have a value", item.Line)
		}
		if IsTextField(item) && (item.Consent || item.Tags != nil) {
			return ast.WalkStop, fmt.Errorf("line %d: text fields cannot be consent or have tags", item.Line)
		}
		if item.Checkbox && (!item.Text || item.Choice) {
			return ast.WalkStop, fmt.Errorf("line %d: checkbox requires an input and cannot have a value", item.Line)
		}
		if item.ID == "" {
			if item.Required || item.Consent || item.Choice || item.Rank || item.SelectAll != nil {
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
			if old.Rank || item.Rank {
				if !old.Rank || !item.Rank || old.Value == item.Value || old.Required != item.Required {
					return nil, fmt.Errorf("line %d: inconsistent ranking for %q", item.Line, item.ID)
				}
				continue
			}
			if !old.Choice || !item.Choice {
				return nil, fmt.Errorf("line %d: duplicate boolean id %q", item.Line, item.ID)
			}
			if old.Value == item.Value || old.Required != item.Required || (old.Initial && item.Initial) {
				return nil, fmt.Errorf("line %d: inconsistent choices for %q", item.Line, item.ID)
			}
		}
		groups[item.ID] = append(groups[item.ID], item)
	}
	for id, group := range groups {
		if !group[0].Rank {
			continue
		}
		if len(group) < 2 {
			return nil, fmt.Errorf("line %d: ranking %q needs at least two items", group[0].Line, id)
		}
		nodes, definitions := rankSource(group, id)
		if err := validateRankConstraints(nodes, definitions); err != nil {
			return nil, fmt.Errorf("ranking %q: %w", id, err)
		}
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
		case key == "existing" && !assigned:
			item.Existing = true
		case key == "checkbox" && !assigned:
			item.Checkbox = true
		case key == "type" && assigned:
			switch InputType(value) {
			case InputFile, InputDirectory, InputPath:
				item.Type, item.Text = InputType(value), true
			default:
				return fmt.Errorf("invalid input type %q", value)
			}
		case key == "default" && assigned:
			item.Default = value
		case key == "rank" && !assigned:
			item.Rank = true
		case key == "fixed" && !assigned:
			item.RankFixed = true
		case key == "max-depth" && assigned:
			depth, err := strconv.Atoi(value)
			if err != nil || depth < 0 {
				return fmt.Errorf("max-depth must be a non-negative integer")
			}
			item.RankMaxDepth = &depth
		case (key == "min-level" || key == "max-level") && assigned:
			level, err := strconv.Atoi(value)
			if err != nil || level < 1 {
				return fmt.Errorf("%s must be a positive integer", key)
			}
			if key == "min-level" {
				item.RankMinLevel = level
			} else {
				item.RankMaxLevel = level
			}
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
	if !item.Rank && (item.RankFixed || item.RankMaxDepth != nil || item.RankMinLevel != 0 || item.RankMaxLevel != 0) {
		return fmt.Errorf("fixed, max-depth, min-level, and max-level require rank")
	}
	if item.RankMaxLevel != 0 && item.RankMinLevel > item.RankMaxLevel {
		return fmt.Errorf("min-level cannot exceed max-level")
	}
	if seen["type"] && seen["text"] {
		return fmt.Errorf("type and text cannot be combined")
	}
	if item.Existing && item.Type == "" {
		return fmt.Errorf("existing requires a path input")
	}
	if seen["default"] && !item.Text {
		return fmt.Errorf("default requires a text or path input")
	}
	if item.Type != "" && strings.ContainsAny(item.Default, "\r\n") {
		return fmt.Errorf("path default must be a single line")
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
			switch {
			case item.Rank:
				value = nil
			case item.Choice || IsTextField(item):
				value = ""
				if IsTextField(item) {
					value = item.Default
				}
			}
			answers[item.ID] = Answer{Value: value}
		}
		if item.Checkbox {
			answer := answers[item.ID]
			answer.Text = item.Default
			answers[item.ID] = answer
		}
		if item.Initial && !item.Consent && !item.Required && !item.Rank && !IsTextField(item) {
			var value any = true
			if item.Choice {
				value = item.Value
			}
			answers[item.ID] = Answer{Value: value, Text: item.Default}
		}
	}
	return answers
}

func Selected(item Item, answer Answer) bool {
	switch {
	case item.Rank:
		return len(rankValues(answer.Value)) > 0
	case item.Choice:
		return answer.Value == item.Value
	case IsTextField(item):
		value, ok := answer.Value.(string)
		return answer.Human && ok && value != ""
	}
	return answer.Value == true
}

// IsTextField distinguishes standalone string input from associated input.
func IsTextField(item Item) bool {
	return item.Text && !item.Choice && !item.Checkbox
}

func Satisfied(item Item, answer Answer) bool {
	if !answer.Human {
		return false
	}
	if item.Rank {
		return len(rankValues(answer.Value)) > 0
	}
	if item.Choice || IsTextField(item) {
		value, ok := answer.Value.(string)
		return ok && value != ""
	}
	return answer.Value == true
}

// SelectAnswer preserves associated input when toggling a checkbox or
// switching choices.  Each option keeps its own draft and initial suggestion.
func SelectAnswer(items []Item, item Item, previous Answer, selected bool, answer Answer) Answer {
	answer.Value = selected
	answer.Text = previous.Text
	answer.Drafts = maps.Clone(previous.Drafts)
	if !item.Choice {
		return answer
	}
	if selected {
		answer.Value = item.Value
	} else {
		answer.Value = ""
	}
	textOptions := 0
	for _, old := range items {
		if old.ID != item.ID || !old.Choice || !old.Text {
			continue
		}
		textOptions++
		if Selected(old, previous) {
			if answer.Drafts == nil {
				answer.Drafts = map[string]string{}
			}
			answer.Drafts[old.Value] = previous.Text
		}
	}
	answer.Text = ""
	if selected && item.Text {
		if draft, exists := answer.Drafts[item.Value]; exists {
			answer.Text = draft
		} else if textOptions == 1 && previous.Text != "" {
			// Version 2 retained one shared draft after selecting a plain option.
			answer.Text = previous.Text
		} else {
			answer.Text = item.Default
		}
	}
	return answer
}

// RankNode is an item in display order, with its parent and derived depth.
type RankNode struct {
	Value, Parent string
	Depth         int
}

func rankSource(items []Item, id string) ([]RankNode, map[string]Item) {
	var source []RankNode
	definitions := map[string]Item{}
	depths := map[string]int{"": -1}
	for _, item := range items {
		if item.ID == id && item.Rank {
			depth := depths[item.RankParent] + 1
			source = append(source, RankNode{Value: item.Value, Parent: item.RankParent, Depth: depth})
			depths[item.Value] = depth
			definitions[item.Value] = item
		}
	}
	return source, definitions
}

func decodeRank(definitions map[string]Item, answer Answer) ([]RankNode, error) {
	order := rankValues(answer.Value)
	if len(order) == 0 || len(order) != len(definitions) {
		return nil, fmt.Errorf("incomplete ranking")
	}
	for value, parent := range answer.RankParents {
		if _, found := definitions[value]; !found || parent == "" {
			return nil, fmt.Errorf("invalid parent for %q", value)
		}
	}
	seen := map[string]bool{}
	var ancestors []string
	nodes := make([]RankNode, 0, len(order))
	for _, value := range order {
		if _, ok := definitions[value]; !ok || seen[value] {
			return nil, fmt.Errorf("invalid ranked item %q", value)
		}
		seen[value] = true
		parent := answer.RankParents[value]
		depth := 0
		if parent != "" {
			index := slices.Index(ancestors, parent)
			if index < 0 {
				return nil, fmt.Errorf("invalid parent for %q", value)
			}
			depth = index + 1
		}
		ancestors = append(ancestors[:depth], value)
		nodes = append(nodes, RankNode{Value: value, Parent: parent, Depth: depth})
	}
	return nodes, validateRankConstraints(nodes, definitions)
}

func validateRankConstraints(nodes []RankNode, definitions map[string]Item) error {
	var ancestors []Item
	lastFixed := map[string]int{}
	for _, node := range nodes {
		item := definitions[node.Value]
		if node.Depth+1 < item.RankMinLevel {
			return fmt.Errorf("%q must stay at level %d or deeper", item.Label, item.RankMinLevel)
		}
		if item.RankMaxLevel != 0 && node.Depth+1 > item.RankMaxLevel {
			return fmt.Errorf("%q must stay at level %d or shallower", item.Label, item.RankMaxLevel)
		}
		if item.RankFixed {
			if node.Parent != item.RankParent || item.Line <= lastFixed[node.Parent] {
				return fmt.Errorf("%q is fixed", item.Label)
			}
			lastFixed[node.Parent] = item.Line
		}
		ancestors = ancestors[:node.Depth]
		for depth, parent := range ancestors {
			if limit := parent.RankMaxDepth; limit != nil && node.Depth-depth > *limit {
				return fmt.Errorf("%q allows at most %d levels below it", parent.Label, *limit)
			}
		}
		ancestors = append(ancestors, item)
	}
	return nil
}

// CheckRank validates a proposed order and its parent relationships.
func CheckRank(items []Item, id string, answer Answer) error {
	_, definitions := rankSource(items, id)
	_, err := decodeRank(definitions, answer)
	return err
}

// RankTree returns a complete answer, or the source tree if the answer is incomplete.
func RankTree(items []Item, id string, answer Answer) ([]RankNode, bool) {
	source, definitions := rankSource(items, id)
	if nodes, err := decodeRank(definitions, answer); err == nil {
		return nodes, true
	}
	return source, false
}

// RankOrder returns values in depth-first order.
func RankOrder(items []Item, id string, answer Answer) ([]string, bool) {
	nodes, ranked := RankTree(items, id, answer)
	values := make([]string, len(nodes))
	for i, node := range nodes {
		values[i] = node.Value
	}
	return values, ranked
}

// RankNumbers numbers the siblings at each level of a depth-first ranking.
func RankNumbers(nodes []RankNode) []string {
	numbers := make([]string, len(nodes))
	var levels []int
	for i, node := range nodes {
		if node.Depth == len(levels) {
			levels = append(levels, 0)
		} else {
			levels = levels[:node.Depth+1]
		}
		levels[node.Depth]++
		var number []byte
		for j, part := range levels {
			if j != 0 {
				number = append(number, '-')
			}
			number = strconv.AppendInt(number, int64(part), 10)
		}
		numbers[i] = string(number)
	}
	return numbers
}

// RankLabels formats an answered ranking for the answer list and archive.
func RankLabels(items []Item, id string, answer Answer) []string {
	nodes, ranked := RankTree(items, id, answer)
	if !ranked {
		return nil
	}
	labels := map[string]string{}
	for _, item := range items {
		if item.Rank && item.ID == id {
			labels[item.Value] = item.Label
		}
	}
	numbers := RankNumbers(nodes)
	result := make([]string, len(nodes))
	for i, node := range nodes {
		result[i] = numbers[i] + ". " + labels[node.Value]
		if node.Parent != "" {
			result[i] += " (under " + labels[node.Parent] + ")"
		}
	}
	return result
}

// rankValues accepts answers decoded from JSON as well as those set in memory.
func rankValues(value any) []string {
	switch value := value.(type) {
	case []string:
		return value
	case []any:
		values := make([]string, 0, len(value))
		for _, v := range value {
			text, ok := v.(string)
			if !ok {
				return nil
			}
			values = append(values, text)
		}
		return values
	}
	return nil
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
