package tui

import (
	"fmt"
	"math"
	"math/rand"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// ─── Task: animated spinner with optional progress bar ────────────────────

// Update reports progress from inside a Task.
type Update struct {
	Label string
	Done  int
	Total int
}

type taskModel struct {
	title   string
	upd     Update
	start   time.Time
	frame   int
	done    bool
	err     error
	summary string
	width   int
}

type taskTick struct{}
type taskDone struct {
	err     error
	summary string
}

var spinFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

func taskTickCmd() tea.Cmd {
	return tea.Tick(70*time.Millisecond, func(time.Time) tea.Msg { return taskTick{} })
}

func (m taskModel) Init() tea.Cmd { return taskTickCmd() }

func (m taskModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
	case taskTick:
		m.frame++
		return m, taskTickCmd()
	case Update:
		m.upd = msg
	case taskDone:
		m.done, m.err, m.summary = true, msg.err, msg.summary
		return m, tea.Quit
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			m.done, m.err = true, errInterrupted
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m taskModel) View() string {
	el := sDim.Render(fmt.Sprintf("%.1fs", time.Since(m.start).Seconds()))
	if m.done {
		if m.err != nil {
			return "  " + sErr.Render("✗ ") + sBold.Render(m.title) + "  " + sDim.Render(m.err.Error()) + "\n"
		}
		line := "  " + sOK.Render("✔ ") + sBold.Render(m.title)
		if m.summary != "" {
			line += "  " + sDim.Render(m.summary)
		}
		return line + "  " + el + "\n"
	}
	spin := fg(cyclicAt(Neon, float64(m.frame)/24)).Render(spinFrames[m.frame%len(spinFrames)])
	line := "  " + spin + " " + Shimmer(m.title, float64(m.frame)/30, Neon...)
	if m.upd.Total > 0 {
		frac := float64(m.upd.Done) / float64(m.upd.Total)
		line += "  " + Bar(frac, 22, Neon...) + " " + sDim.Render(fmt.Sprintf("%d/%d", m.upd.Done, m.upd.Total))
	}
	line += "  " + el
	if m.upd.Label != "" {
		w := m.width
		if w <= 0 {
			w = 100
		}
		room := w - lipgloss.Width(line) - 4
		if room > 8 {
			line += "  " + sDim.Render(ansi.Truncate(m.upd.Label, room, "…"))
		}
	}
	return line + "\n"
}

var errInterrupted = fmt.Errorf("interrupted")

// Task runs fn while showing a spinner. fn may call report to update the
// label or progress; label-only updates are throttled. The returned summary
// (if any) is shown on the final ✔ line.
func Task(title string, fn func(report func(Update)) (string, error)) error {
	p := tea.NewProgram(taskModel{title: title, start: time.Now()})
	var mu sync.Mutex
	last := time.Time{}
	lastDone := -1
	report := func(u Update) {
		mu.Lock()
		defer mu.Unlock()
		if u.Done == lastDone && time.Since(last) < 60*time.Millisecond {
			return
		}
		last, lastDone = time.Now(), u.Done
		p.Send(u)
	}
	go func() {
		summary, err := fn(report)
		p.Send(taskDone{err: err, summary: summary})
	}()
	res, err := p.Run()
	if err != nil {
		return err
	}
	return res.(taskModel).err
}

// ─── Confirm dialog ───────────────────────────────────────────────────────

type confirmModel struct {
	question string
	detail   string
	danger   bool
	yes      bool
	answered bool
	frame    int
}

func (m confirmModel) Init() tea.Cmd { return taskTickCmd() }

func (m confirmModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case taskTick:
		m.frame++
		return m, taskTickCmd()
	case tea.KeyMsg:
		switch msg.String() {
		case "left", "right", "tab", "shift+tab", "h", "l":
			m.yes = !m.yes
		case "y", "Y":
			m.yes, m.answered = true, true
			return m, tea.Quit
		case "n", "N", "esc", "q", "ctrl+c":
			m.yes, m.answered = false, true
			return m, tea.Quit
		case "enter", " ":
			m.answered = true
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m confirmModel) View() string {
	if m.answered {
		if m.yes {
			return "  " + sOK.Render("✔ ") + sText.Render(m.question) + sDim.Render("  yes") + "\n"
		}
		return "  " + sDim.Render("✗ "+m.question+"  cancelled") + "\n"
	}
	stops := Neon
	icon := sPink.Render("◆")
	if m.danger {
		stops = Fire
		icon = sErr.Render("⚠")
	}
	btn := func(label string, active bool, color string) string {
		st := lipgloss.NewStyle().Padding(0, 2).Bold(true)
		if active {
			return st.Foreground(lipgloss.Color("#0E0F1A")).Background(lipgloss.Color(color)).Render(label)
		}
		return st.Foreground(cDim).Background(cKeyBg).Render(label)
	}
	yesColor := Mint
	if m.danger {
		yesColor = Red
	}
	body := icon + " " + sBold.Render(m.question)
	if m.detail != "" {
		body += "\n" + sDim.Render("  "+m.detail)
	}
	body += "\n\n  " + btn("Yes", m.yes, yesColor) + "   " + btn("No", !m.yes, Violet) +
		"     " + sDim.Render("y / n · ←→ · enter")
	w := clamp(lipgloss.Width(m.question)+10, 52, 90)
	return Panel(Shimmer("confirm", float64(m.frame)/30, stops...), body, w, 0, stops...) + "\n"
}

// Confirm shows a yes/no dialog. danger turns it red (permanent deletion).
func Confirm(question, detail string, def, danger bool) bool {
	res, err := tea.NewProgram(confirmModel{question: question, detail: detail, danger: danger, yes: def}).Run()
	if err != nil {
		return false
	}
	return res.(confirmModel).yes
}

// ─── Result card with count-up animation ──────────────────────────────────

// Result describes what a cleanup achieved.
type Result struct {
	Verb    string // "freed", "moved to Trash", "would be freed"
	Bytes   int64
	Items   int
	Failed  int
	Elapsed time.Duration
	Notes   []string
}

type resultModel struct {
	r     Result
	frame int
	stars []star
}

type star struct {
	x     int
	phase float64
}

const resultFrames = 34

func (m resultModel) Init() tea.Cmd {
	return tea.Tick(28*time.Millisecond, func(time.Time) tea.Msg { return taskTick{} })
}

func (m resultModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg.(type) {
	case taskTick:
		m.frame++
		if m.frame >= resultFrames {
			return m, tea.Quit
		}
		return m, m.Init()
	case tea.KeyMsg:
		m.frame = resultFrames
		return m, tea.Quit
	}
	return m, nil
}

func (m resultModel) View() string {
	t := math.Min(1, float64(m.frame)/float64(resultFrames-6))
	ease := 1 - math.Pow(1-t, 3)
	shown := int64(float64(m.r.Bytes) * ease)
	phase := float64(m.frame) / 40

	width := 58
	var sky strings.Builder
	row := []rune(strings.Repeat(" ", width-6))
	for _, s := range m.stars {
		v := math.Sin(float64(m.frame)/4 + s.phase)
		ch := "·"
		switch {
		case v > 0.6:
			ch = "✦"
		case v > 0.1:
			ch = "✧"
		case v > -0.4:
			ch = "⋆"
		}
		if s.x < len(row) {
			row[s.x] = []rune(ch)[0]
		}
	}
	sky.WriteString(Shimmer(string(row), phase, Neon...))

	big := BoldGradient(Size(shown), Neon...)
	head := "  " + big + "  " + sBold.Render(m.r.Verb)
	if m.r.Bytes == 0 && m.r.Items == 0 {
		head = "  " + sBold.Render("Nothing was removed")
	}
	parts := []string{fmt.Sprintf("%d items", m.r.Items)}
	if m.r.Failed > 0 {
		parts = append(parts, sWarn.Render(fmt.Sprintf("%d skipped", m.r.Failed)))
	}
	if m.r.Elapsed > 0 {
		parts = append(parts, fmt.Sprintf("%.1fs", m.r.Elapsed.Seconds()))
	}
	body := sky.String() + "\n" + head + "\n" + "  " + sDim.Render(strings.Join(parts, " · "))
	for _, n := range m.r.Notes {
		body += "\n  " + sDim.Render(n)
	}
	body += "\n" + sky.String()
	return Panel(Shimmer("✨ done", phase, Neon...), body, width, 0, Neon...) + "\n"
}

// ShowResult animates the result card, then leaves it on screen.
func ShowResult(r Result) {
	stars := make([]star, 9)
	for i := range stars {
		stars[i] = star{x: rand.Intn(52), phase: rand.Float64() * 6}
	}
	_, _ = tea.NewProgram(resultModel{r: r, stars: stars}).Run()
}

// ─── Static helpers ───────────────────────────────────────────────────────

// Header prints a gradient section title with a rule.
func Header(icon, title string) string {
	rule := Gradient(strings.Repeat("━", 46), Neon...)
	return "\n  " + icon + " " + BoldGradient(title, Neon...) + "\n  " + rule
}

// Card renders a key/value info box (used for the app being uninstalled).
func Card(title string, rows [][2]string) string {
	var b strings.Builder
	for _, r := range rows {
		if r[1] == "" {
			continue
		}
		b.WriteString(sDim.Render(padRight(r[0], 11)) + sText.Render(r[1]) + "\n")
	}
	w := 30
	for _, r := range rows {
		w = max(w, lipgloss.Width(r[1])+17)
	}
	return Panel(BoldGradient(title, Neon...), b.String(), min(w, 100), 0, Neon...)
}
