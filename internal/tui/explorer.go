package tui

import (
	"fmt"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/santu-1506/sweepey/internal/analyze"
	"github.com/santu-1506/sweepey/internal/fsutil"
)

// ExploreOptions wires the explorer to the rest of sweepr.
type ExploreOptions struct {
	Start     string
	TrashName string // "Trash" or "Recycle Bin"
	Trash     func(path string, size int64) error
	CanTrash  func(path string) error // refuses protected locations up front
	Reveal    func(path string) error
	Explain   func(path string) (string, error) // nil when AI is unavailable
}

type exploreModel struct {
	opt      ExploreOptions
	dir      string
	entries  []analyze.Entry
	total    int64
	cache    map[string][]analyze.Entry
	cursors  map[string]int
	cursor   int
	offset   int
	loading  bool
	err      error
	w, h     int
	frame    int
	confirm  bool
	status   string
	panel    string // AI explanation
	thinking bool
}

type loadedMsg struct {
	dir     string
	entries []analyze.Entry
	err     error
}
type explainedMsg struct {
	text string
	err  error
}
type trashedMsg struct {
	path string
	err  error
}

func loadCmd(dir string) tea.Cmd {
	return func() tea.Msg {
		e, err := analyze.Children(dir)
		return loadedMsg{dir, e, err}
	}
}

func (m exploreModel) Init() tea.Cmd { return tea.Batch(loadCmd(m.dir), animCmd()) }

func (m *exploreModel) open(dir string) tea.Cmd {
	m.cursors[m.dir] = m.cursor
	m.dir, m.panel, m.status, m.confirm = dir, "", "", false
	m.cursor = m.cursors[dir]
	m.offset = 0
	if e, ok := m.cache[dir]; ok {
		m.setEntries(e)
		return nil
	}
	m.loading = true
	return loadCmd(dir)
}

func (m *exploreModel) setEntries(e []analyze.Entry) {
	m.entries, m.loading, m.total = e, false, 0
	for _, x := range e {
		m.total += max(x.Size, 0)
	}
	m.cursor = clamp(m.cursor, 0, max(0, len(e)-1))
}

func (m exploreModel) listHeight() int { return max(4, m.h-11) }

