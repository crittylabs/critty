package form

import (
	"maps"
	"slices"
	"strings"
	"testing"
)

func TestParseMarkdownForms(t *testing.T) {
	source := "# Form\n\n- [ ] ordinary\n- [x] Confirm <!-- form: id=confirm required -->\n- [x] Publish <!-- form: id=publish consent -->\n- [ ] Small <!-- form: id=size value=small required -->\n- [ ] Large <!-- form: id=size value='large size' required -->\n  continued label\n\n> - [ ] quoted <!-- form: id=quote required -->\n\n```md\n- [ ] code <!-- form: invalid -->\n```\n\n\\- [ ] escaped\n"
	items, err := Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 5 {
		t.Fatalf("items: %+v", items)
	}
	for _, item := range items {
		if source[item.Offset:item.Offset+3] != "[ ]" && source[item.Offset:item.Offset+3] != "[x]" {
			t.Fatalf("wrong offset: %+v", item)
		}
		if strings.Contains(item.Label, "form:") {
			t.Fatalf("metadata in label: %+v", item)
		}
	}
	if items[4].Value != "large size" || !strings.Contains(items[4].Label, "continued label") {
		t.Fatalf("multiline/quoted value: %+v", items[4])
	}
	defaults := Defaults(items)
	if Selected(items[1], defaults["confirm"]) || Selected(items[2], defaults["publish"]) {
		t.Fatal("source defaults fulfilled confirmation or consent")
	}
}

func TestInvalidForms(t *testing.T) {
	for _, source := range []string{
		"- [ ] A <!-- form: required -->",
		"- [ ] A <!-- form: id=a invalid -->",
		"- [ ] A <!-- form: id=a; consent -->",
		"- [ ] A <!-- form: id=a required=true -->",
		"- [ ] A <!-- form: id=a id=b -->",
		"- [ ] A <!-- form: id=a -->\n- [ ] B <!-- form: id=a -->",
		"- [ ] A <!-- form: id=a -->\n- [ ] B <!-- form: id=a value=b -->",
		"- [ ] A <!-- form: id=a value=b -->\n- [ ] B <!-- form: id=a value=b -->",
		"- [ ] A <!-- form: id=a value=b required -->\n- [ ] B <!-- form: id=a value=c -->",
		"- [x] A <!-- form: id=a value=b -->\n- [x] B <!-- form: id=a value=c -->",
		"- [ ] A <!-- form: id=a value=b consent -->",
	} {
		t.Run(source, func(t *testing.T) {
			if _, err := Parse(source); err == nil {
				t.Fatal("accepted invalid definition")
			}
		})
	}
}

func TestMultilineMetadataAndNestedItems(t *testing.T) {
	source := "1. [ ] Parent <!-- form:\n   id=parent required -->\n   continued\n   - [ ] Child <!-- form: id=child -->\n\n   Another paragraph.\n"
	items, err := Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || items[0].ID != "parent" || items[1].ID != "child" || strings.Contains(items[0].Definition, "id=child") {
		t.Fatalf("nested definitions: %+v", items)
	}
	for _, item := range items {
		if strings.Contains(item.Label, "form:") {
			t.Fatalf("multiline metadata in label: %q", item.Label)
		}
	}
}

func TestSelectAllDefinitions(t *testing.T) {
	source := "- [x] All <!-- form: id=all select-all=sections -->\n" +
		"- [ ] API <!-- form: id=all-api tags=sections select-all=api -->\n" +
		"- [ ] UI <!-- form: id=all-ui tags='sections' select-all=\"ui, api\" -->\n" +
		"- [ ] Auth <!-- form: id=auth tags=security,api,api -->\n" +
		"- [ ] Keys <!-- form: id=keys tags=ui -->\n" +
		"- [ ] Small <!-- form: id=size value=small tags=ui -->\n" +
		"- [ ] Large <!-- form: id=size value=large -->\n"
	items, err := Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(items[3].Tags, []string{"api", "security"}) || !slices.Equal(items[2].SelectAll, []string{"api", "ui"}) {
		t.Fatalf("tag sets: %+v", items)
	}
	targets, err := Targets(items)
	if err != nil {
		t.Fatal(err)
	}
	want := map[int][]int{0: {3, 4, 5}, 1: {3}, 2: {3, 4, 5}}
	if !maps.EqualFunc(targets, want, slices.Equal) {
		t.Fatalf("targets: %v", targets)
	}
	defaults := Defaults(items)
	if _, ok := defaults["all"]; ok {
		t.Fatal("select-all item has an answer")
	}
	if CountSelected(items, targets[0], defaults) != 0 {
		t.Fatal("initial select-all mark selected targets")
	}
}

func TestInvalidSelectAll(t *testing.T) {
	for _, source := range []string{
		"- [ ] A <!-- form: select-all=a -->\n- [ ] B <!-- form: tags=a -->",
		"- [ ] A <!-- form: id=all select-all=a required -->\n- [ ] B <!-- form: tags=a -->",
		"- [ ] A <!-- form: id=all select-all=a value=x -->\n- [ ] B <!-- form: tags=a -->",
		"- [ ] A <!-- form: id=all select-all=a consent -->\n- [ ] B <!-- form: tags=a -->",
		"- [ ] A <!-- form: id=all select-all=a tags=a -->",
		"- [ ] A <!-- form: id=all select-all=a,b -->\n- [ ] B <!-- form: tags=a -->",
		"- [ ] A <!-- form: tags= -->",
		"- [ ] A <!-- form: tags=a,,b -->",
		"- [ ] A <!-- form: tags=a* -->",
		"- [ ] A <!-- form: tags=a tags=b -->",
		"- [ ] A <!-- form: id=a select-all=b tags=a -->\n- [ ] B <!-- form: id=b select-all=a tags=b -->",
		"- [ ] All <!-- form: id=all select-all=q -->\n- [ ] A <!-- form: id=q value=a tags=q -->\n- [ ] B <!-- form: id=q value=b tags=q -->",
	} {
		t.Run(source, func(t *testing.T) {
			if _, err := Parse(source); err == nil {
				t.Fatal("accepted invalid definition")
			}
		})
	}
}

