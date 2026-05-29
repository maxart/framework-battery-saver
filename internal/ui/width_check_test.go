package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestRenderAtWidths(t *testing.T) {
	for _, w := range []int{40, 60, 100, 140} {
		d := Dashboard{snap: sampleSnapshot(true), width: w}
		view := d.View()
		maxW := 0
		for _, ln := range strings.Split(view, "\n") {
			if x := lipgloss.Width(ln); x > maxW {
				maxW = x
			}
		}
		if maxW > w {
			t.Errorf("width=%d produced a line of width %d (overflow)", w, maxW)
		}
		t.Logf("width=%d -> max line width %d", w, maxW)
	}
}