func (m exploreModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
	case animTick:
		m.frame++
		return m, animCmd()
	case loadedMsg:
		if msg.err != nil {
			m.cache[msg.dir] = nil
		} else {
			m.cache[msg.dir] = msg.entries
		}
		if msg.dir == m.dir {
			m.err = msg.err
			m.setEntries(msg.entries)
		}
	case explainedMsg:
		m.thinking = false
		if msg.err != nil {
			m.panel = sErr.Render("AI: ") + msg.err.Error()
		} else {
			m.panel = msg.text
		}
	case trashedMsg:
		if msg.err != nil {
			m.status = sErr.Render("✗ ") + msg.err.Error()
		} else {
			m.status = sOK.Render("✔ moved to the "+m.opt.TrashName+": ") + filepath.Base(msg.path)
			delete(m.cache, m.dir)
			for p := filepath.Dir(m.dir); ; p = filepath.Dir(p) {
				delete(m.cache, p)
				if p == filepath.Dir(p) {
					break
				}
			}
			m.loading = true
			return m, loadCmd(m.dir)
		}
	case tea.KeyMsg:
		k := msg.String()
		if m.confirm {
			m.confirm = false
			if (k == "y" || k == "Y") && len(m.entries) > 0 {
				e := m.entries[m.cursor]
				m.status = sDim.Render("moving " + e.Name + " to the " + m.opt.TrashName + "…")
				return m, func() tea.Msg { return trashedMsg{e.Path, m.opt.Trash(e.Path, e.Size)} }
			}
			m.status = sDim.Render("cancelled")
			return m, nil
		}
		switch k {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "esc":
			if m.panel != "" {
				m.panel = ""
				return m, nil
			}
			return m, tea.Quit
		case "up", "k":
			m.cursor--
		case "down", "j":
			m.cursor++
		case "pgup":
			m.cursor -= m.listHeight()
		case "pgdown":
			m.cursor += m.listHeight()
		case "home", "g":
			m.cursor = 0
		case "end", "G":
			m.cursor = len(m.entries) - 1
		case "enter", "right", "l":
			if len(m.entries) > 0 && m.entries[m.cursor].IsDir && !m.loading {
				return m, m.open(m.entries[m.cursor].Path)
			}
		case "left", "h", "backspace", "u":
			if parent := filepath.Dir(m.dir); parent != m.dir && !m.loading {
				return m, m.open(parent)
			}
		case "r":
			if len(m.entries) > 0 && m.opt.Reveal != nil {
				_ = m.opt.Reveal(m.entries[m.cursor].Path)
				m.status = sDim.Render("revealed " + m.entries[m.cursor].Name)
			}
		case "t", "delete":
			if len(m.entries) > 0 && m.opt.Trash != nil {
				if m.opt.CanTrash != nil {
					if err := m.opt.CanTrash(m.entries[m.cursor].Path); err != nil {
						m.status = sWarn.Render("🛡 ") + sDim.Render(err.Error())
						return m, nil
					}
				}
				m.confirm = true
			}
		case "e", "?":
			if len(m.entries) > 0 && !m.thinking {
				if m.opt.Explain == nil {
					m.panel = sDim.Render("Set ANTHROPIC_API_KEY to let Claude explain folders.")
					return m, nil
				}
				m.thinking, m.panel = true, ""
				path := m.entries[m.cursor].Path
				return m, func() tea.Msg { t, err := m.opt.Explain(path); return explainedMsg{t, err} }
			}
		}
		m.cursor = clamp(m.cursor, 0, max(0, len(m.entries)-1))
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

func (m exploreModel) View() string {
	w := max(64, m.w)
	phase := float64(m.frame) / 50
	var b strings.Builder

	// Header with breadcrumb
	crumbs := strings.Split(strings.Trim(filepath.ToSlash(fsutil.Tilde(m.dir)), "/"), "/")
	for i, c := range crumbs {
		if i == len(crumbs)-1 {
			crumbs[i] = BoldGradient(c, Pink, Cyan)
		} else {
			crumbs[i] = sDim.Render(c)
		}
	}
	path := strings.Join(crumbs, sFaint.Render(" › "))
	if strings.HasPrefix(filepath.ToSlash(m.dir), "/") && !strings.HasPrefix(fsutil.Tilde(m.dir), "~") {
		path = sFaint.Render("/ ") + path
	}
	right := BoldGradient(Size(m.total), Neon...) + sDim.Render(fmt.Sprintf("  %d items", len(m.entries)))
	left := "  📊 " + path
	gap := w - lipgloss.Width(left) - lipgloss.Width(right) - 2
	b.WriteString("\n" + left + strings.Repeat(" ", max(1, gap)) + right + "\n")
	b.WriteString("  " + Shimmer(strings.Repeat("━", w-4), phase, Neon...) + "\n\n")

	lh := m.listHeight()
	if m.loading {
		spin := fg(cyclicAt(Neon, phase*3)).Render(spinFrames[m.frame%len(spinFrames)])
		b.WriteString("  " + spin + " " + Shimmer("measuring "+fsutil.Tilde(m.dir)+" …", phase*2, Neon...) + "\n")
		b.WriteString(strings.Repeat("\n", lh-1))
	} else {
		if m.err != nil {
			b.WriteString("  " + sErr.Render("✗ ") + sDim.Render(m.err.Error()) + "\n")
			lh--
		}
		var maxSize int64
		for _, e := range m.entries {
			maxSize = max(maxSize, e.Size)
		}
		barW := clamp(w/4, 10, 34)
		end := min(len(m.entries), m.offset+lh)
		for i := m.offset; i < end; i++ {
			e := m.entries[i]
			active := i == m.cursor
			pct := 0.0
			if m.total > 0 && e.Size > 0 {
				pct = float64(e.Size) / float64(m.total)
			}
			frac := 0.0
			if maxSize > 0 && e.Size > 0 {
				frac = float64(e.Size) / float64(maxSize)
			}
			pointer := "  "
			if active {
				pointer = fg(cyclicAt(Neon, phase*2)).Bold(true).Render("▶ ")
			}
			name := e.Name
			icon := "  "
			if e.IsDir {
				name += "/"
				icon = "▸ "
			}
			name = ansi.Truncate(name, max(10, w-barW-30), "…")
			var nameR string
			switch {
			case active:
				nameR = BoldGradient(name, Pink, Violet, Cyan)
			case e.IsDir:
				nameR = fg(Blue).Bold(true).Render(name)
			default:
				nameR = sText.Render(name)
			}
			line := " " + pointer + sBold.Render(padLeft(Size(e.Size), 9)) + " " + sDim.Render(fmt.Sprintf("%5.1f%%", pct*100)) + " " +
				Bar(frac, barW, Neon...) + " " + sFaint.Render(icon) + nameR
			if active {
				line = lipgloss.NewStyle().Background(cHilite).Render(padRight(line, w-1))
			}
			b.WriteString(line + "\n")
		}
		for i := end - m.offset; i < lh; i++ {
			b.WriteString("\n")
		}
	}

	// Bottom panel: AI explanation, confirmation or status
	var bottom string
	switch {
	case m.confirm && len(m.entries) > 0:
		e := m.entries[m.cursor]
		bottom = Panel(sErr.Render("⚠ move to "+m.opt.TrashName), sBold.Render(fmt.Sprintf("Move %s (%s) to the %s?", e.Name, Size(e.Size), m.opt.TrashName))+
			"\n"+Keys("y", "yes", "any key", "cancel"), w-2, 2, Fire...)
	case m.thinking:
		bottom = Panel(Shimmer("✨ Claude is thinking", phase*3, Neon...),
			sDim.Render(spinFrames[m.frame%len(spinFrames)]+" looking at "+m.entries[m.cursor].Name+" …"), w-2, 2, Neon...)
	case m.panel != "":
		bottom = Panel(BoldGradient("✨ AI", Neon...), m.panel, w-2, 5, Neon...)
	default:
		bottom = "  " + m.status
	}
	b.WriteString(bottom + "\n")
	keys := Keys("↑↓", "move", "→/enter", "open", "←", "back", "r", "reveal", "t", "trash", "e", "AI explain", "q", "quit")
	b.WriteString(" " + ansi.Truncate(keys, w-2, "…"))
	return b.String()
}

// Explore runs the interactive disk explorer.
func Explore(opt ExploreOptions) error {
	m := exploreModel{opt: opt, dir: opt.Start, cache: map[string][]analyze.Entry{}, cursors: map[string]int{},
		loading: true, w: 100, h: 32}
	_, err := tea.NewProgram(m, tea.WithAltScreen()).Run()
	return err
}
