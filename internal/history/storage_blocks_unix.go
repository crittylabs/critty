//go:build darwin || linux

package history

import (
	"fmt"
	"io/fs"
	"math"
	"syscall"
)

func allocatedDiskSpace(info fs.FileInfo) (diskAllocation, error) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat == nil || stat.Blocks < 0 || stat.Blocks > math.MaxInt64/512 {
		return diskAllocation{}, fmt.Errorf("allocated disk space unavailable")
	}
	// st_blocks uses 512-byte units, independently of the preferred I/O size.
	return diskAllocation{stat.Blocks * 512, uint64(stat.Dev), stat.Ino}, nil
}
