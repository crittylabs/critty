package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestWriteVersionPlainAndBranded(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	t.Setenv("CLICOLOR_FORCE", "")
	t.Setenv("TTY_FORCE", "")
	var plain bytes.Buffer
	if err := writeVersion(&plain, "critty", "v1.2.3"); err != nil || plain.String() != "critty version v1.2.3\n" {
		t.Fatalf("plain version = %q, %v", plain.String(), err)
	}

	t.Setenv("TTY_FORCE", "1")
	t.Setenv("CLICOLOR_FORCE", "1")
	var branded bytes.Buffer
	if err := writeVersion(&branded, "critty", "v1.2.3"); err != nil {
		t.Fatal(err)
	}
	if got := ansi.Strip(branded.String()); got != ">_ critty v1.2.3\n" {
		t.Fatalf("branded version text = %q", got)
	}
	if !strings.Contains(branded.String(), "\x1b[") {
		t.Fatalf("branded version has no styling: %q", branded.String())
	}

	t.Setenv("NO_COLOR", "1")
	var disabled bytes.Buffer
	if err := writeVersion(&disabled, "critty", "v1.2.3"); err != nil || disabled.String() != plain.String() {
		t.Fatalf("NO_COLOR version = %q, %v", disabled.String(), err)
	}
}
