package form

import (
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
