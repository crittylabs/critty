package cli

import (
	"testing"

	"github.com/spf13/cobra"
)

func TestTransientCommandSelection(t *testing.T) {
	oldSession, oldDemo := rootSession, reviewDemo
	oldDiff, oldCode := reviewDiff, reviewCode
	t.Cleanup(func() {
		rootSession, reviewDemo = oldSession, oldDemo
		reviewDiff, reviewCode = oldDiff, oldCode
	})
	for _, tc := range []struct {
		name    string
		cmd     *cobra.Command
		args    []string
		session string
		demo    bool
		want    bool
	}{
		{"bare Git review", rootCmd, nil, "", false, false},
		{"implicit document view", rootCmd, []string{"notes.md:3"}, "", false, true},
		{"explicit document review", reviewCmd, []string{"notes.md"}, "", false, false},
		{"legacy document resume", rootCmd, []string{"notes.md"}, "session", false, false},
		{"explicit view", viewCmd, nil, "", false, true},
		{"demo review", reviewCmd, []string{"notes.md"}, "", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rootSession, reviewDemo = tc.session, tc.demo
			if got := transientReview(tc.cmd, tc.args); got != tc.want {
				t.Fatalf("transientReview = %t, want %t", got, tc.want)
			}
		})
	}
	rootSession, reviewDemo = "", false
	for _, code := range []bool{false, true} {
		reviewDiff, reviewCode = "-", code
		if defaultFileView([]string{"input.diff"}) {
			t.Fatal("treated a supplied diff as a document view")
		}
	}
	reviewDiff, reviewCode = "", true
	if defaultFileView([]string{"notes.md"}) {
		t.Fatal("treated explicit code mode as a document view")
	}
}
