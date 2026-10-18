//go:build darwin || linux

package history

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestUsageMatchesDuAllocation(t *testing.T) {
	s := testStore(t)
	task := startTask(t, s, "Small files")
	session := addReview(t, s, task)
	for i := range 32 {
		if err := os.WriteFile(filepath.Join(session.Dir, fmt.Sprintf("small-%d", i)), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Link(filepath.Join(session.Dir, "small-0"), filepath.Join(session.Dir, "same-inode")); err != nil {
		t.Fatal(err)
	}
	var logical int64
	if err := filepath.WalkDir(session.Dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || !entry.Type().IsRegular() {
			return err
		}
		info, err := entry.Info()
		if err == nil {
			logical += info.Size()
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	usage := s.Usage(logical)
	if len(usage.Warnings) != 0 || !usage.Exceeded || usage.Bytes <= logical {
		t.Fatalf("small files were not budgeted by allocation: logical=%d, usage=%+v", logical, usage)
	}
	output, err := exec.Command("du", "-sk", session.Dir).Output()
	if err != nil {
		t.Fatal(err)
	}
	fields := strings.Fields(string(output))
	if len(fields) < 2 {
		t.Fatalf("unexpected du output: %q", output)
	}
	kib, err := strconv.ParseInt(fields[0], 10, 64)
	if err != nil || (usage.Bytes+1023)/1024 != kib {
		t.Fatalf("allocation differs from du: usage=%+v du=%q err=%v", usage, output, err)
	}
}