func TestApplyBulk(t *testing.T) {
	items, err := Parse("- [ ] All <!-- form: id=all select-all=t -->\n" +
		"- [ ] A <!-- form: id=a tags=t consent -->\n" +
		"- [ ] B <!-- form: id=b tags=t -->\n" +
		"- [ ] Yes <!-- form: id=q value=yes tags=t -->\n" +
		"- [ ] No <!-- form: id=q value=no -->\n")
	if err != nil {
		t.Fatal(err)
	}
	targets, err := Targets(items)
	if err != nil {
		t.Fatal(err)
	}
	answers := Defaults(items)
	answers["b"] = Answer{Value: true, Human: true}
	human := Answer{Human: true, Author: "Human", Via: "all"}
	ApplyBulk(items, targets[0], answers, human)
	if CountSelected(items, targets[0], answers) != 3 || !Satisfied(items[1], answers["a"]) || answers["b"].Via != "all" || answers["q"].Value != "yes" {
		t.Fatalf("partial selection was not completed: %+v", answers)
	}
	ApplyBulk(items, targets[0], answers, human)
	if CountSelected(items, targets[0], answers) != 0 || answers["q"].Value != "" {
		t.Fatalf("full selection was not cleared: %+v", answers)
	}
	answers["q"] = Answer{Value: "no", Human: true}
	ApplyBulk(items, targets[0], answers, human)
	if answers["q"].Value != "yes" {
		t.Fatal("selection did not replace another choice")
	}
}

func TestTextFields(t *testing.T) {
	items, err := Parse("- [x] Reason <!-- form: id=reason text required -->\n- [ ] Other <!-- form: id=q value=other text tags=t -->\n- [ ] Plain <!-- form: id=q value=plain -->\n")
	if err != nil {
		t.Fatal(err)
	}
	if !IsTextField(items[0]) || IsTextField(items[1]) {
		t.Fatalf("text kinds: %+v", items)
	}
	defaults := Defaults(items)
	if defaults["reason"].Value != "" || Selected(items[0], defaults["reason"]) {
		t.Fatal("initial mark answered a text field")
	}
	if !Satisfied(items[0], Answer{Value: "because", Human: true}) || Satisfied(items[0], Answer{Value: "", Human: true}) {
		t.Fatal("text field satisfaction")
	}
	for _, source := range []string{
		"- [ ] A <!-- form: id=a text consent -->",
		"- [ ] A <!-- form: id=a text tags=t -->",
		"- [ ] All <!-- form: id=all select-all=t text -->\n- [ ] B <!-- form: tags=t -->",
		"- [ ] A <!-- form: id=a text=yes -->",
	} {
		if _, err := Parse(source); err == nil {
			t.Errorf("accepted %q", source)
		}
	}
}

func TestRankFields(t *testing.T) {
	items, err := Parse("1. [ ] A <!-- form: id=r rank value=a required -->\n2. [x] B <!-- form: id=r rank value=b required -->\n")
	if err != nil {
		t.Fatal(err)
	}
	defaults := Defaults(items)
	if order, ranked := RankOrder(items, "r", defaults["r"]); ranked || !slices.Equal(order, []string{"a", "b"}) || Satisfied(items[0], defaults["r"]) {
		t.Fatalf("initial ranking: %v %v", order, ranked)
	}
	answer := Answer{Value: []any{"b", "a"}, Human: true}
	if order, ranked := RankOrder(items, "r", answer); !ranked || !slices.Equal(order, []string{"b", "a"}) || !Satisfied(items[1], answer) {
		t.Fatalf("decoded ranking: %v %v", order, ranked)
	}
	if _, ranked := RankOrder(items, "r", Answer{Value: []any{"a", "c"}}); ranked {
		t.Fatal("accepted a ranking of other values")
	}
	for _, source := range []string{
		"- [ ] A <!-- form: id=r rank value=a -->",
		"- [ ] A <!-- form: id=r rank -->\n- [ ] B <!-- form: id=r rank -->",
		"- [ ] A <!-- form: rank value=a -->\n- [ ] B <!-- form: rank value=b -->",
		"- [ ] A <!-- form: id=r rank value=a -->\n- [ ] B <!-- form: id=r value=b -->",
		"- [ ] A <!-- form: id=r rank value=a -->\n- [ ] B <!-- form: id=r rank value=a -->",
		"- [ ] A <!-- form: id=r rank value=a text -->\n- [ ] B <!-- form: id=r rank value=b -->",
		"- [ ] A <!-- form: id=r rank value=a tags=t -->\n- [ ] B <!-- form: id=r rank value=b -->",
	} {
		if _, err := Parse(source); err == nil {
			t.Errorf("accepted %q", source)
		}
	}
}
