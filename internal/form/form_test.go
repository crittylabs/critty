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
	if len(items) != 4 {
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
	if items[3].Value != "large size" || !strings.Contains(items[3].Label, "continued label") {
		t.Fatalf("multiline/quoted value: %+v", items[3])
	}
	defaults := Defaults(items)
	if Selected(items[0], defaults["confirm"]) || Selected(items[1], defaults["publish"]) {
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

func TestOnlyAnnotatedTaskItemsAreForms(t *testing.T) {
	source := "- [ ] Pending\n- [x] Done <!-- ordinary comment -->\n" +
		"- [ ] Literal `<!-- form: id=literal -->`\n" +
		"- [ ] Parent\n  - [x] Child <!-- form: -->\n" +
		"- [ ] Active <!-- form: id=active -->\n  - [ ] Inert child\n"
	items, err := Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || items[0].ID != "auto:5" || items[1].ID != "active" {
		t.Fatalf("interactive items: %+v", items)
	}
	if !Selected(items[0], Defaults(items)["auto:5"]) {
		t.Fatal("empty annotation lost the source default")
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
	if tree, ranked := RankTree(items, "r", defaults["r"]); ranked || !slices.Equal(tree, []RankNode{{Value: "a"}, {Value: "b"}}) || Satisfied(items[0], defaults["r"]) {
		t.Fatalf("initial ranking: %v %v", tree, ranked)
	}
	answer := Answer{Value: []any{"b", "a"}, Human: true}
	if tree, ranked := RankTree(items, "r", answer); !ranked || !slices.Equal(tree, []RankNode{{Value: "b"}, {Value: "a"}}) || !Satisfied(items[1], answer) {
		t.Fatalf("decoded ranking: %v %v", tree, ranked)
	}
	if _, ranked := RankTree(items, "r", Answer{Value: []any{"a", "c"}}); ranked {
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

func TestPathInputs(t *testing.T) {
	for _, tt := range []struct {
		name, annotation string
		kind             InputType
		standalone       bool
		value, text      any
	}{
		{"file", `type=file default='my file.md'`, InputFile, true, "my file.md", ""},
		{"directory", `type=directory default='~/日本語'`, InputDirectory, true, "~/日本語", ""},
		{"path", `type=path`, InputPath, true, "", ""},
		{"choice", `value=custom type=directory default='/my docs'`, InputDirectory, false, "", ""},
		{"checkbox", `checkbox type=file default='$HOME/report.md' tags=docs`, InputFile, false, false, "$HOME/report.md"},
		{"multiline checkbox", `checkbox text default='First line'`, "", false, false, "First line"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			items, err := Parse("- [ ] Input <!-- form: id=target " + tt.annotation + " -->\n")
			if err != nil {
				t.Fatal(err)
			}
			item := items[0]
			answer := Defaults(items)["target"]
			if item.Type != tt.kind || IsTextField(item) != tt.standalone || answer.Value != tt.value || answer.Text != tt.text {
				t.Fatalf("item=%+v answer=%+v", item, answer)
			}
			if answer.Human || Satisfied(item, answer) || Selected(item, answer) {
				t.Fatal("author's default counted as a human answer")
			}
		})
	}
	for _, annotation := range []string{
		`type=unknown`, `type=file text`, `type=file consent`, `checkbox`,
		`checkbox type=file value=custom`, `default=hello`, `type=file tags=docs`,
		`existing`, `text existing`, `type=path existing=true`, `type=path rank value=a`, "type=path default='first\nsecond'",
	} {
		t.Run(annotation, func(t *testing.T) {
			if _, err := Parse("- [ ] Input <!-- form: id=target " + annotation + " -->\n"); err == nil {
				t.Fatal("accepted invalid input definition")
			}
		})
	}
}

func TestAssociatedInputDrafts(t *testing.T) {
	items, err := Parse("- [ ] Notes <!-- form: id=where value=notes type=directory default='/notes' tags=preferred -->\n" +
		"- [ ] Project <!-- form: id=where value=project type=directory default='/project' -->\n" +
		"- [ ] History <!-- form: id=where value=history -->\n" +
		"- [ ] Copy <!-- form: id=copy checkbox type=file default='/copy.md' tags=preferred -->\n" +
		"- [ ] All <!-- form: id=all select-all=preferred -->\n")
	if err != nil {
		t.Fatal(err)
	}
	answers := Defaults(items)
	human := Answer{Human: true, Author: "Human"}
	answers["where"] = SelectAnswer(items, items[0], answers["where"], true, human)
	if answers["where"].Text != "/notes" {
		t.Fatal("first choice lost its default")
	}
	answers["where"] = Answer{Value: "notes", Text: "/custom notes", Human: true}
	answers["where"] = SelectAnswer(items, items[1], answers["where"], true, human)
	if answers["where"].Text != "/project" {
		t.Fatal("choice inherited another option's input")
	}
	cleared := answers["where"]
	cleared.Text = "" // An explicitly cleared value stays empty.
	answers["where"] = cleared
	answers["where"] = SelectAnswer(items, items[0], answers["where"], true, human)
	if answers["where"].Text != "/custom notes" {
		t.Fatal("choice draft was not restored")
	}
	previous := answers["where"]
	answers["where"] = SelectAnswer(items, items[2], answers["where"], true, human)
	if answers["where"].Text != "" || len(previous.Drafts) != 2 {
		t.Fatal("plain choice leaked a note or changed the previous answer")
	}
	answers["where"] = SelectAnswer(items, items[1], answers["where"], true, human)
	if answers["where"].Text != "" {
		t.Fatal("cleared draft was replaced by the default")
	}
	targets, err := Targets(items)
	if err != nil {
		t.Fatal(err)
	}
	ApplyBulk(items, targets[4], answers, human)
	if answers["where"].Text != "/custom notes" || answers["copy"].Text != "/copy.md" || answers["copy"].Value != true {
		t.Fatalf("bulk selection lost associated input: %+v", answers)
	}
	ApplyBulk(items, targets[4], answers, human)
	if answers["copy"].Value != false || answers["copy"].Text != "/copy.md" {
		t.Fatal("clearing the checkbox lost its draft")
	}
}

func TestExistingPathDefinition(t *testing.T) {
	items, err := Parse("- [ ] Input <!-- form: id=input type=file existing -->\n")
	if err != nil || len(items) != 1 || !items[0].Existing {
		t.Fatalf("existing path: items=%+v err=%v", items, err)
	}
	before, err := Fingerprint(items, "input")
	if err != nil {
		t.Fatal(err)
	}
	items[0].Existing = false
	after, err := Fingerprint(items, "input")
	if err != nil {
		t.Fatal(err)
	}
	if before == after {
		t.Fatal("existing requirement did not change the definition")
	}
}

func TestRankTreeRejectsInvalidAnswers(t *testing.T) {
	t.Parallel()
	items, err := Parse(`- [ ] First <!-- form: id=rank rank value=first -->
  - [ ] Child <!-- form: id=rank rank value=child -->
- [ ] Next <!-- form: id=rank rank value=next -->`)
	if err != nil {
		t.Fatal(err)
	}
	source := []RankNode{{Value: "first"}, {Value: "child", Parent: "first", Depth: 1}, {Value: "next"}}
	for _, tt := range []struct {
		name    string
		order   []string
		parents map[string]string
	}{
		{"duplicate", []string{"first", "first", "next"}, nil},
		{"self parent", []string{"first", "child", "next"}, map[string]string{"child": "child"}},
		{"later parent", []string{"first", "child", "next"}, map[string]string{"child": "next"}},
		{"separated from parent", []string{"first", "next", "child"}, map[string]string{"child": "first"}},
		{"unknown child", []string{"first", "child", "next"}, map[string]string{"other": "first"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			want := Answer{Value: tt.order, RankParents: tt.parents}
			if tree, ranked := RankTree(items, "rank", want); ranked || !slices.Equal(tree, source) {
				t.Fatalf("invalid answer produced tree %v, ranked %t", tree, ranked)
			}
		})
	}
}

func TestRankConstraints(t *testing.T) {
	t.Parallel()
	items, err := Parse(`- [ ] Inbox <!-- form: id=r rank value=inbox fixed max-depth=1 -->
  - [ ] Triage <!-- form: id=r rank value=triage -->
- [ ] Review <!-- form: id=r rank value=review -->
  - [ ] Verify <!-- form: id=r rank value=verify fixed -->
- [ ] Later <!-- form: id=r rank value=later fixed max-depth=0 -->
`)
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name    string
		order   []string
		parents map[string]string
		want    string
	}{
		{"fixed descendant follows branch", []string{"review", "verify", "inbox", "triage", "later"},
			map[string]string{"verify": "review", "triage": "inbox"}, ""},
		{"one level allowed", []string{"inbox", "triage", "review", "verify", "later"},
			map[string]string{"triage": "inbox", "verify": "review"}, ""},
		{"ancestor limit", []string{"inbox", "triage", "review", "verify", "later"},
			map[string]string{"triage": "inbox", "review": "inbox", "verify": "review"}, "Inbox"},
		{"leaf limit", []string{"inbox", "review", "verify", "later", "triage"},
			map[string]string{"verify": "review", "triage": "later"}, "Later"},
		{"fixed parent", []string{"inbox", "triage", "review", "verify", "later"},
			map[string]string{"triage": "inbox"}, "Verify"},
		{"fixed order", []string{"later", "inbox", "triage", "review", "verify"},
			map[string]string{"triage": "inbox", "verify": "review"}, "Inbox"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			answer := Answer{Value: tt.order, RankParents: tt.parents}
			err := CheckRank(items, "r", answer)
			switch {
			case tt.want == "" && err == nil:
			case tt.want != "" && err != nil && strings.Contains(err.Error(), tt.want):
			default:
				t.Fatalf("validation error = %v, want %q", err, tt.want)
			}
		})
	}
	for _, source := range []string{
		"- [ ] A <!-- form: fixed -->",
		"- [ ] A <!-- form: id=r rank value=a max-depth=-1 -->\n- [ ] B <!-- form: id=r rank value=b -->",
		"- [ ] A <!-- form: id=r rank value=a max-depth=0 -->\n  - [ ] B <!-- form: id=r rank value=b -->",
	} {
		if _, err := Parse(source); err == nil {
			t.Errorf("parsed invalid constraints: %q", source)
		}
	}
}

func TestRankLevelsOfMovedDescendants(t *testing.T) {
	t.Parallel()
	const source = `- [ ] Root <!-- form: id=r rank value=root -->
  - [ ] Branch <!-- form: id=r rank value=branch -->
    - [ ] Leaf <!-- form: id=r rank value=leaf min-level=3 max-level=3 -->
  - [ ] Peer <!-- form: id=r rank value=peer -->
`
	items, err := Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	answer := Answer{Value: []string{"root", "peer", "branch", "leaf"}, RankParents: map[string]string{"peer": "root", "leaf": "branch"}}
	if err := CheckRank(items, "r", answer); err == nil || !strings.Contains(err.Error(), "Leaf") {
		t.Fatalf("promoting its ancestor returned %v", err)
	}
	answer.RankParents["branch"] = "peer"
	if err := CheckRank(items, "r", answer); err == nil || !strings.Contains(err.Error(), "shallower") {
		t.Fatalf("indenting its ancestor returned %v", err)
	}
	answer.RankParents["leaf"] = "peer"
	if err := CheckRank(items, "r", answer); err != nil {
		t.Fatalf("reparenting at the same level failed: %v", err)
	}
	for _, invalid := range []string{
		"min-level=0", "min-level=4", "min-level=none",
		"min-level=3 max-level=0", "min-level=3 max-level=2", "min-level=3 max-level=none",
	} {
		if _, err := Parse(strings.Replace(source, "min-level=3 max-level=3", invalid, 1)); err == nil {
			t.Errorf("accepted %s", invalid)
		}
	}
}
