// Package ui holds the small terminal toolkit sweepr uses: colors, size
// formatting, confirmations and the numbered multi-select list.
package ui

import (
	"bufio"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"

	"golang.org/x/term"
)

var (
	colorOn = detectColor()
	reader  = bufio.NewReader(os.Stdin)

	// AssumeYes answers every confirmation with yes (set by --yes).
	AssumeYes bool
)

func detectColor() bool {
	if os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb" {
		return false
	}
	return isTerminal(os.Stdout)
}

func isTerminal(f *os.File) bool { return term.IsTerminal(int(f.Fd())) }

// Interactive reports whether stdin is a terminal we can prompt on.
func Interactive() bool { return isTerminal(os.Stdin) }

func paint(code, s string) string {
	if !colorOn {
		return s
	}
	return "\x1b[" + code + "m" + s + "\x1b[0m"
}

func Bold(s string) string   { return paint("1", s) }
func Dim(s string) string    { return paint("2", s) }
func Red(s string) string    { return paint("31", s) }
func Green(s string) string  { return paint("32", s) }
func Yellow(s string) string { return paint("33", s) }
func Blue(s string) string   { return paint("34", s) }
func Cyan(s string) string   { return paint("36", s) }

func Title(s string) {
	fmt.Println()
	fmt.Println(Bold(Cyan("▸ " + s)))
}

func Info(format string, a ...any) { fmt.Printf("  "+format+"\n", a...) }
func OK(format string, a ...any)   { fmt.Printf("  %s %s\n", Green("✓"), fmt.Sprintf(format, a...)) }
func Warn(format string, a ...any) { fmt.Printf("  %s %s\n", Yellow("!"), fmt.Sprintf(format, a...)) }
func Fail(format string, a ...any) { fmt.Printf("  %s %s\n", Red("✗"), fmt.Sprintf(format, a...)) }

// Size formats bytes the way the host OS does: Finder counts in powers of
// 1000, Explorer in powers of 1024. Negative means unknown.
func Size(n int64) string {
	if n < 0 {
		return "?"
	}
	base := 1024.0
	if runtime.GOOS == "darwin" {
		base = 1000.0
	}
	if float64(n) < base {
		return fmt.Sprintf("%d B", n)
	}
	v := float64(n)
	for _, unit := range []string{"KB", "MB", "GB", "TB", "PB"} {
		v /= base
		if v < base {
			if v < 10 {
				return fmt.Sprintf("%.1f %s", v, unit)
			}
			return fmt.Sprintf("%.0f %s", v, unit)
		}
	}
	return fmt.Sprintf("%.0f PB", v)
}

func ReadLine() string {
	s, _ := reader.ReadString('\n')
	return strings.TrimSpace(s)
}

func Ask(q string) string {
	fmt.Print(q + " ")
	return ReadLine()
}

// Confirm asks a yes/no question. Without a terminal (and without --yes) the
// answer is always no, so nothing destructive happens in pipelines by accident.
func Confirm(q string, def bool) bool {
	if AssumeYes {
		return true
	}
	if !Interactive() {
		return false
	}
	hint := "[y/N]"
	if def {
		hint = "[Y/n]"
	}
	fmt.Printf("  %s %s ", q, Dim(hint))
	switch strings.ToLower(ReadLine()) {
	case "":
		return def
	case "y", "yes":
		return true
	default:
		return false
	}
}

// Row is one line of a selectable list.
type Row struct {
	Label    string
	Note     string
	Size     int64
	Selected bool
}

// Choose prints rows and lets the user toggle them until they press enter.
// It returns false if the user cancelled. Without a terminal the defaults are
// kept as-is.
func Choose(rows []Row) bool {
	if len(rows) == 0 {
		return true
	}
	for {
		printRows(rows)
		if AssumeYes || !Interactive() {
			return true
		}
		in := Ask(fmt.Sprintf("\n  %s", Dim("Toggle numbers (e.g. 1 4 7-9), a=all, n=none, enter=continue, q=cancel:")))
		switch strings.ToLower(in) {
		case "":
			return true
		case "q", "quit":
			return false
		case "a", "all":
			setAll(rows, true)
			continue
		case "n", "none":
			setAll(rows, false)
			continue
		}
		idx, err := ParseSelection(in, len(rows))
		if err != nil {
			Warn("%v", err)
			continue
		}
		for _, i := range idx {
			rows[i].Selected = !rows[i].Selected
		}
	}
}

func setAll(rows []Row, v bool) {
	for i := range rows {
		rows[i].Selected = v
	}
}

func printRows(rows []Row) {
	fmt.Println()
	width := len(strconv.Itoa(len(rows)))
	var total int64
	count := 0
	for i, r := range rows {
		box := Dim("[ ]")
		if r.Selected {
			box = Green("[x]")
			count++
			if r.Size > 0 {
				total += r.Size
			}
		}
		size := ""
		if r.Size != 0 {
			size = fmt.Sprintf("%9s", Size(r.Size))
		} else {
			size = strings.Repeat(" ", 9)
		}
		line := fmt.Sprintf("  %s %*d. %s  %s", box, width, i+1, Bold(size), r.Label)
		if r.Note != "" {
			line += "  " + Dim(r.Note)
		}
		fmt.Println(line)
	}
	fmt.Printf("\n  %s selected, %s\n", Bold(strconv.Itoa(count)), Bold(Size(total)))
}

// ParseSelection turns "1 3, 5-7" into zero-based indexes, validated against n.
func ParseSelection(in string, n int) ([]int, error) {
	var out []int
	fields := strings.FieldsFunc(in, func(r rune) bool { return r == ' ' || r == ',' })
	for _, f := range fields {
		lo, hi := f, f
		if a, b, ok := strings.Cut(f, "-"); ok {
			lo, hi = a, b
		}
		l, err1 := strconv.Atoi(lo)
		h, err2 := strconv.Atoi(hi)
		if err1 != nil || err2 != nil {
			return nil, fmt.Errorf("not a number or range: %q", f)
		}
		if l > h {
			l, h = h, l
		}
		if l < 1 || h > n {
			return nil, fmt.Errorf("%q is outside 1-%d", f, n)
		}
		for i := l; i <= h; i++ {
			out = append(out, i-1)
		}
	}
	return out, nil
}
