package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestTextconvReviewSources(t *testing.T) {
	dir := newRepo(t)
	path := "data 日本語 ' $(touch injected).bin"
	writeFile(t, dir, ".gitattributes", []byte("*.bin diff=records\n"))
	writeFile(t, dir, "convert.sh", []byte("tr '\\000' '\\n' < \"$1\"\n"))
	command := "sh convert.sh"
	runGit(t, dir, "config", "diff.records.textconv", command)
	runGit(t, dir, "config", "diff.records.binary", "true")
	writeFile(t, dir, path, []byte("base\x00same\x00"))
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-qm", "base")
	runGit(t, dir, "tag", "base")
	writeFile(t, dir, path, []byte("committed\x00same\x00"))
	runGit(t, dir, "commit", "-qam", "tip")
	runGit(t, dir, "tag", "tip")
	writeFile(t, dir, path, []byte("staged\x00same\x00"))
	runGit(t, dir, "add", ".")
	writeFile(t, dir, path, []byte("working\x00same\x00"))
	for _, tc := range []struct {
		scope, want, old string
	}{
		{"all", "working", "committed"},
		{"staged", "staged", "committed"},
		{"unstaged", "working", "staged"},
		{"range", "committed", "base"},
	} {
		t.Run(tc.scope, func(t *testing.T) {
			s := ReviewSource{Scope: tc.scope, Base: "HEAD", TextconvDrivers: []string{"*"}}
			if tc.scope == "range" {
				s.Base, s.Head = "base", "tip"
			}
			files, err := s.Files()
			if err != nil || len(files) != 1 {
				t.Fatalf("files=%+v, err=%v", files, err)
			}
			f := files[0]
			if f.IsBinary() || f.Textconv != (Textconv{Driver: "records", Command: command}) {
				t.Fatalf("wrong representation: %+v", f)
			}
			content, err := s.TextContent(path, f.Textconv)
			if err != nil || string(content) != tc.want+"\nsame\n" {
				t.Fatalf("content=%q, err=%v", content, err)
			}
			diff, err := s.Diff(path, "")
			if err != nil || diff == nil || !diff.ChangedLines[1] || diff.DeletedAfter[0][0].Content != tc.old {
				t.Fatalf("diff=%+v, err=%v", diff, err)
			}
			_, raw, err := s.Snapshot()
			if err != nil || !strings.Contains(raw, "GIT binary patch") {
				t.Fatalf("snapshot lost original bytes: %q, %v", raw, err)
			}
		})
	}
	if _, err := os.Stat("injected"); !os.IsNotExist(err) {
		t.Fatal("filename was interpreted as shell code")
	}
	writeFile(t, dir, "new.bin", []byte("new\x00"))
	files, err := (ReviewSource{Scope: "unstaged", TextconvDrivers: []string{"records"}}).Files()
	if err != nil || len(files) != 2 || files[1].Textconv.Command != command {
		t.Fatalf("untracked conversion missing: %+v, %v", files, err)
	}
}

