package tui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	tea "charm.land/bubbletea/v2"
	gitpkg "github.com/crittylabs/critty/internal/git"
	"github.com/crittylabs/critty/internal/terminalbinding"
)

type externalOpenMsg struct{ status string }

// externalOpener names the command that opens files and URLs with the
// desktop's default application.
func externalOpener() string {
	switch runtime.GOOS {
	case "darwin":
		return "open"
	case "linux", "freebsd", "openbsd", "netbsd":
		return "xdg-open"
	}
	return ""
}

// remoteSession reports whether this review is known to run over SSH, where
// a desktop opener would act on the wrong machine.
var remoteSession = func() bool {
	target, err := terminalbinding.FromEnvironment()
	if err != nil {
		target = terminalbinding.Target{}
	}
	return terminalbinding.Remote(target, terminalbinding.CaptureOrigin(os.Getpid()))
}

var startOpener = func(name, arg string) error {
	cmd := exec.Command(name, arg)
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}

// openImageExternally opens an image file or web URL with the desktop's
// default application, except in remote sessions.
func (m *AppModel) openImageExternally(target imageTarget) tea.Cmd {
	if m.readOnly {
		m.locationError = "Dry run: external applications are disabled"
		return nil
	}
	source := m.imageSource()
	return func() tea.Msg {
		return externalOpenMsg{openImageTarget(source, target)}
	}
}

func openImageTarget(source gitpkg.ReviewSource, target imageTarget) string {
	opener := externalOpener()
	if opener == "" {
		return "Opening images is not supported on this platform"
	}
	if remoteSession() {
		return "Not opened: this session runs over SSH"
	}
	arg, err := externalOpenArgument(source, target)
	if err != nil {
		return fmt.Sprintf("Cannot open %s: %v", target.label(), err)
	}
	if err := startOpener(opener, arg); err != nil {
		return fmt.Sprintf("Cannot open %s: %v", target.label(), err)
	}
	return "Opened " + target.label()
}

// externalOpenArgument validates the target and returns an argument the
// opener cannot mistake for an option: an http(s) URL or an absolute path to
// an image file, extracted from the review source for the before side.
func externalOpenArgument(source gitpkg.ReviewSource, target imageTarget) (string, error) {
	if target.url != "" {
		u, ok := webImageURL(target.url)
		if !ok {
			return "", fmt.Errorf("not a web URL")
		}
		return u, nil
	}
	if !imagePath(target.path) {
		return "", fmt.Errorf("not an image file")
	}
	if target.before {
		data, err := source.PreviewContent(target.path, true, maxPreviewBytes)
		if err != nil {
			return "", err
		}
		f, err := os.CreateTemp("", "critty-*"+filepath.Ext(target.path))
		if err != nil {
			return "", err
		}
		if _, err := f.Write(data); err != nil {
			f.Close()
			return "", err
		}
		if err := f.Close(); err != nil {
			return "", err
		}
		return f.Name(), nil
	}
	abs, err := filepath.Abs(target.path)
	if err != nil {
		return "", err
	}
	if !regularFile(abs) {
		return "", fmt.Errorf("no such file")
	}
	return abs, nil
}
