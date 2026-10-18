//go:build !darwin && !linux

package history

import (
	"fmt"
	"io/fs"
)

func allocatedDiskSpace(info fs.FileInfo) (diskAllocation, error) {
	return diskAllocation{}, fmt.Errorf("allocated disk space unavailable on this platform")
}
