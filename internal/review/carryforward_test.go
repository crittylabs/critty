package review

import (
	"strings"
	"testing"
)

func carryOne(t *testing.T, c Comment, prev, next string) Comment {
	t.Helper()
	out := CarryForwardFile([]Comment{c}, prev, next, Now())
	if len(out) != 1 {
		t.Fatalf("expected 1 comment, got %d", len(out))
	}
	return out[0]
}

func TestCarryForwardPreservesCommentIdentity(t *testing.T) {
	orig := Comment{
		ID: "c_old111", StartLine: 2, EndLine: 2, Anchor: "line two",
		Body: "fix", Author: "Human", ReviewRound: 1, Resolved: true, ResolvedRound: 1,
		Replies: []Reply{{ID: "rp_1", Body: "done", Author: "AI"}},
	}
	content := "line one\nline two\nline three\n"

	got := carryOne(t, orig, content, content)

	if got.ID != orig.ID {
		t.Errorf("expected stable ID %q, got %q", orig.ID, got.ID)
	}
	if !got.CarriedForward {
		t.Error("expected CarriedForward")
	}
	if got.StartLine != 2 || got.EndLine != 2 || got.Drifted {
		t.Errorf("unchanged content should keep position: %+v", got)
	}
	if got.ReviewRound != 1 || !got.Resolved || got.ResolvedRound != 1 {
		t.Errorf("round/resolution not preserved: %+v", got)
	}
	if len(got.Replies) != 1 || got.Replies[0].ID != "rp_1" {
		t.Errorf("replies not preserved: %+v", got.Replies)
	}
}

func TestCarryForwardRemapsAnchor(t *testing.T) {
	const fox = "the quick brown fox jumps"
	for _, tt := range []struct {
		name, prev, next, anchor string
		start, end               int
		drifted                  bool
	}{
		{"shifts with inserted lines", "alpha\nbeta\ngamma\n", "intro\nmore\nalpha\nbeta\ngamma\n", "beta\ngamma", 4, 5, false},
		{"finds moved anchor", "target line here\nfiller\nother\n", "other\nfiller\ntarget line here\n", "target line here", 3, 3, false},
		{"tolerates in-place edit", "first\n" + fox + "\nlast\n", "first\n" + fox + " again\nlast\n", fox, 2, 2, false},
		{"prefers exact moved anchor over similar text", "first\n" + fox + "\nsecond\nthird\n", "first\n" + fox + " again\nsecond\nthird\n" + fox + "\n", fox, 5, 5, false},
		{"drifts when anchor is removed", "keep\nremove me entirely\nkeep too\n", "keep\nkeep too\n", "remove me entirely", 2, 2, true},
		{"clamps beyond EOF", "a\nb\nc\nd\n", "a\n", "d", 2, 2, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			start := 1 + strings.Count(tt.prev[:strings.Index(tt.prev, tt.anchor)], "\n")
			c := Comment{ID: "c_1", StartLine: start, EndLine: start + strings.Count(tt.anchor, "\n"), Anchor: tt.anchor}
			got := carryOne(t, c, tt.prev, tt.next)
			if got.StartLine != tt.start || got.EndLine != tt.end || got.Drifted != tt.drifted {
				t.Errorf("range = %d-%d, drifted=%v; want %d-%d, drifted=%v", got.StartLine, got.EndLine, got.Drifted, tt.start, tt.end, tt.drifted)
			}
		})
	}
}

func TestCarryForwardResizesRange(t *testing.T) {
	for _, tt := range []struct {
		name, before, after string
		wantEnd             int
	}{
		{"insert inside", "begin\nmiddle\nend", "begin\nadded one\nadded two\nmiddle\nend", 6},
		{"delete inside", "begin\nremoved one\nremoved two\nmiddle\nend", "begin\nmiddle\nend", 4},
		{"separate insertions", "begin\nmiddle\nend", "begin\nfirst large addition\nmiddle\nsecond large addition\nend", 6},
		{"separate deletions", "begin\nfirst large removal\nmiddle\nsecond large removal\nend", "begin\nmiddle\nend", 4},
	} {
		t.Run(tt.name, func(t *testing.T) {
			prev := "outside before\n" + tt.before + "\noutside after\n"
			next := "outside before\n" + tt.after + "\noutside after\n"
			c := Comment{ID: "c_range", StartLine: 2, EndLine: 2 + strings.Count(tt.before, "\n"), Anchor: tt.before}
			got := carryOne(t, c, prev, next)
			if got.StartLine != 2 || got.EndLine != tt.wantEnd || got.Drifted {
				t.Fatalf("resized range = %d-%d, drifted=%v; want 2-%d", got.StartLine, got.EndLine, got.Drifted, tt.wantEnd)
			}
			// A later round must retain the resized range and original anchor.
			got = carryOne(t, got, next, "new heading\n"+next)
			if got.StartLine != 3 || got.EndLine != tt.wantEnd+1 || got.Drifted || got.Anchor != tt.before {
				t.Errorf("next round lost resized range: %+v", got)
			}
		})
	}
}

