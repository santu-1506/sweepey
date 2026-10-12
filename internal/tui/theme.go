// Package tui is sweepr's full-screen terminal interface: animated menu,
// pickers, live dashboard and disk explorer, built on Bubble Tea.
package tui

import (
	"math"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/lucasb-eyer/go-colorful"

	"github.com/santu-1506/sweepey/internal/ui"
)

// Palette: a neon sweep from pink through violet and blue to cyan.
const (
	Pink   = "#FF4FD8"
	Violet = "#9B5CFF"
	Blue   = "#4D7CFF"
	Cyan   = "#2FE0FF"
	Mint   = "#2BE4A7"
	Green  = "#43E97B"
	Yellow = "#FFD45E"
	Orange = "#FF9F43"
	Red    = "#FF4D6D"
)

var (
	Neon   = []string{Pink, Violet, Blue, Cyan}
	Aurora = []string{Mint, Cyan, Blue, Violet}
	Fire   = []string{Yellow, Orange, Red, Pink}
	Calm   = []string{Violet, Blue, Cyan}

	cText   = lipgloss.AdaptiveColor{Light: "#1E2233", Dark: "#E8EAF6"}
	cDim    = lipgloss.AdaptiveColor{Light: "#6B7186", Dark: "#7C82A0"}
	cFaint  = lipgloss.AdaptiveColor{Light: "#C5C9D6", Dark: "#363B52"}
	cKeyBg  = lipgloss.AdaptiveColor{Light: "#E6E1FF", Dark: "#262A40"}
	cHilite = lipgloss.AdaptiveColor{Light: "#EFEAFF", Dark: "#1C1F33"}

	sText  = lipgloss.NewStyle().Foreground(cText)
	sBold  = lipgloss.NewStyle().Foreground(cText).Bold(true)
	sDim   = lipgloss.NewStyle().Foreground(cDim)
	sFaint = lipgloss.NewStyle().Foreground(cFaint)
	sOK    = lipgloss.NewStyle().Foreground(lipgloss.Color(Green)).Bold(true)
	sWarn  = lipgloss.NewStyle().Foreground(lipgloss.Color(Yellow)).Bold(true)
	sErr   = lipgloss.NewStyle().Foreground(lipgloss.Color(Red)).Bold(true)
	sCyan  = lipgloss.NewStyle().Foreground(lipgloss.Color(Cyan))
	sPink  = lipgloss.NewStyle().Foreground(lipgloss.Color(Pink)).Bold(true)
	sKey   = lipgloss.NewStyle().Foreground(lipgloss.Color(Cyan)).Background(cKeyBg).Bold(true).Padding(0, 1)
)

func fg(hex string) lipgloss.Style { return lipgloss.NewStyle().Foreground(lipgloss.Color(hex)) }

// colorAt blends along the stops; t in [0,1].
func colorAt(stops []string, t float64) string {
	if len(stops) == 1 {
		return stops[0]
	}
	t = math.Max(0, math.Min(1, t))
	seg := t * float64(len(stops)-1)
	i := int(seg)
	if i >= len(stops)-1 {
		return stops[len(stops)-1]
	}
	a, _ := colorful.Hex(stops[i])
	b, _ := colorful.Hex(stops[i+1])
	return a.BlendLuv(b, seg-float64(i)).Clamped().Hex()
}

// cyclicAt wraps around, so a shifting phase animates smoothly.
func cyclicAt(stops []string, t float64) string {
	t -= math.Floor(t)
	loop := append(append([]string{}, stops...), stops[0])
	return colorAt(loop, t)
}

// Gradient colors each visible rune of plain text along the stops.
func Gradient(s string, stops ...string) string {
	return gradientFn(s, func(t float64) string { return colorAt(stops, t) }, false)
}

// Shimmer is a gradient whose colors flow with phase.
func Shimmer(s string, phase float64, stops ...string) string {
	return gradientFn(s, func(t float64) string { return cyclicAt(stops, t*0.6+phase) }, false)
}

