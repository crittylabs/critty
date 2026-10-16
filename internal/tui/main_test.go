package tui

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	// Tests size previews in cells only; the developer's terminal must not
	// cap them at the test images' tiny pixel sizes.
	cellPixelSize = func() cellSize { return cellSize{} }
	os.Exit(m.Run())
}