func TestCarryForwardRangeBoundaries(t *testing.T) {
	for _, tt := range []struct {
		name, next string
		start, end int
		drifted    bool
	}{
		{"outside insertions", "before\nnew before\nbegin\nmiddle\nend\nnew after\nafter", 3, 5, false},
		{"entire range removed", "before\nafter", 2, 2, true},
		{"entire range replaced", "before\nunrelated\nreplacement\ntext\nafter", 2, 2, true},
		{"exact moved range", "before\nbegin\nchanged\nend\nafter\nbegin\nmiddle\nend", 6, 8, false},
		{"shrinks at EOF", "before\nbegin\nend", 2, 3, false},
		{"in-place edits on every line", "before\nbegin!\nmiddle!\nend!\nafter", 2, 4, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c := Comment{ID: "c_range", StartLine: 2, EndLine: 4, Anchor: "begin\nmiddle\nend"}
			got := carryOne(t, c, "before\nbegin\nmiddle\nend\nafter", tt.next)
			if got.StartLine != tt.start || got.EndLine != tt.end || got.Drifted != tt.drifted {
				t.Errorf("range = %d-%d, drifted=%v; want %d-%d, drifted=%v", got.StartLine, got.EndLine, got.Drifted, tt.start, tt.end, tt.drifted)
			}
		})
	}
}

func TestCarryForwardToleratesMiddleCut(t *testing.T) {
	for _, tt := range []struct{ name, before, after string }{
		{"removed clause", `continue. If this organization has no locations yet,{" "}`, `continue.{" "}`},
		{"inserted clause", `continue.{" "}`, `continue. If this organization has no locations yet,{" "}`},
		{"Japanese clause", "処理を開始し、必要な設定をすべて読み込んでから終了します。", "処理を終了します。"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c := Comment{ID: "c_1", StartLine: 2, EndLine: 2, Anchor: tt.before}
			got := carryOne(t, c, "first\n"+tt.before+"\nlast\n", "first\n"+tt.after+"\nlast\n")
			if got.Drifted || got.StartLine != 2 || got.EndLine != 2 {
				t.Errorf("middle cut should keep position: %+v", got)
			}
		})
	}
}

func TestCarryForwardSkipsFileScopeAndOldSide(t *testing.T) {
	prev := "a\nb\n"
	next := "x\ny\nz\n"
	fileC := Comment{ID: "c_1", Scope: "file", Body: "file-level"}
	oldC := Comment{ID: "c_2", Side: "old", StartLine: 2, EndLine: 2, Anchor: "gone"}

	out := CarryForwardFile([]Comment{fileC, oldC}, prev, next, Now())

	if out[0].StartLine != 0 || out[0].Drifted {
		t.Errorf("file-scope comment should be untouched: %+v", out[0])
	}
	if out[1].StartLine != 2 || out[1].Drifted {
		t.Errorf("old-side comment should keep its base-ref position: %+v", out[1])
	}
}

func TestOneMiddleCut(t *testing.T) {
	for _, tt := range []struct {
		short, long string
		want        bool
	}{
		{"abef", "abcdef", true},
		{"ab", "abcdef", true},
		{"ef", "abcdef", true},
		{"", "abcdef", true},
		{"abde", "abcdexf", false},
		{"efab", "abcdef", false},
		{"abcdef", "abcdef", false},
		{"abcdefg", "abcdef", false},
		{"a—f", "a—cdf", true},
		{"af", "a—f", true},
		{"é", "ê©", false}, // Matching bytes must not split UTF-8 runes.
	} {
		t.Run(tt.short+"/"+tt.long, func(t *testing.T) {
			if got := oneMiddleCut(tt.short, tt.long); got != tt.want {
				t.Errorf("oneMiddleCut(%q, %q) = %v, want %v", tt.short, tt.long, got, tt.want)
			}
		})
	}
}

func TestAnchorSimilar(t *testing.T) {
	tests := []struct {
		a, b string
		want bool
	}{
		{"same text", "same text", true},
		{"prefix of a longer line", "prefix of a longer", true}, // containment, >= 8 chars
		{"}", "} // end", false},                                // short anchors never contain-match
		{"the quick brown fox", "the quick brwon fox", true},    // small typo, ratio >= 0.7
		{"completely different", "nothing alike here!", false},
		{"return nil, err", `return fmt.Errorf("failed to open config file %q: %w", path, err)`, false},
		{"t.Fatal(err)", `t.Fatalf("unexpected error reading %s: %v", path, err)`, false},
	}
	for _, tt := range tests {
		if got := anchorSimilar(tt.a, tt.b); got != tt.want {
			t.Errorf("anchorSimilar(%q, %q) = %v, want %v", tt.a, tt.b, got, tt.want)
		}
	}
}
