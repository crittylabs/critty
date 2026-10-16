//go:build !darwin && !linux

package tui

var cellPixelSize = func() cellSize { return cellSize{} }
