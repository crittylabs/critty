package form

import (
	"fmt"
	"slices"
	"strings"
)

// Targets maps the index of each select-all item to the indices of the
// answerable items it controls, following nested select-all items.
func Targets(items []Item) (map[int][]int, error) {
	direct := map[int][]int{}
	for i, item := range items {
		if item.SelectAll == nil {
			continue
		}
		for _, tag := range item.SelectAll {
			found := false
			for j, other := range items {
				if j != i && slices.Contains(other.Tags, tag) {
					found = true
					if !slices.Contains(direct[i], j) {
						direct[i] = append(direct[i], j)
					}
				}
			}
			if !found {
				return nil, fmt.Errorf("line %d: select-all tag %q matches no other item", item.Line, tag)
			}
		}
	}
	targets := map[int][]int{}
	const (
		visiting = iota + 1
		done
	)
	state := map[int]int{}
	var path []int
	var visit func(int) error
	visit = func(i int) error {
		switch state[i] {
		case done:
			return nil
		case visiting:
			cycle := path[slices.Index(path, i):]
			var steps []string
			for _, j := range append(cycle, i) {
				steps = append(steps, fmt.Sprintf("%s (line %d)", items[j].ID, items[j].Line))
			}
			return fmt.Errorf("line %d: select-all cycle: %s", items[i].Line, strings.Join(steps, " -> "))
		}
		state[i] = visiting
		path = append(path, i)
		var leaves []int
		for _, j := range direct[i] {
			if items[j].SelectAll == nil {
				leaves = append(leaves, j)
				continue
			}
			if err := visit(j); err != nil {
				return err
			}
			leaves = append(leaves, targets[j]...)
		}
		slices.Sort(leaves)
		leaves = slices.Compact(leaves)
		choices := map[string]int{}
		for _, j := range leaves {
			if !items[j].Choice {
				continue
			}
			if k, ok := choices[items[j].ID]; ok {
				return fmt.Errorf("line %d: select-all %q targets multiple choices of %q (lines %d and %d)",
					items[i].Line, items[i].ID, items[j].ID, items[k].Line, items[j].Line)
			}
			choices[items[j].ID] = j
		}
		targets[i] = leaves
		path = path[:len(path)-1]
		state[i] = done
		return nil
	}
	for i := range items {
		if items[i].SelectAll != nil {
			if err := visit(i); err != nil {
				return nil, err
			}
		}
	}
	return targets, nil
}

// CountSelected reports how many of the target items are selected.
func CountSelected(items []Item, targets []int, answers map[string]Answer) int {
	count := 0
	for _, i := range targets {
		if Selected(items[i], answers[items[i].ID]) {
			count++
		}
	}
	return count
}

// ApplyBulk selects every target, or deselects them when all are selected.
// Each answer copies the provenance in answer.
func ApplyBulk(items []Item, targets []int, answers map[string]Answer, answer Answer) {
	selecting := CountSelected(items, targets, answers) < len(targets)
	for _, i := range targets {
		item := items[i]
		switch {
		case item.Choice && selecting:
			answer.Value = item.Value
		case item.Choice:
			// Every target choice is selected, so no other choice is lost.
			answer.Value = ""
		default:
			answer.Value = selecting
		}
		answer.Text = answers[item.ID].Text
		answers[item.ID] = answer
	}
}
