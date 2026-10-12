package tui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// Item is one row of a picker.
type Item struct {
	Label    string // main text
	Sub      string // dim text after the label
	Badge    string // pill on the left: high/medium/low/shared, a category…
	AI       string // AI verdict: remove/keep/unsure
	Detail   string // shown in the detail pane for the highlighted row
	Size     int64  // <0 unknown, 0 hidden
	Selected bool
}

// PickOptions configures a picker.
type PickOptions struct {
	Icon     string
	Title    string
	Subtitle string
	Items    []Item
	Single   bool   // choose one row with enter (no checkboxes)
	Filter   bool   // start with the filter focused (type to search)
	Action   string // verb on the enter key, e.g. "clean"
}

type sortMode int

const (
	sortNatural sortMode = iota
	sortSize
	sortName
)

var sortNames = []string{"natural", "size", "name"}

type pickModel struct {
	opt       PickOptions
	items     []Item
	view      []int
	cursor    int
	offset    int
	filter    string
	filtering bool
	sort      sortMode
	w, h      int
	frame     int
	done      bool
	ok        bool
	maxSize   int64
	badgeW    int
	hasSize   bool
}

type pickTick struct{}

func pickTickCmd() tea.Cmd {
	return tea.Tick(90*time.Millisecond, func(time.Time) tea.Msg { return pickTick{} })
}

func newPick(opt PickOptions) pickModel {
	m := pickModel{opt: opt, items: append([]Item(nil), opt.Items...), filtering: opt.Filter, w: 100, h: 30}
	for _, it := range m.items {
		m.maxSize = max(m.maxSize, it.Size)
		m.hasSize = m.hasSize || it.Size != 0
		if it.Badge != "" {
			m.badgeW = max(m.badgeW, len([]rune(it.Badge))+2)
		}
	}
	m.rebuild()
	return m
}

func (m *pickModel) rebuild() {
	q := strings.ToLower(m.filter)
	m.view = m.view[:0]
	for i, it := range m.items {
		if q == "" || fuzzy(strings.ToLower(it.Label+" "+it.Sub+" "+it.Badge), q) {
			m.view = append(m.view, i)
		}
	}
	switch m.sort {
	case sortSize:
		sort.SliceStable(m.view, func(a, b int) bool { return m.items[m.view[a]].Size > m.items[m.view[b]].Size })
	case sortName:
		sort.SliceStable(m.view, func(a, b int) bool {
			return strings.ToLower(m.items[m.view[a]].Label) < strings.ToLower(m.items[m.view[b]].Label)
		})
	}
	m.cursor = clamp(m.cursor, 0, max(0, len(m.view)-1))
}

// fuzzy matches when every query rune appears in order.
func fuzzy(s, q string) bool {
	if strings.Contains(s, q) {
		return true
	}
	rs := []rune(s)
	i := 0
	for _, c := range q {
		for i < len(rs) && rs[i] != c {
			i++
		}
		if i == len(rs) {
			return false
		}
		i++
	}
	return true
}

func (m pickModel) Init() tea.Cmd { return pickTickCmd() }

func (m pickModel) listHeight() int {
	return max(3, m.h-12)
}

