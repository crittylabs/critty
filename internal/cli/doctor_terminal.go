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

type markerProbeResult struct {
	Checked bool
	Reason  string
	Target  terminalbinding.Target
}

var probeDoctorMarker = probeCurrentTerminalMarker

func (r *doctorReport) checkTerminalBinding(request string, notifyOnly bool) {
	r.checkCodexWrapper()
	origin := terminalbinding.CaptureOrigin(os.Getpid())
	if request == "" {
		result, err := probeDoctorMarker(origin)
		advice := "To test the agent output path, prepare a request with `critty terminal prepare`, display its marker in agent output, then run `critty doctor --terminal-request ID`."
		switch {
		case err != nil:
			r.add("terminal/marker", "warn", "local marker check failed: "+err.Error(), "Check the current multiplexer connection or tfil --critty-notify, then retry doctor.")
		case !result.Checked:
			r.add("terminal/marker", "info", "skipped: "+result.Reason, advice)
		default:
			r.add("terminal/marker", "ok", "local marker round trip succeeded via "+result.Target.Kind+"; agent output path untested", advice)
			origin = result.Target.Origin
		}
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
			message := "visible marker resolved to " + target.Kind
			if notifyOnly || target.Kind == "terminal" {
				message = "visible marker resolved through a terminal filter notification"
			}
			r.add("terminal/marker", "ok", message, "")
		}
	}
	message := "optional filter is not available on PATH; no action required"
	if bin, err := lookPath("tfil"); err == nil {
		message = "optional filter installed at " + bin
	}
	pid, _ := tfilAncestor(origin.PID, func(pid int) (int, string, error) {
		p, err := ps.FindProcess(pid)
		if err != nil {
			return 0, "", err
		}
		return p.PPID(), p.Command(), nil
	})
	if pid != 0 {
		message += fmt.Sprintf("; detected in process ancestry (PID %d), ancestry alone does not verify agent output notification", pid)
	}
	r.add("terminal/tfil", "info", message, "")
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
