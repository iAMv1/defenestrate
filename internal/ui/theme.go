// Package ui — theme.go holds the shared design tokens so every view renders
// from one system: one accent, a status triad, dim neutrals, stable bars.
// Terminal-native rules: dense rows, aligned columns, no emoji, no banners
// mid-flow, motion only where it explains state.
package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// ANSI palette (256-color): restrained, readable on dark and light.
var (
	AccentColor = lipgloss.Color("39")  // cyan — headers, focus, identity
	OkColor     = lipgloss.Color("42")  // green — success, healthy
	WarnColor   = lipgloss.Color("214") // amber — held-for-review, caution
	BadColor    = lipgloss.Color("204") // red — errors, unhealthy
	DimColor    = lipgloss.Color("241") // neutrals
	SelBg       = lipgloss.Color("236") // selected row background
)

var (
	StyleTitle  = lipgloss.NewStyle().Bold(true).Foreground(AccentColor)
	StyleSection = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("250"))
	StyleGood   = lipgloss.NewStyle().Foreground(OkColor)
	StyleWarn   = lipgloss.NewStyle().Foreground(WarnColor)
	StyleBad    = lipgloss.NewStyle().Foreground(BadColor)
	StyleDim    = lipgloss.NewStyle().Foreground(DimColor)
	StyleSel    = lipgloss.NewStyle().Bold(true).Foreground(AccentColor)
	StyleHelp   = lipgloss.NewStyle().Foreground(DimColor)
)

// Section renders the fixed rhythm mole uses: title line, then content,
// then exactly one blank line. Never two blanks, never none.
func Header(title, right string, width int) string {
	t := StyleTitle.Render(title)
	if right == "" {
		return t + "\n"
	}
	gap := width - visibleLen(title) - visibleLen(right)
	if gap < 1 {
		gap = 1
	}
	pad := ""
	for i := 0; i < gap; i++ {
		pad += " "
	}
	return t + pad + StyleDim.Render(right) + "\n"
}

func visibleLen(s string) int {
	// lipgloss.Width would be ideal; avoid extra import cycles by stripping
	// ANSI escapes manually — our own styles are the only source.
	n := 0
	inEsc := false
	for _, r := range s {
		if inEsc {
			if r == 'm' {
				inEsc = false
			}
			continue
		}
		if r == '\033' {
			inEsc = true
			continue
		}
		n++
	}
	return n
}

// Sparkline renders recent history as unicode blocks, newest last.
// Zero values render as thin baseline so gaps stay visible.
func Sparkline(values []float64, width int) string {
	if len(values) == 0 {
		return strings.Repeat("▁", width)
	}
	// Take the last `width` samples.
	v := values
	if len(v) > width {
		v = v[len(v)-width:]
	}
	max := 0.000001
	for _, x := range v {
		if x > max {
			max = x
		}
	}
	blocks := []rune{'▁', '▂', '▃', '▄', '▅', '▆', '▇', '█'}
	out := make([]rune, 0, len(v))
	for _, x := range v {
		idx := int(x / max * float64(len(blocks)-1))
		if idx < 0 {
			idx = 0
		}
		if idx >= len(blocks) {
			idx = len(blocks) - 1
		}
		out = append(out, blocks[idx])
	}
	for len(out) < width {
		out = append([]rune{'▁'}, out...)
	}
	return string(out)
}

// BarColored colors the bar by load thresholds (share of capacity).
func BarColored(frac float64, width int) string {
	bar := Bar(frac, width)
	switch {
	case frac >= 0.9:
		return StyleBad.Render(bar)
	case frac >= 0.7:
		return StyleWarn.Render(bar)
	default:
		return bar
	}
}

// HealthText colors a 0-100 health score.
func HealthText(h int) string {
	s := fmt.Sprint(h)
	switch {
	case h >= 80:
		return StyleGood.Render(s)
	case h >= 50:
		return StyleWarn.Render(s)
	default:
		return StyleBad.Render(s)
	}
}