func (m pickModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
	case pickTick:
		m.frame++
		return m, pickTickCmd()
	case tea.KeyMsg:
		k := msg.String()
		if k == "ctrl+c" {
			m.done, m.ok = true, false
			return m, tea.Quit
		}
		if m.filtering {
			switch k {
			case "esc":
				if m.filter != "" {
					m.filter = ""
					m.rebuild()
				} else if m.opt.Single {
					m.done = true
					return m, tea.Quit
				} else {
					m.filtering = false
				}
				return m, nil
			case "enter":
				if m.opt.Single {
					return m.choose()
				}
				m.filtering = false
				return m, nil
			case "backspace":
				if r := []rune(m.filter); len(r) > 0 {
					m.filter = string(r[:len(r)-1])
					m.rebuild()
				}
				return m, nil
			case "up", "down", "pgup", "pgdown":
				// fall through to navigation below
			default:
				if msg.Type == tea.KeyRunes || k == " " {
					m.filter += string(msg.Runes)
					m.cursor = 0
					m.rebuild()
				}
				return m, nil
			}
		}
		switch k {
		case "up", "k":
			m.cursor--
		case "down", "j":
			m.cursor++
		case "pgup", "ctrl+u":
			m.cursor -= m.listHeight()
		case "pgdown", "ctrl+d":
			m.cursor += m.listHeight()
		case "home", "g":
			m.cursor = 0
		case "end", "G":
			m.cursor = len(m.view) - 1
		case " ", "x":
			if !m.opt.Single && len(m.view) > 0 {
				it := &m.items[m.view[m.cursor]]
				it.Selected = !it.Selected
				m.cursor++
			}
		case "a":
			m.setVisible(func(bool) bool { return true })
		case "n":
			m.setVisible(func(bool) bool { return false })
		case "i":
			m.setVisible(func(v bool) bool { return !v })
		case "/":
			m.filtering = true
		case "s":
			m.sort = (m.sort + 1) % 3
			m.rebuild()
		case "enter":
			return m.choose()
		case "esc", "q":
			if m.filter != "" {
				m.filter = ""
				m.rebuild()
				return m, nil
			}
			m.done, m.ok = true, false
			return m, tea.Quit
		}
		m.cursor = clamp(m.cursor, 0, max(0, len(m.view)-1))
		lh := m.listHeight()
		if m.cursor < m.offset {
			m.offset = m.cursor
		}
		if m.cursor >= m.offset+lh {
			m.offset = m.cursor - lh + 1
		}
	}
	return m, nil
}

func (m *pickModel) setVisible(f func(bool) bool) {
	if m.opt.Single {
		return
	}
	for _, i := range m.view {
		m.items[i].Selected = f(m.items[i].Selected)
	}
}

func (m pickModel) choose() (tea.Model, tea.Cmd) {
	if m.opt.Single {
		if len(m.view) == 0 {
			return m, nil
		}
		for i := range m.items {
			m.items[i].Selected = i == m.view[m.cursor]
		}
	}
	m.done, m.ok = true, true
	return m, tea.Quit
}

func (m pickModel) totals() (int, int64) {
	n, total := 0, int64(0)
	for _, it := range m.items {
		if it.Selected {
			n++
			total += max(it.Size, 0)
		}
	}
	return n, total
}

func (m pickModel) View() string {
	if m.done {
		return ""
	}
	w := max(60, m.w)
	phase := float64(m.frame) / 45
	var b strings.Builder

	// Header
	n, total := m.totals()
	title := m.opt.Icon + " " + BoldGradient(m.opt.Title, Neon...)
	right := ""
	if m.opt.Single {
		right = sDim.Render(fmt.Sprintf("%d items", len(m.items)))
	} else {
		right = sBold.Render(fmt.Sprintf("%d", n)) + sDim.Render(fmt.Sprintf("/%d selected  ", len(m.items))) +
			BoldGradient(Size(total), Neon...)
	}
	gap := w - 4 - lipgloss.Width(title) - lipgloss.Width(right)
	b.WriteString("\n  " + title + strings.Repeat(" ", max(1, gap)) + right + "\n")
	b.WriteString("  " + Shimmer(strings.Repeat("━", w-4), phase, Neon...) + "\n")

	// Subtitle / filter line
	if m.filtering || m.filter != "" {
		cursor := " "
		if m.filtering && m.frame%10 < 6 {
			cursor = "▌"
		}
		b.WriteString("  " + sPink.Render("⌕ ") + sBold.Render(m.filter) + fg(Pink).Render(cursor) +
			sDim.Render(fmt.Sprintf("   %d matches", len(m.view))) + "\n")
	} else {
		sub := m.opt.Subtitle
		if m.sort != sortNatural {
			sub = strings.TrimSpace(sub + "   sorted by " + sortNames[m.sort])
		}
		b.WriteString("  " + sDim.Render(ansi.Truncate(sub, w-4, "…")) + "\n")
	}
	b.WriteString("\n")

	// Rows
	lh := m.listHeight()
	end := min(len(m.view), m.offset+lh)
	for row := m.offset; row < end; row++ {
		b.WriteString(m.renderRow(row, w) + "\n")
	}
	for row := end - m.offset; row < lh; row++ {
		if row == 0 && len(m.view) == 0 {
			b.WriteString("    " + sDim.Render("no matches") + "\n")
			continue
		}
		b.WriteString("\n")
	}

	// Scroll indicator
	if len(m.view) > lh {
		frac := float64(m.offset) / float64(max(1, len(m.view)-lh))
		b.WriteString("  " + sDim.Render(fmt.Sprintf("%d–%d of %d ", m.offset+1, end, len(m.view))) + Bar(frac, 12, Calm...) + "\n")
	} else {
		b.WriteString("\n")
	}

	// Detail pane
	detail := ""
	if len(m.view) > 0 {
		it := m.items[m.view[m.cursor]]
		detail = sBold.Render(it.Label)
		if it.Detail != "" {
			detail += "\n" + sDim.Render(it.Detail)
		}
	}
	b.WriteString(Panel(sDim.Render("details"), detail, w-2, 3, Calm...) + "\n")

	// Keys
	var keys string
	switch {
	case m.filtering:
		keys = Keys("type", "filter", "↑↓", "move", "enter", map[bool]string{true: "choose", false: "done"}[m.opt.Single], "esc", "clear")
	case m.opt.Single:
		keys = Keys("↑↓", "move", "/", "filter", "s", "sort", "enter", "choose", "esc", "cancel")
	default:
		action := m.opt.Action
		if action == "" {
			action = "continue"
		}
		keys = Keys("↑↓", "move", "space", "toggle", "a/n/i", "all/none/invert", "/", "filter", "s", "sort", "enter", action, "esc", "cancel")
	}
	b.WriteString(" " + ansi.Truncate(keys, w-2, "…"))
	return b.String()
}

