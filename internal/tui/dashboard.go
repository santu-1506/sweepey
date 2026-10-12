package tui

import (
	"fmt"
	"math"
	"os"
	"runtime"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type sampleMsg Sample
type animTick struct{}

func animCmd() tea.Cmd {
	return tea.Tick(120*time.Millisecond, func(time.Time) tea.Msg { return animTick{} })
}

func sampleCmd(s *Sampler, wait time.Duration) tea.Cmd {
	return tea.Tick(wait, func(time.Time) tea.Msg { return sampleMsg(s.Sample()) })
}

type dashModel struct {
	s      *Sampler
	cur    Sample
	have   bool
	w, h   int
	frame  int
	byMem  bool
	osName string
	host   string
}

func (m dashModel) Init() tea.Cmd {
	return tea.Batch(sampleCmd(m.s, 300*time.Millisecond), animCmd())
}

func (m dashModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
	case sampleMsg:
		m.cur, m.have = Sample(msg), true
		return m, sampleCmd(m.s, time.Second)
	case animTick:
		m.frame++
		return m, animCmd()
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "esc", "ctrl+c":
			return m, tea.Quit
		case "s", "m":
			m.byMem = !m.byMem
		}
	}
	return m, nil
}

// AreaChart draws values as a filled chart `height` rows tall using 1/8
// blocks, colored top-to-bottom along the stops.
func AreaChart(vals []float64, maxV float64, width, height int, stops ...string) string {
	if len(vals) > width {
		vals = vals[len(vals)-width:]
	}
	pad := width - len(vals)
	rows := make([]string, height)
	for r := 0; r < height; r++ {
		var b strings.Builder
		b.WriteString(strings.Repeat(" ", pad))
		color := fg(colorAt(stops, float64(r)/math.Max(1, float64(height-1))))
		for _, v := range vals {
			level := 0.0
			if maxV > 0 {
				level = v / maxV * float64(height*8)
			}
			fromBottom := float64(height-1-r) * 8
			cell := level - fromBottom
			switch {
			case cell >= 8:
				b.WriteString(color.Render("█"))
			case cell > 0:
				b.WriteString(color.Render(string(sparkRunes[int(cell)])))
			default:
				b.WriteString(sFaint.Render(map[bool]string{true: "·", false: " "}[r == height-1]))
			}
		}
		rows[r] = b.String()
	}
	return strings.Join(rows, "\n")
}

func rate(bps float64) string { return Size(int64(bps)) + "/s" }

func maxOf(v []float64) float64 {
	m := 0.0
	for _, x := range v {
		m = math.Max(m, x)
	}
	return m
}

func dur(d time.Duration) string {
	days := int(d.Hours()) / 24
	h := int(d.Hours()) % 24
	mins := int(d.Minutes()) % 60
	if days > 0 {
		return fmt.Sprintf("%dd %dh", days, h)
	}
	return fmt.Sprintf("%dh %dm", h, mins)
}

