package cli

import (
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
)

// pagerCommand chooses the pager the way Git does: PAGER, defaulting to
// less, with an empty value or cat meaning no pager.  Values with shell
// syntax run through the shell.  For less, -R keeps the colored output
// readable and LESS defaults to FRX when the user has not set it.
func pagerCommand(lookup func(string) (string, bool)) (argv, env []string) {
	pager, ok := lookup("PAGER")
	if !ok {
		pager = "less"
	}
	argv = strings.Fields(pager)
	if len(argv) == 0 || pager == "cat" {
		return nil, nil
	}
	less := filepath.Base(argv[0]) == "less"
	if less {
		if _, ok := lookup("LESS"); !ok {
			env = append(env, "LESS=FRX")
		}
	}
	if strings.ContainsAny(pager, "|&;<>()$`\\\"'*?[#~=%") {
		return []string{"sh", "-c", pager}, env
	}
	if less {
		argv = append(argv, "-R")
	}
	return argv, env
}

// showInPager pipes text through the pager on the terminal.  It reports
// false when no pager is configured or the pager could not be started, so
// the caller can print the text directly.
func showInPager(text string) bool {
	argv, env := pagerCommand(os.LookupEnv)
	if argv == nil {
		return false
	}
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Env = append(os.Environ(), env...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return false
	}
	if err := cmd.Start(); err != nil {
		return false
	}
	// The pager owns the terminal until it exits; an interrupt is its to
	// handle, not a reason to exit while it is still drawing.
	signal.Ignore(os.Interrupt)
	defer signal.Reset(os.Interrupt)
	_, _ = io.WriteString(stdin, text)
	_ = stdin.Close()
	_ = cmd.Wait()
	return true
}