func (m pickModel) renderRow(row, w int) string {
	idx := m.view[row]
	it := m.items[idx]
	active := row == m.cursor

	pointer := "  "
	if active {
		pointer = fg(cyclicAt(Neon, float64(m.frame)/20)).Bold(true).Render("▶ ")
	}
	check := ""
	if !m.opt.Single {
		if it.Selected {
			check = sOK.Render("◉ ")
		} else {
			check = sFaint.Render("○ ")
		}
	}
	badge := ""
	if m.badgeW > 0 {
		badge = Badge(it.Badge, m.badgeW) + " "
	}
	size := ""
	if m.hasSize {
		size = strings.Repeat(" ", 9)
	}
	if it.Size != 0 {
		st := sDim
		if it.Selected || m.opt.Single {
			st = sBold
		}
		size = st.Render(padLeft(Size(it.Size), 9))
	}
	bar := ""
	if m.maxSize > 0 {
		frac := 0.0
		if it.Size > 0 {
			frac = float64(it.Size) / float64(m.maxSize)
		}
		bar = " " + Bar(frac, 10, Neon...) + " "
	}
	ai := ""
	if it.AI != "" {
		ai = " " + AITag(it.AI)
	}
	prefix := " " + pointer + check + badge + size + bar
	room := w - lipgloss.Width(prefix) - lipgloss.Width(ai) - 3
	label := ansi.Truncate(it.Label, max(10, room), "…")
	var labelR string
	switch {
	case active:
		labelR = BoldGradient(label, Pink, Violet, Cyan)
	case it.Selected || m.opt.Single:
		labelR = sText.Render(label)
	default:
		labelR = sDim.Render(label)
	}
	line := prefix + labelR
	if it.Sub != "" {
		if left := room - lipgloss.Width(label) - 2; left > 6 {
			line += "  " + sFaint.Render(ansi.Truncate(it.Sub, left, "…"))
		}
	}
	line += ai
	if active {
		line = lipgloss.NewStyle().Background(cHilite).Render(padRight(line, w-1))
	}
	return line
}

// Pick runs the picker full-screen. It updates Selected on opt.Items in
// place and reports whether the user confirmed (enter) or cancelled.
func Pick(opt PickOptions) bool {
	res, err := tea.NewProgram(newPick(opt), tea.WithAltScreen()).Run()
	if err != nil {
		return false
	}
	m := res.(pickModel)
	if !m.ok {
		return false
	}
	for i := range opt.Items {
		opt.Items[i].Selected = m.items[i].Selected
	}
	return true
}

// PickOne returns the index of the chosen item, or -1 if cancelled.
func PickOne(opt PickOptions) int {
	opt.Single = true
	if !Pick(opt) {
		return -1
	}
	for i, it := range opt.Items {
		if it.Selected {
			return i
		}
	}
	return -1
}
