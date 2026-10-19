//go:build darwin || linux

package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/crittylabs/critty/internal/terminalbinding"
	"github.com/tklauser/ps"
	"golang.org/x/sys/unix"
)

func probeCurrentTerminalMarker(origin terminalbinding.Origin) (result markerProbeResult, err error) {
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return markerProbeResult{Reason: "no current TTY available"}, nil
	}
	defer func() { err = errors.Join(err, tty.Close()) }()
	foreground, err := unix.IoctlGetInt(int(tty.Fd()), unix.TIOCGPGRP)
	if err != nil || foreground != unix.Getpgrp() {
		return markerProbeResult{Reason: "current TTY is not owned by this foreground process"}, nil
	}
	target, err := terminalbinding.FromEnvironment()
	if err != nil {
		return result, err
	}
	target.Origin = origin
	if target.Kind == "" {
		pid, err := tfilAncestor(origin.PID, func(pid int) (int, string, error) {
			p, err := ps.FindProcess(pid)
			if err != nil {
				return 0, "", err
			}
			return p.PPID(), p.Command(), nil
		})
		if err != nil || pid == 0 {
			return markerProbeResult{Reason: "no readable multiplexer pane or active tfil detected"}, nil
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	resolved, err := terminalbinding.DefaultStore().Probe(ctx, target, func(marker string) error {
		_, err := fmt.Fprintln(tty, marker)
		return err
	}, terminalbinding.Snapshot)
	return markerProbeResult{Checked: err == nil, Target: resolved}, err
}
