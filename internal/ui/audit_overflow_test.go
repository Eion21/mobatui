package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestAuditOverflow40x16(t *testing.T) {
	a := hybridApp(t, sampleHybrid(), 10, 40, 16)
	t.Logf("detail.height=%d vp.Height=%d", a.detail.height, a.detail.vp.Height)
	t.Logf("headerBlock=%d itemsFooter=%d", lipgloss.Height(a.detail.headerBlock()), lipgloss.Height(a.detail.itemsFooter()))
	t.Logf("app header=%d footer=%d", lipgloss.Height(a.header()), lipgloss.Height(a.footer()))
	t.Logf("detail.View()=%d", lipgloss.Height(a.detail.View()))
	t.Logf("View() total=%d (límite %d)", lipgloss.Height(a.View()), a.height)
	for i, line := range strings.Split(a.View(), "\n") {
		if i > 22 {
			break
		}
		t.Logf("  %2d |%s", i, collapsePlain(ansiRE.ReplaceAllString(line, "")))
	}
}
