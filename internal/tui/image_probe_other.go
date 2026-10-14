//go:build !darwin && !linux

package tui

func CheckTerminalImages() (ImageCapability, error) {
	return ImageCapability{Reason: "current TTY image probing is unavailable on this platform"}, nil
}
