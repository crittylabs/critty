package cli

import (
	"context"
	"debug/buildinfo"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/crittylabs/critty/internal/terminalbinding"
	"github.com/tklauser/ps"
)

func (r *doctorReport) checkTerminalBinding(request string, notifyOnly bool) {
	r.checkCodexWrapper()
	origin := terminalbinding.CaptureOrigin(os.Getpid())
	resolved := false
	if request == "" {
		r.add("terminal/marker", "info", "terminal binding has not been tested", "Prepare a request with `critty terminal prepare`, display its marker in agent output, then run `critty doctor --terminal-request ID`.")
	} else {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		var target terminalbinding.Target
		var err error
		if notifyOnly {
			target, err = terminalbinding.DefaultStore().ResolveNotification(ctx, request)
		} else {
			target, err = terminalbinding.DefaultStore().Resolve(ctx, request, terminalbinding.Snapshot)
		}
		if err != nil {
			advice := "Start Codex through `critty codex` or tfil with --critty-notify, then prepare and display a new marker. Use the same XDG_STATE_HOME in the agent and terminal filter."
			if notifyOnly {
				advice = "Enable tfil --critty-notify with Critty on its PATH and the same XDG_STATE_HOME as the agent, then prepare and display a new marker."
			}
			r.add("terminal/marker", "warn", err.Error(), advice)
		} else {
			origin = target.Origin
			resolved = true
			message := "visible marker resolved to " + target.Kind
			if notifyOnly || target.Kind == "terminal" {
				message = "visible marker resolved through a terminal filter notification"
			}
			r.add("terminal/marker", "ok", message, "")
		}
	}
	if bin, err := lookPath("tfil"); err == nil {
		r.add("terminal/tfil", "ok", "installed at "+bin, "Installation alone does not establish an active filter.")
	} else {
		r.add("terminal/tfil", "info", "tfil is not available on PATH", "tfil is optional; an active filter may also have been launched by an absolute path.")
	}
	pid, err := tfilAncestor(origin.PID, func(pid int) (int, string, error) {
		p, err := ps.FindProcess(pid)
		if err != nil {
			return 0, "", err
		}
		return p.PPID(), p.Command(), nil
	})
	if pid != 0 {
		r.add("terminal/tfil-session", "ok", fmt.Sprintf("tfil detected in terminal process ancestry (PID %d)", pid), "A marker test verifies terminal binding; process detection alone does not verify --critty-notify.")
	} else if err != nil {
		r.add("terminal/tfil-session", "info", "could not inspect the full terminal process ancestry", "Use a marker test to verify terminal binding, including daemon-backed sessions.")
	} else if resolved && origin.PID == 0 {
		r.add("terminal/tfil-session", "info", "resolved terminal did not supply client process metadata; tfil activity is unverified", "Update the Critty executable used by the wrapper or terminal filter, restart the client, and test a new marker.")
	} else {
		r.add("terminal/tfil-session", "info", "tfil was not detected in this process ancestry", "A daemon can hide the client ancestry. Use a marker test to check the originating terminal.")
	}
}

func (r *doctorReport) checkCodexWrapper() {
	bin, err := lookPath("codex")
	if err != nil {
		r.add("terminal/codex-wrapper", "info", "codex is not available on PATH", "The Codex wrapper is optional for other agents.")
		return
	}
	if isTfilWrapper(bin) {
		r.add("terminal/codex-wrapper", "ok", "PATH selects a tfil-generated Codex wrapper at "+bin, "Wrapper detection does not verify --critty-notify. Test a displayed marker with --terminal-request ID --terminal-notify-only.")
		return
	}
	bi, err := buildinfo.ReadFile(bin)
	if err == nil && bi.Path == "github.com/crittylabs/critty/cmd/codex" {
		r.add("terminal/codex-wrapper", "ok", "PATH selects the Critty Codex wrapper at "+bin, "A marker test verifies the current session; PATH selection applies to future launches.")
		return
	}
	r.add("terminal/codex-wrapper", "info", "PATH selects "+bin+"; not identified as a Critty or tfil wrapper", "A shim, custom wrapper, or tfil may still provide terminal binding. Use a marker test, or launch with `critty codex`.")
}

func tfilAncestor(pid int, inspect func(int) (int, string, error)) (int, error) {
	seen := map[int]bool{}
	for depth := 0; pid > 1 && !seen[pid] && depth < 128; depth++ {
		seen[pid] = true
		parent, command, err := inspect(pid)
		if err != nil {
			return 0, err
		}
		if filepath.Base(command) == "tfil" {
			return pid, nil
		}
		pid = parent
	}
	return 0, nil
}

// tfil uses this exact prefix to recognize its own generated wrappers.
func isTfilWrapper(path string) bool {
	const marker = "#!/bin/sh\n# tfil-wrapper\n"
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	var header [len(marker)]byte
	_, err = io.ReadFull(f, header[:])
	return err == nil && string(header[:]) == marker
}