func (m dashModel) View() string {
	w := max(64, m.w)
	phase := float64(m.frame) / 50
	s := m.cur
	var b strings.Builder

	// Header
	score, label, color := Health(s)
	left := "  ⚡ " + BoldGradient("sweepr status", Neon...) + "  " +
		sDim.Render(fmt.Sprintf("%s · %s · %s", m.host, m.osName, runtime.GOARCH))
	if s.Uptime > 0 {
		left += sDim.Render(" · up " + dur(s.Uptime))
	}
	health := sDim.Render("health ") + fg(color).Bold(true).Render(fmt.Sprintf("%d %s", score, label)) + " " +
		Bar(float64(score)/100, 10, Red, Yellow, Mint)
	if !m.have {
		health = sDim.Render("sampling…")
	}
	gap := w - lipgloss.Width(left) - lipgloss.Width(health) - 2
	b.WriteString("\n" + left + strings.Repeat(" ", max(1, gap)) + health + "\n")
	b.WriteString("  " + Shimmer(strings.Repeat("━", w-4), phase, Neon...) + "\n")

	wide := w >= 100
	colW := w - 2
	if wide {
		colW = (w - 3) / 2
	}
	chartW := colW - 4

	// CPU panel
	cpuBody := AreaChart(s.CPUHist, 100, chartW, 4, Pink, Violet, Blue) + "\n"
	var cores strings.Builder
	for _, c := range s.Cores {
		lvl := int(math.Round(c / 100 * 7))
		cores.WriteString(fg(colorAt([]string{Mint, Yellow, Red}, c/100)).Render(string(sparkRunes[clamp(lvl, 0, 7)])))
	}
	loadTxt := ""
	if s.Load[0] > 0 {
		loadTxt = sDim.Render(fmt.Sprintf("   load %.2f %.2f %.2f", s.Load[0], s.Load[1], s.Load[2]))
	}
	cpuBody += sDim.Render("cores ") + cores.String() + loadTxt
	cpuTitle := sBold.Render("CPU ") + BoldGradient(fmt.Sprintf("%.0f%%", s.CPU), Pink, Violet)
	cpuP := Panel(cpuTitle, cpuBody, colW, 5, Pink, Violet, Blue)

	// Memory panel
	memBody := AreaChart(s.MemHist, 100, chartW, 3, Violet, Blue, Cyan) + "\n"
	memBody += Meter(s.MemPct/100, max(10, chartW-26)) + "  " +
		sBold.Render(Size(int64(s.MemUsed))) + sDim.Render(" / "+Size(int64(s.MemTotal))) + "\n"
	if s.SwapTotal > 0 {
		memBody += sDim.Render("swap  ") + Meter(float64(s.SwapUsed)/float64(s.SwapTotal), 12) + " " +
			sDim.Render(Size(int64(s.SwapUsed))+" / "+Size(int64(s.SwapTotal)))
	} else {
		memBody += sDim.Render("swap  none in use")
	}
	memTitle := sBold.Render("Memory ") + BoldGradient(fmt.Sprintf("%.0f%%", s.MemPct), Violet, Cyan)
	memP := Panel(memTitle, memBody, colW, 5, Violet, Blue, Cyan)

	// Disk panel
	var diskBody strings.Builder
	for _, d := range s.Disks {
		used := float64(d.Total-d.Free) / math.Max(1, float64(d.Total))
		diskBody.WriteString(padRight(sBold.Render(d.Mount), 6) + " " + Meter(used, max(10, chartW-34)) + " " +
			fmt.Sprintf("%3.0f%%  ", used*100) + sBold.Render(Size(int64(d.Free))) + sDim.Render(" free of "+Size(int64(d.Total))) + "\n")
	}
	diskP := Panel(sBold.Render("Disks"), diskBody.String(), colW, 3, Blue, Cyan, Mint)

	// Network panel
	rxMax := math.Max(maxOf(s.RxHist), 1024)
	txMax := math.Max(maxOf(s.TxHist), 1024)
	sw := max(8, chartW-18)
	netBody := sCyan.Render("↓ ") + padRight(sBold.Render(rate(s.RxRate)), 13) + Sparkline(s.RxHist, rxMax, sw, Blue, Cyan) + "\n" +
		fg(Pink).Render("↑ ") + padRight(sBold.Render(rate(s.TxRate)), 13) + Sparkline(s.TxHist, txMax, sw, Violet, Pink) + "\n" +
		sDim.Render(fmt.Sprintf("peak ↓ %s  ↑ %s", rate(rxMax), rate(txMax)))
	netP := Panel(sBold.Render("Network"), netBody, colW, 3, Mint, Cyan, Blue)

	if wide {
		b.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, " ", cpuP, " ", memP) + "\n")
		b.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, " ", diskP, " ", netP) + "\n")
	} else {
		b.WriteString(" " + strings.ReplaceAll(cpuP, "\n", "\n ") + "\n")
		b.WriteString(" " + strings.ReplaceAll(memP, "\n", "\n ") + "\n")
		b.WriteString(" " + strings.ReplaceAll(diskP, "\n", "\n ") + "\n")
	}

	// Processes
	used := strings.Count(b.String(), "\n")
	rows := clamp(m.h-used-5, 3, 14)
	procs := append([]Proc(nil), s.Procs...)
	if m.byMem {
		for i := 1; i < len(procs); i++ {
			for j := i; j > 0 && procs[j].RSS > procs[j-1].RSS; j-- {
				procs[j], procs[j-1] = procs[j-1], procs[j]
			}
		}
	}
	nameW := max(12, w-60)
	var pb strings.Builder
	pb.WriteString(sDim.Render(fmt.Sprintf("%7s  %-*s %6s  %-16s %9s", "PID", nameW, "NAME", "CPU%", "", "MEMORY")) + "\n")
	for i := 0; i < rows && i < len(procs); i++ {
		p := procs[i]
		name := p.Name
		if len([]rune(name)) > nameW {
			name = string([]rune(name)[:nameW-1]) + "…"
		}
		pb.WriteString(fmt.Sprintf("%7d  ", p.PID) + sText.Render(padRight(name, nameW)) + " " +
			fg(colorAt([]string{Mint, Yellow, Red}, math.Min(1, p.CPU/100))).Bold(true).Render(fmt.Sprintf("%6.1f", p.CPU)) +
			"  " + Bar(math.Min(1, p.CPU/100), 16, Mint, Yellow, Red) + " " + padLeft(sDim.Render(Size(int64(p.RSS))), 9) + "\n")
	}
	if len(procs) == 0 {
		pb.WriteString(sDim.Render("  measuring…"))
	}
	sortLbl := "cpu"
	if m.byMem {
		sortLbl = "memory"
	}
	procTitle := sBold.Render("Processes ") + sDim.Render(fmt.Sprintf("%d running · by %s", s.ProcCount, sortLbl))
	b.WriteString(" " + strings.ReplaceAll(Panel(procTitle, pb.String(), w-2, rows+1, Calm...), "\n", "\n ") + "\n")
	b.WriteString(" " + Keys("s", "sort cpu/memory", "q", "quit") + sDim.Render("   live · refreshes every second"))
	return b.String()
}

// Dashboard runs the live status screen until the user quits.
func Dashboard(osName string) error {
	host, _ := os.Hostname()
	m := dashModel{s: NewSampler(true), osName: osName, host: host, w: 100, h: 34}
	_, err := tea.NewProgram(m, tea.WithAltScreen()).Run()
	return err
}
