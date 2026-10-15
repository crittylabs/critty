package cli

import "github.com/tklauser/ps"

func inspectParentProcess(pid int) (int, error) {
	proc, err := ps.FindProcess(pid)
	if err != nil {
		return 0, err
	}
	return proc.PPID(), nil
}

type reviewMultiplexerContext interface {
	kind() string
}

type reviewLaunch interface {
	close() error
	restoreFocus()
}

type multiplexerDetector interface {
	environmentPresent() bool
	environmentContext() reviewMultiplexerContext
	processContexts() map[int]reviewMultiplexerContext
}

var supportedMultiplexerDetectors = []multiplexerDetector{
	herdrDetector{},
	tmuxDetector{},
}

func findMultiplexerContext() reviewMultiplexerContext {
	detectors := make([]multiplexerDetector, 0, len(supportedMultiplexerDetectors))
	for _, detector := range supportedMultiplexerDetectors {
		if detector.environmentPresent() {
			detectors = append(detectors, detector)
		}
	}
	fromEnvironment := len(detectors) > 0
	if !fromEnvironment {
		detectors = supportedMultiplexerDetectors
	}

	if ctx := walkAncestorContexts(detectors); ctx != nil {
		return ctx
	}
	// Preserve direct environment-based operation when exactly one
	// multiplexer is present but its host cannot expose process metadata.
	if fromEnvironment && len(detectors) == 1 {
		return detectors[0].environmentContext()
	}
	return nil
}

func walkAncestorContexts(detectors []multiplexerDetector) reviewMultiplexerContext {
	contexts := make([]map[int]reviewMultiplexerContext, len(detectors))
	for i, detector := range detectors {
		contexts[i] = detector.processContexts()
	}

	seen := make(map[int]bool)
	for pid := parentProcessID(); pid > 1 && !seen[pid]; {
		seen[pid] = true
		for _, candidates := range contexts {
			if ctx, ok := candidates[pid]; ok {
				return ctx
			}
		}
		ppid, err := inspectProcess(pid)
		if err != nil {
			break
		}
		pid = ppid
	}

	return nil
}
