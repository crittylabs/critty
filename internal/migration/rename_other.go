//go:build !darwin && !linux && !windows

package migration

import "fmt"

func renameAbsent(from, to string) error {
	return fmt.Errorf("automatic name migration is unavailable on this platform; rename %s to %s", from, to)
}