func BoldGradient(s string, stops ...string) string {
	return gradientFn(s, func(t float64) string { return colorAt(stops, t) }, true)
}

func gradientFn(s string, color func(float64) string, bold bool) string {
	runes := []rune(s)
	n := len(runes)
	if n == 0 {
		return ""
	}
	var b strings.Builder
	for i, r := range runes {
		if r == ' ' {
			b.WriteRune(r)
			continue
		}
		t := 0.0
		if n > 1 {
			t = float64(i) / float64(n-1)
		}
		st := fg(color(t))
		if bold {
			st = st.Bold(true)
		}
		b.WriteString(st.Render(string(r)))
	}
	return b.String()
}

var eighths = []string{"", "▏", "▎", "▍", "▌", "▋", "▊", "▉"}

// Bar draws a smooth (1/8-cell) gradient bar.
func Bar(frac float64, width int, stops ...string) string {
	if width <= 0 {
		return ""
	}
	frac = math.Max(0, math.Min(1, frac))
	cells := frac * float64(width)
	full := int(cells)
	part := int((cells - float64(full)) * 8)
	var b strings.Builder
	for i := 0; i < width; i++ {
		t := float64(i) / math.Max(1, float64(width-1))
		switch {
		case i < full:
			b.WriteString(fg(colorAt(stops, t)).Render("█"))
		case i == full && part > 0:
			b.WriteString(fg(colorAt(stops, t)).Render(eighths[part]))
		default:
			b.WriteString(sFaint.Render("·"))
		}
	}
	return b.String()
}

// Meter is a bar colored by how full it is (green → yellow → red).
func Meter(frac float64, width int) string {
	stops := []string{Mint, Cyan}
	switch {
	case frac > 0.9:
		stops = []string{Orange, Red}
	case frac > 0.75:
		stops = []string{Yellow, Orange}
	}
	return Bar(frac, width, stops...)
}

var sparkRunes = []rune("▁▂▃▄▅▆▇█")

// Sparkline renders values (0..max) as a gradient sparkline of width cells.
func Sparkline(vals []float64, max float64, width int, stops ...string) string {
	if width <= 0 {
		return ""
	}
	if len(vals) > width {
		vals = vals[len(vals)-width:]
	}
	var b strings.Builder
	b.WriteString(strings.Repeat(" ", width-len(vals)))
	for i, v := range vals {
		lvl := 0
		if max > 0 {
			lvl = int(math.Round(v / max * 7))
		}
		lvl = int(math.Max(0, math.Min(7, float64(lvl))))
		t := float64(i) / math.Max(1, float64(len(vals)-1))
		b.WriteString(fg(colorAt(stops, t)).Render(string(sparkRunes[lvl])))
	}
	return b.String()
}

// Panel draws a rounded box whose border flows through the gradient stops,
// with an inline title. Body lines are truncated or padded to fit.
func Panel(title, body string, width, height int, stops ...string) string {
	if width < 6 {
		width = 6
	}
	if len(stops) == 0 {
		stops = Calm
	}
	inner := width - 2
	lines := strings.Split(strings.TrimRight(body, "\n"), "\n")
	if height > 0 {
		for len(lines) < height {
			lines = append(lines, "")
		}
		lines = lines[:height]
	}
	t := ""
	if title != "" {
		t = " " + title + " "
	}
	tw := lipgloss.Width(t)
	topRunes := []rune("╭─")
	var top strings.Builder
	pos := 0
	edge := func(r string) {
		top.WriteString(fg(colorAt(stops, float64(pos)/float64(width-1))).Render(r))
		pos++
	}
	for _, r := range topRunes {
		edge(string(r))
	}
	top.WriteString(t)
	pos += tw
	for pos < width-1 {
		edge("─")
	}
	edge("╮")

	left := fg(stops[0]).Render("│")
	right := fg(stops[len(stops)-1]).Render("│")
	var b strings.Builder
	b.WriteString(top.String() + "\n")
	for _, l := range lines {
		l = ansi.Truncate(l, inner-2, "…")
		pad := inner - 2 - lipgloss.Width(l)
		if pad < 0 {
			pad = 0
		}
		b.WriteString(left + " " + l + strings.Repeat(" ", pad) + " " + right + "\n")
	}
	var bottom strings.Builder
	for i := 0; i < width; i++ {
		r := "─"
		if i == 0 {
			r = "╰"
		} else if i == width-1 {
			r = "╯"
		}
		bottom.WriteString(fg(colorAt(stops, float64(i)/float64(width-1))).Render(r))
	}
	b.WriteString(bottom.String())
	return b.String()
}

