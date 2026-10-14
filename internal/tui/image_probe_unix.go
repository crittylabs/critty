//go:build darwin || linux

package tui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
	"golang.org/x/term"
)

// CheckTerminalImages probes only this process's foreground controlling TTY.
// Terminal traffic goes to /dev/tty, keeping redirected reports free of escapes.
func CheckTerminalImages() (result ImageCapability, err error) {
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return ImageCapability{Reason: "no current TTY available"}, nil
	}
	defer func() { err = errors.Join(err, tty.Close()) }()
	fd := int(tty.Fd())
	foreground, e := unix.IoctlGetInt(fd, unix.TIOCGPGRP)
	if e != nil || foreground != unix.Getpgrp() {
		return ImageCapability{Reason: "current TTY is not owned by this foreground process"}, nil
	}
	_, height, e := term.GetSize(fd)
	if e != nil || height < imageProbeRow {
		return ImageCapability{Reason: "current TTY size is unavailable or too small"}, nil
	}
	if ready, e := ttyReadable(fd, 0); e != nil || ready {
		return ImageCapability{Reason: "current TTY input is busy or unavailable"}, nil
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	state, e := term.MakeRaw(fd)
	if e != nil {
		return ImageCapability{Reason: "current TTY could not be opened for probing"}, nil
	}
	defer func() {
		if restore := term.Restore(fd, state); restore != nil {
			err = errors.Join(err, fmt.Errorf("restoring terminal settings: %w", restore))
		}
	}()
	result.Checked = true
	tmux := os.Getenv("TMUX") != ""
	for range maxImageProbeAttempts {
		p := newTerminalImageProbe(tmux)
		sequence := p.display.probeSequence()
		if sequence == "" {
			return ImageCapability{Reason: "terminal mode cannot safely probe images"}, nil
		}
		if _, e := tty.WriteString(sequence); e != nil {
			return result, fmt.Errorf("writing terminal image probe: %w", e)
		}
		if e := readImageProbe(ctx, tty, &p); e != nil {
			return result, e
		}
		if p.display.enabled {
			result.Supported = true
			result.Reason = "Kitty virtual placement accepted without moving the cursor"
			return result, nil
		}
		if !p.display.checking {
			result.Reason = "terminal rejected virtual placement or moved the cursor"
			return result, nil
		}
	}
	result.Reason = "no complete image capability response; support remains unconfirmed"
	return result, nil
}

func readImageProbe(ctx context.Context, tty *os.File, p *terminalImageProbe) error {
	deadline := time.Now().Add(imageProbeTimeout)
	fd := int(tty.Fd())
	buffer := make([]byte, 256)
	// Read the cursor reply even after rejection, so it cannot leak to the shell.
	for !p.complete() && time.Now().Before(deadline) {
		if ctx.Err() != nil {
			return fmt.Errorf("image check interrupted")
		}
		wait := min(100*time.Millisecond, max(0, time.Until(deadline)))
		ready, err := ttyReadable(fd, wait)
		if errors.Is(err, unix.EINTR) {
			continue
		}
		if err != nil {
			return fmt.Errorf("reading terminal image reply: %w", err)
		}
		if !ready {
			continue
		}
		n, err := tty.Read(buffer)
		if err != nil || n == 0 {
			return fmt.Errorf("reading terminal image reply: %w", errors.Join(err, os.ErrClosed))
		}
		if err := p.feed(buffer[:n]); err != nil {
			return err
		}
	}
	return nil
}

// poll reports POLLNVAL for /dev/tty on macOS; select supports that device.
func ttyReadable(fd int, timeout time.Duration) (bool, error) {
	var set unix.FdSet
	if fd < 0 || fd >= len(set.Bits)*unix.NFDBITS {
		return false, fmt.Errorf("terminal descriptor exceeds select limit")
	}
	set.Set(fd)
	value := unix.NsecToTimeval(timeout.Nanoseconds())
	n, err := unix.Select(fd+1, &set, nil, nil, &value)
	return n > 0 && set.IsSet(fd), err
}
