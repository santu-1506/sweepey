package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/santu-1506/sweepey/internal/tui"
	"github.com/santu-1506/sweepey/internal/ui"
)

// fancy is true when sweepr drives a real terminal: full-screen pickers,
// animations and the live dashboard. Pipes, scripts, --yes and --plain get
// the plain line-based interface instead.
var fancy bool

func setupFront(o opts) {
	fancy = !o.Plain && os.Getenv("SWEEPR_PLAIN") == "" && !o.Yes && !o.JSON &&
		ui.Interactive() && ui.OutputTerminal()
}

func header(icon, title string) {
	if fancy {
		fmt.Println(tui.Header(icon, title))
		return
	}
	ui.Title(title)
}

// task runs fn with a spinner (fancy) or a plain status line.
func task(title string, fn func(report func(tui.Update)) (string, error)) error {
	if fancy {
		return tui.Task(title, fn)
	}
	ui.Info("%s…", title)
	summary, err := fn(func(tui.Update) {})
	if err == nil && summary != "" {
		ui.OK("%s", summary)
	}
	return err
}

// pick lets the user adjust which items are selected. It updates Selected in
// place and returns false if the user cancelled.
func pick(opt tui.PickOptions) bool {
	if fancy {
		return tui.Pick(opt)
	}
	rows := make([]ui.Row, len(opt.Items))
	for i, it := range opt.Items {
		label := it.Label
		if it.Sub != "" {
			label += "  " + ui.Dim(it.Sub)
		}
		var note []string
		if it.Badge != "" {
			note = append(note, it.Badge)
		}
		if d, _, _ := strings.Cut(it.Detail, "\n"); d != "" {
			note = append(note, d)
		}
		rows[i] = ui.Row{Label: label, Note: strings.Join(note, ": "), Size: it.Size, Selected: it.Selected}
	}
	if !ui.Choose(rows) {
		return false
	}
	for i := range rows {
		opt.Items[i].Selected = rows[i].Selected
	}
	return true
}

// pickOne returns the index of the chosen item or -1.
func pickOne(opt tui.PickOptions) int {
	if fancy {
		return tui.PickOne(opt)
	}
	fmt.Println()
	for i, it := range opt.Items {
		fmt.Printf("  %3d. %-40s %s\n", i+1, it.Label, ui.Dim(it.Sub))
	}
	for {
		in := ui.Ask("\n  Number (q to cancel):")
		if in == "q" || in == "" {
			return -1
		}
		if n, err := strconv.Atoi(in); err == nil && n >= 1 && n <= len(opt.Items) {
			return n - 1
		}
		ui.Warn("Enter a number between 1 and %d", len(opt.Items))
	}
}

// confirm asks a yes/no question; danger marks irreversible actions.
func confirm(question, detail string, def, danger bool) bool {
	if fancy {
		return tui.Confirm(question, detail, def, danger)
	}
	if detail != "" {
		ui.Info("%s", ui.Dim(detail))
	}
	return ui.Confirm(question, def)
}

// pause keeps a command's output on screen before the menu takes over again.
// It returns false when the user wants to quit.
func pause() bool {
	in := ui.Ask("\n  " + tui.Keys("enter", "back to menu", "q", "quit"))
	return strings.ToLower(strings.TrimSpace(in)) != "q"
}