func TestTextconvEncodingAndErrors(t *testing.T) {
	if _, err := exec.LookPath("iconv"); err != nil {
		t.Skip("iconv unavailable")
	}
	dir := newRepo(t)
	writeFile(t, dir, ".gitattributes", []byte("*.txt diff=latin\n"))
	runGit(t, dir, "config", "diff.latin.textconv", "iconv -f ISO-8859-1 -t UTF-8")
	writeFile(t, dir, "cafe.txt", []byte("caf\xe9\n"))
	s := ReviewSource{Scope: "unstaged", TextconvDrivers: []string{"latin"}}
	conversion, err := textconvForPath("cafe.txt", s.TextconvDrivers)
	if err != nil {
		t.Fatal(err)
	}
	data, err := s.TextContent("cafe.txt", conversion)
	if err != nil || string(data) != "café\n" {
		t.Fatalf("content=%q, err=%v", data, err)
	}
	for _, tc := range []struct{ name, command, want string }{
		{"invalid UTF-8", "cat", "did not produce UTF-8 text"},
		{"failure", "false", "textconv latin"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			runGit(t, dir, "config", "diff.latin.textconv", tc.command)
			conversion, err := textconvForPath("cafe.txt", s.TextconvDrivers)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := s.TextContent("cafe.txt", conversion); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestBinaryPlistTextconv(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("requires macOS plutil")
	}
	dir := newRepo(t)
	path := "settings.plist"
	writeFile(t, dir, ".gitattributes", []byte("*.plist diff=plist\n"))
	runGit(t, dir, "config", "diff.plist.textconv", "plutil -convert xml1 -o -")
	for _, value := range []string{"Before", "After"} {
		writeFile(t, dir, path, []byte(`<?xml version="1.0" encoding="UTF-8"?><plist version="1.0"><dict><key>Name</key><string>`+value+`</string></dict></plist>`))
		if out, err := exec.Command("plutil", "-convert", "binary1", filepath.Join(dir, path)).CombinedOutput(); err != nil {
			t.Fatalf("plutil: %s, %v", out, err)
		}
		if value == "Before" {
			runGit(t, dir, "add", ".")
			runGit(t, dir, "commit", "-qm", "base")
		}
	}
	s := ReviewSource{Scope: "all", Base: "HEAD", TextconvDrivers: []string{"plist"}}
	files, err := s.Files()
	if err != nil || len(files) != 1 || files[0].IsBinary() {
		t.Fatalf("plist files=%+v, err=%v", files, err)
	}
	data, err := s.TextContent(path, files[0].Textconv)
	if err != nil || !strings.Contains(string(data), "<string>After</string>") {
		t.Fatalf("plist text=%q, err=%v", data, err)
	}
	diff, err := s.Diff(path, "")
	if err != nil || diff == nil || len(diff.ChangedLines) != 1 {
		t.Fatalf("plist diff=%+v, err=%v", diff, err)
	}
}

func TestTextconvOptIn(t *testing.T) {
	dir := newRepo(t)
	writeFile(t, dir, ".gitattributes", []byte("*.png diff=image\n*.bin diff=records\n"))
	runGit(t, dir, "config", "diff.image.textconv", "touch forbidden; false")
	runGit(t, dir, "config", "diff.records.textconv", "cat")
	writeFile(t, dir, "image.png", []byte("\x00before"))
	writeFile(t, dir, "data.bin", []byte("before\n"))
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-qm", "base")
	writeFile(t, dir, "image.png", []byte("\x00after"))
	writeFile(t, dir, "data.bin", []byte("after\n"))
	for _, tc := range []struct {
		name    string
		enabled []string
	}{
		{"default", nil}, {"selected", []string{"records"}}, {"pattern", []string{"rec*"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := ReviewSource{Scope: "all", Base: "HEAD", TextconvDrivers: tc.enabled}
			files, err := s.Files()
			if err != nil {
				t.Fatal(err)
			}
			for _, f := range files {
				if f.Path == "image.png" && (!f.IsBinary() || f.Textconv.Command != "") {
					t.Fatalf("unselected image lost binary preview: %+v", f)
				}
				if f.Path == "data.bin" && (f.Textconv.Command != "") != (len(tc.enabled) > 0) {
					t.Fatalf("driver opt-in not respected: %+v", f)
				}
				if _, err := s.Diff(f.Path, ""); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := os.Stat("forbidden"); !os.IsNotExist(err) {
				t.Fatal("unselected textconv was executed")
			}
		})
	}
	// A rename cannot implicitly run an unselected old-side driver.
	runGit(t, dir, "mv", "image.png", "image.bin")
	s := ReviewSource{Scope: "all", Base: "HEAD", TextconvDrivers: []string{"records"}}
	if _, err := s.Diff("image.bin", "image.png"); err == nil {
		t.Fatal("accepted rename with different converter selection")
	}
	if _, err := os.Stat("forbidden"); !os.IsNotExist(err) {
		t.Fatal("rename executed an unselected converter")
	}
}

func TestTextconvDriverPatterns(t *testing.T) {
	for _, tc := range []struct {
		pattern, driver string
		want            bool
	}{
		{"*", "plist", true},
		{"*", "custom/plist", true},
		{"plist", "plist", true},
		{"plist", "plist-extra", false},
		{"plist*", "plist-extra", true},
		{"cp93?", "cp932", true},
		{"cp93[12]", "cp932", true},
		{"cp93[12]", "cp933", false},
	} {
		t.Run(tc.pattern+"/"+tc.driver, func(t *testing.T) {
			got, err := textconvEnabled(tc.driver, []string{tc.pattern})
			if err != nil || got != tc.want {
				t.Fatalf("matched=%t, err=%v", got, err)
			}
		})
	}
	if _, err := textconvEnabled("plist", []string{"["}); err == nil {
		t.Fatal("accepted invalid pattern")
	}
}
