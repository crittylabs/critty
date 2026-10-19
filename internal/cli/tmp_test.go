package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/crittylabs/critty/internal/form"
	"github.com/crittylabs/critty/internal/review"
	"github.com/crittylabs/critty/internal/workspace"
)

func TestTmpCreate(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	root, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	var previous string
	for range 2 {
		cmd := newTmpCommand()
		var output bytes.Buffer
		cmd.SetOut(&output)
		cmd.SetArgs([]string{"create"})
		if err := cmd.Execute(); err != nil {
			t.Fatal(err)
		}
		var result struct {
			Directory string `json:"directory"`
		}
		if err := json.Unmarshal(output.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		if !filepath.IsAbs(result.Directory) || filepath.Dir(result.Directory) != filepath.Join(root, ".critty", "tmp") || result.Directory == previous {
			t.Fatalf("invalid scratch directory: %q", result.Directory)
		}
		entries, err := os.ReadDir(result.Directory)
		if err != nil || len(entries) != 0 {
			t.Fatalf("scratch directory is not empty: %v, %v", entries, err)
		}
		info, err := os.Stat(result.Directory)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm()&0077 != 0 {
			t.Fatalf("scratch directory is not private: %v", info.Mode())
		}
		previous = result.Directory
	}
	ignore, err := os.ReadFile(filepath.Join(root, ".critty", "tmp", ".gitignore"))
	if err != nil || string(ignore) != "*\n" {
		t.Fatalf("scratch ignore: %q, %v", ignore, err)
	}
	if _, err := os.Stat(filepath.Join(root, ".git")); !os.IsNotExist(err) {
		t.Fatalf("unexpected repository: %v", err)
	}
}

func TestWorkspaceConsentBindsSnapshot(t *testing.T) {
	for _, version := range []int{0, 1} {
		t.Run(fmt.Sprintf("request-version-%d", version), func(t *testing.T) {
			t.Setenv("XDG_STATE_HOME", t.TempDir())
			s := workspace.Snapshot{RequestVersion: version, Manifest: workspace.Manifest{Version: 1, Root: t.TempDir(), Files: map[string]workspace.File{"a": {}}}, After: map[string]workspace.File{"a": {Exists: true, Digest: "sha256:abc", Mode: 0600}}, Diff: "fixed diff"}
			session, err := review.NewSession("", review.SessionEntry{CWD: t.TempDir(), Mode: "diff"})
			if err != nil {
				t.Fatal(err)
			}
			if err := session.Save(); err != nil {
				t.Fatal(err)
			}
			state, err := session.BeginForms(review.ReviewInput{Scope: "diff", Diff: s.Diff}, []review.AttachedDocument{{ID: "apply-doc", Source: "/APPLY.md", Content: s.Request()}})
			if err != nil {
				t.Fatal(err)
			}
			state.Documents[0].Answers["apply"] = form.Answer{Value: true, Human: true, Revision: state.Revision, Round: 1}
			result := state.Result(true, 1)
			result.Session = session.Key
			result.Verdict = review.VerdictApprove
			if err := session.SaveFormResult(result); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(t.TempDir(), "result.json")
			encode := func(r review.FormResult) {
				t.Helper()
				data, err := json.Marshal(r)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, data, 0600); err != nil {
					t.Fatal(err)
				}
			}
			encode(result)
			if err := verifyWorkspaceConsent(path, s); err != nil {
				t.Fatal(err)
			}
			// An unchanged consent survives a comment round and later approval.
			if err := session.SaveForms(state); err != nil {
				t.Fatal(err)
			}
			state, err = session.BeginForms(state.Input, []review.AttachedDocument{state.Documents[0].AttachedDocument})
			if err != nil {
				t.Fatal(err)
			}
			result = state.Result(true, 2)
			result.Session, result.Verdict = session.Key, review.VerdictApprove
			if err := session.SaveFormResult(result); err != nil {
				t.Fatal(err)
			}
			encode(result)
			if err := verifyWorkspaceConsent(path, s); err != nil {
				t.Fatalf("carried consent: %v", err)
			}
			changed := s
			changed.Diff = "other diff"
			if err := verifyWorkspaceConsent(path, changed); err == nil {
				t.Fatal("accepted different diff")
			}
			changed = s
			changed.Root = t.TempDir()
			if err := verifyWorkspaceConsent(path, changed); err == nil {
				t.Fatal("accepted different destinations")
			}
			changed = s
			changed.After = map[string]workspace.File{"a": {Exists: true, Digest: "sha256:changed", Mode: 0600}}
			if err := verifyWorkspaceConsent(path, changed); err == nil {
				t.Fatal("accepted different file states with the same diff")
			}
			changed = s
			changed.RequestVersion = 1 - version
			if err := verifyWorkspaceConsent(path, changed); err == nil {
				t.Fatal("accepted a rewritten application request")
			}
			result.Documents[0].Answers["apply"] = form.Answer{Value: true, Human: false, Revision: state.Revision, Round: 1}
			encode(result)
			if err := verifyWorkspaceConsent(path, s); err == nil {
				t.Fatal("accepted altered result")
			}
			if err := session.SaveFormResult(result); err != nil {
				t.Fatal(err)
			}
			if err := verifyWorkspaceConsent(path, s); err == nil {
				t.Fatal("accepted nonhuman consent")
			}
		})
	}
}
