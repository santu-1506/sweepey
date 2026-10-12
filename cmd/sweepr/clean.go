package main

import (
	"errors"
	"fmt"
	"io/fs"

	"github.com/santu-1506/sweepey/internal/clean"
	"github.com/santu-1506/sweepey/internal/safety"
	"github.com/santu-1506/sweepey/internal/ui"
)

func cmdClean(o opts) error {
	ui.Title("Scanning for junk")
	groups := clean.Scan(clean.Targets())

	var rows []ui.Row
	var picked []clean.Group
	for _, g := range groups {
		if g.Target.ID == "trash" && o.Trash {
			continue // moving the Trash into the Trash would be silly
		}
		if g.Size == 0 && g.Err == nil {
			continue
		}
		note := g.Target.Note
		if n := len(g.Items); n > 0 {
			note = joinNote(note, itemsText(n))
		}
		if g.Err != nil {
			if errors.Is(g.Err, fs.ErrPermission) {
				note = joinNote(note, ui.Yellow("no access (grant Full Disk Access to your terminal)"))
			} else {
				note = joinNote(note, ui.Yellow(g.Err.Error()))
			}
		}
		rows = append(rows, ui.Row{
			Label:    fmt.Sprintf("%-10s %s", g.Target.Category, g.Target.Name),
			Note:     note,
			Size:     g.Size,
			Selected: (g.Target.Default || o.All) && (g.Size > 0 || g.Target.SpecialRun != nil),
		})
		picked = append(picked, g)
	}
	if len(rows) == 0 {
		ui.OK("Nothing to clean, your machine is already tidy")
		return nil
	}
	if !ui.Choose(rows) {
		ui.Info("Cancelled")
		return nil
	}

	var targets []safety.Target
	var specials []clean.Group
	var total int64
	for i, r := range rows {
		if !r.Selected {
			continue
		}
		g := picked[i]
		if g.Target.SpecialRun != nil {
			specials = append(specials, g)
			if g.Size > 0 {
				total += g.Size
			}
			continue
		}
		for _, it := range g.Items {
			targets = append(targets, safety.Target{Path: it.Path, Size: it.Size})
			total += max(it.Size, 0)
		}
	}
	if len(targets)+len(specials) == 0 {
		ui.Info("Nothing selected")
		return nil
	}
	if !confirmRemoval(o, len(targets)+len(specials), total, o.Trash) {
		ui.Info("Cancelled")
		return nil
	}
	runRemoval("clean", targets, o, o.Trash)
	for _, g := range specials {
		if o.DryRun {
			ui.OK("Dry run: would empty %s (%s)", g.Target.Name, ui.Size(g.Size))
			continue
		}
		if err := g.Target.SpecialRun(); err != nil {
			ui.Fail("%s: %v", g.Target.Name, err)
		} else {
			ui.OK("Emptied %s", g.Target.Name)
		}
	}
	return nil
}

func joinNote(a, b string) string {
	if a == "" {
		return b
	}
	return a + " · " + b
}
