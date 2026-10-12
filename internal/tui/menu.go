package tui

import (
	"fmt"
	"math"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// MenuItem is one action on the home screen.
type MenuItem struct {
	ID    string
	Icon  string
	Title string
	Desc  string
}

var MenuItems = []MenuItem{
	{"clean", "🧹", "Clean", "caches, logs, temp files, browser & dev junk"},
	{"uninstall", "🚮", "Uninstall", "remove an app and every file it left behind"},
	{"leftovers", "👻", "Leftovers", "data from apps you already deleted"},
	{"purge", "📦", "Purge", "node_modules, target/, .venv in your projects"},
	{"installer", "💿", "Installers", "old .dmg · .pkg · .msi · setup.exe files"},
	{"analyze", "📊", "Analyze", "explore what is eating your disk"},
	{"status", "⚡", "Status", "live CPU · memory · disk · network"},
	{"ai", "✨", "AI advisor", "Claude builds a cleanup plan for this machine"},
	{"history", "📜", "History", "everything sweepr removed, and when"},
}

type menuModel struct {
	cursor  int
	w, h    int
	frame   int
	s       *Sampler
	cur     Sample
	have    bool
	chosen  string
	version string
	osName  string
}

type menuSample Sample

func menuSampleCmd(s *Sampler, d time.Duration) tea.Cmd {
	return tea.Tick(d, func(time.Time) tea.Msg { return menuSample(s.Sample()) })
}

func (m menuModel) Init() tea.Cmd {
	return tea.Batch(animFast(), menuSampleCmd(m.s, 200*time.Millisecond))
}

func animFast() tea.Cmd {
	return tea.Tick(60*time.Millisecond, func(time.Time) tea.Msg { return animTick{} })
}

func (m menuModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
	case animTick:
		m.frame++
		return m, animFast()
	case menuSample:
		m.cur, m.have = Sample(msg), true
		return m, menuSampleCmd(m.s, 1500*time.Millisecond)
	case tea.KeyMsg:
		k := msg.String()
		switch k {
		case "up", "k", "shift+tab":
			m.cursor = (m.cursor - 1 + len(MenuItems)) % len(MenuItems)
		case "down", "j", "tab":
			m.cursor = (m.cursor + 1) % len(MenuItems)
		case "enter", " ", "right", "l":
			m.chosen = MenuItems[m.cursor].ID
			return m, tea.Quit
		case "q", "esc", "ctrl+c":
			return m, tea.Quit
		default:
			if len(k) == 1 && k[0] >= '1' && k[0] <= '9' {
				if i := int(k[0] - '1'); i < len(MenuItems) {
					m.cursor = i
					m.chosen = MenuItems[i].ID
					return m, tea.Quit
				}
			}
		}
	}
	return m, nil
}

func (m menuModel) View() string {
	w, h := max(50, m.w), max(20, m.h)
	phase := float64(m.frame) / 70
	var top strings.Builder

	// Logo + tagline
	logo := SmallLogo(phase)
	if w >= LogoWidth()+4 && h >= 30 {
		logo = Logo(phase)
	}
	top.WriteString(lipgloss.PlaceHorizontal(w, lipgloss.Center, logo) + "\n")
	tag := Shimmer("deep clean  ·  complete uninstall  ·  AI review", phase*1.5, Calm...)
	top.WriteString(lipgloss.PlaceHorizontal(w, lipgloss.Center, tag) + "\n")
	meta := sDim.Render(fmt.Sprintf("v%s · %s", m.version, m.osName))
	top.WriteString(lipgloss.PlaceHorizontal(w, lipgloss.Center, meta) + "\n\n")

	// Menu panel
	menuW := clamp(w-42, 50, 76)
	var mb strings.Builder
	for i, it := range MenuItems {
		active := i == m.cursor
		num := sFaint.Render(fmt.Sprintf("%d", i+1))
		pointer := "  "
		title := sText.Render(padRight(it.Title, 12))
		desc := sDim.Render(it.Desc)
		if active {
			pulse := (math.Sin(float64(m.frame)/4) + 1) / 2
			pointer = fg(colorAt([]string{Pink, Cyan}, pulse)).Bold(true).Render("▶ ")
			title = BoldGradient(padRight(it.Title, 12), Pink, Violet, Cyan)
			desc = sText.Render(it.Desc)
			num = sPink.Render(fmt.Sprintf("%d", i+1))
		}
		mb.WriteString(pointer + num + " " + it.Icon + "  " + title + desc + "\n")
	}
	menu := Panel(BoldGradient("actions", Neon...), mb.String(), menuW, len(MenuItems), Neon...)

	// Live system panel
	sysW := 38
	var sb strings.Builder
	if m.have {
		s := m.cur
		for _, d := range s.Disks {
			if d.Total == 0 {
				continue
			}
			used := float64(d.Total-d.Free) / float64(d.Total)
			sb.WriteString(sDim.Render(padRight("disk", 7)) + Meter(used, 16) + fmt.Sprintf(" %3.0f%%", used*100) + "\n")
			sb.WriteString(sDim.Render(padRight("", 7)) + sBold.Render(Size(int64(d.Free))) + sDim.Render(" free") + "\n")
			break
		}
		sb.WriteString(sDim.Render(padRight("memory", 7)) + Meter(s.MemPct/100, 16) + fmt.Sprintf(" %3.0f%%", s.MemPct) + "\n")
		sb.WriteString(sDim.Render(padRight("cpu", 7)) + Sparkline(s.CPUHist, 100, 16, Mint, Cyan, Pink) + fmt.Sprintf(" %3.0f%%", s.CPU) + "\n")
		score, label, color := Health(s)
		sb.WriteString("\n" + sDim.Render(padRight("health", 7)) + fg(color).Bold(true).Render(fmt.Sprintf("%d · %s", score, label)) + "\n")
		if s.Uptime > 0 {
			sb.WriteString(sDim.Render(padRight("uptime", 7)) + sText.Render(dur(s.Uptime)) + "\n")
		}
	} else {
		sb.WriteString(sDim.Render(spinFrames[m.frame%len(spinFrames)] + " reading sensors…"))
	}
	sys := Panel(BoldGradient("system", Aurora...), sb.String(), sysW, len(MenuItems), Aurora...)

	var body string
	if w >= menuW+sysW+3 {
		body = lipgloss.JoinHorizontal(lipgloss.Top, menu, " ", sys)
	} else {
		body = menu
	}
	top.WriteString(lipgloss.PlaceHorizontal(w, lipgloss.Center, body) + "\n\n")
	keys := Keys("↑↓", "move", "enter", "open", "1-9", "jump", "q", "quit")
	top.WriteString(lipgloss.PlaceHorizontal(w, lipgloss.Center, keys))

	out := top.String()
	if pad := (h - lipgloss.Height(out)) / 2; pad > 0 {
		out = strings.Repeat("\n", pad) + out
	}
	return out
}

// Menu shows the animated home screen and returns the chosen action ID
// ("" when the user quits). start pre-selects an item.
func Menu(version, osName string, start int) string {
	m := menuModel{s: NewSampler(false), version: version, osName: osName, cursor: start, w: 100, h: 34}
	res, err := tea.NewProgram(m, tea.WithAltScreen()).Run()
	if err != nil {
		return ""
	}
	return res.(menuModel).chosen
}

// MenuIndex returns the position of an action, for returning to it.
func MenuIndex(id string) int {
	for i, it := range MenuItems {
		if it.ID == id {
			return i
		}
	}
	return 0
}
