package tui

import "strings"

// Block letters in the "ANSI Shadow" style.
var glyphs = map[rune][]string{
	'S': {
		"███████╗",
		"██╔════╝",
		"███████╗",
		"╚════██║",
		"███████║",
		"╚══════╝",
	},
	'W': {
		"██╗    ██╗",
		"██║    ██║",
		"██║ █╗ ██║",
		"██║███╗██║",
		"╚███╔███╔╝",
		" ╚══╝╚══╝ ",
	},
	'E': {
		"███████╗",
		"██╔════╝",
		"█████╗  ",
		"██╔══╝  ",
		"███████╗",
		"╚══════╝",
	},
	'P': {
		"██████╗ ",
		"██╔══██╗",
		"██████╔╝",
		"██╔═══╝ ",
		"██║     ",
		"╚═╝     ",
	},
	'R': {
		"██████╗ ",
		"██╔══██╗",
		"██████╔╝",
		"██╔══██╗",
		"██║  ██║",
		"╚═╝  ╚═╝",
	},
}

var logoLines = buildLogo("SWEEPR")

func buildLogo(word string) []string {
	rows := make([]string, 6)
	for _, r := range word {
		for i, l := range glyphs[r] {
			rows[i] += l
		}
	}
	return rows
}

// LogoWidth is the width of the big logo in cells.
func LogoWidth() int { return len([]rune(logoLines[0])) }

// Logo renders the block logo with a flowing diagonal gradient. Solid
// blocks get the bright color, the box-drawing shadow a dimmer one.
func Logo(phase float64) string {
	w := float64(LogoWidth())
	var b strings.Builder
	for y, line := range logoLines {
		for x, r := range []rune(line) {
			if r == ' ' {
				b.WriteRune(' ')
				continue
			}
			t := float64(x)/w*0.8 + float64(y)*0.04 + phase
			c := cyclicAt(Neon, t)
			if r != '█' {
				c = cyclicAt([]string{"#5A3C8C", "#2E4A8C", "#1F6B80", "#6B2E70"}, t)
			}
			b.WriteString(fg(c).Render(string(r)))
		}
		if y < len(logoLines)-1 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

// SmallLogo is used when the terminal is too narrow for the block logo.
func SmallLogo(phase float64) string {
	return Shimmer("◆ s w e e p r ◆", phase, Neon...)
}