// Wrap word-wraps prose to width cells (ANSI-aware).
func Wrap(s string, width int) string { return ansi.Wordwrap(s, width, " ") }

// ProsePanel is a Panel whose body is word-wrapped instead of truncated.
func ProsePanel(title, body string, width, height int, stops ...string) string {
	return Panel(title, Wrap(body, width-4), width, height, stops...)
}

// Keys renders a help line of keycaps: Keys("↑↓", "move", "enter", "open").
func Keys(pairs ...string) string {
	var parts []string
	for i := 0; i+1 < len(pairs); i += 2 {
		parts = append(parts, sKey.Render(pairs[i])+" "+sDim.Render(pairs[i+1]))
	}
	return strings.Join(parts, "  ")
}

// badgeColors maps known badge names to their color.
var badgeColors = map[string]string{
	"high": Green, "medium": Yellow, "low": "#8A90A8", "shared": Orange,
	"browsers": Cyan, "developer": Violet, "system": Blue, "apps": Pink,
	"recent": Yellow, "old": Mint, "dir": Blue, "app": Violet,
}

// Badge renders a small colored pill with fixed width.
func Badge(text string, width int) string {
	if text == "" {
		return strings.Repeat(" ", width)
	}
	c, ok := badgeColors[strings.ToLower(text)]
	if !ok {
		c = Violet
	}
	label := strings.ToUpper(text)
	if len([]rune(label)) > width-2 {
		label = string([]rune(label)[:width-2])
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color("#0E0F1A")).Background(lipgloss.Color(c)).Bold(true).
		Width(width).Align(lipgloss.Center).Render(label)
}

// AITag renders the AI verdict marker.
func AITag(verdict string) string {
	switch verdict {
	case "remove":
		return fg(Mint).Bold(true).Render("✦AI✓")
	case "keep":
		return fg(Red).Bold(true).Render("✦AI✗")
	case "unsure":
		return fg(Yellow).Bold(true).Render("✦AI?")
	}
	return ""
}

func Size(n int64) string { return ui.Size(n) }

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func padRight(s string, w int) string {
	if d := w - lipgloss.Width(s); d > 0 {
		return s + strings.Repeat(" ", d)
	}
	return s
}

func padLeft(s string, w int) string {
	if d := w - lipgloss.Width(s); d > 0 {
		return strings.Repeat(" ", d) + s
	}
	return s
}

// Risk renders a low/medium/high risk label in traffic-light colors.
func Risk(r string) string {
	c := map[string]string{"low": Mint, "medium": Yellow, "high": Red}[r]
	if c == "" {
		return ""
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color("#0E0F1A")).Background(lipgloss.Color(c)).Bold(true).
		Padding(0, 1).Render(strings.ToUpper(r) + " RISK")
}

// Bullet renders a sparkle list line with a dim explanation.
func Bullet(main, sub string) string {
	line := "  " + fg(Pink).Render("✦ ") + sBold.Render(main)
	if sub != "" {
		line += "  " + sDim.Render(sub)
	}
	return line
}

// Dim and BoldText expose the theme's text styles to callers.
func Dim(s string) string      { return sDim.Render(s) }
func BoldText(s string) string { return sBold.Render(s) }
