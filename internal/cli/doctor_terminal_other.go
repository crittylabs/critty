//go:build !darwin && !linux

package cli

import "github.com/crittylabs/critty/internal/terminalbinding"

func probeCurrentTerminalMarker(terminalbinding.Origin) (markerProbeResult, error) {
	return markerProbeResult{Reason: "local marker probing is unavailable on this platform"}, nil
}
