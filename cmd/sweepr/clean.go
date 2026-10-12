package main

import (
	"errors"
	"fmt"
	"io/fs"
	"strings"

	"github.com/santu-1506/sweepey/internal/clean"
	"github.com/santu-1506/sweepey/internal/fsutil"
	"github.com/santu-1506/sweepey/internal/safety"
	"github.com/santu-1506/sweepey/internal/tui"
	"github.com/santu-1506/sweepey/internal/ui"
)

func cmdClean(o opts) error {
	header("🧹", "Clean")
	var groups []clean.Group
	targets := clean.Targets()
	_ = task("Scanning for junk", func(report func(tui.Update)) (string, error) {
		groups = clean.ScanProgress(targets, func(i, n int, name string) {
			report(tui.Update{Label: name, Done: i, Total: n})
		})
		var total int64
		for _, g := range groups {
			total += max(g.Size, 0)
		}
		return ui.Size(total) + " of junk found", nil
	})

	var items []tui.Item
	var picked []clean.Group
	for _, g := range groups {
		if g.Target.ID == "trash" && o.Trash {
			continue // moving the Trash into the Trash would be silly
		}
		if g.Size == 0 && g.Err == nil {
			continue
		}
		var sub []string
		if n := len(g.Items); n > 0 {
			sub = append(sub, itemsText(n))
		}
		if g.Target.Note != "" {
			sub = append(sub, g.Target.Note)
		}
		var detail []string
		if g.Err != nil {
			if errors.Is(g.Err, fs.ErrPermission) {
				sub = append(sub, "no access")
				detail = append(detail, "macOS blocks this folder: grant Full Disk Access to your terminal to include it.")
			} else {
				detail = append(detail, g.Err.Error())
			}
		}
		for i, it := range g.Items {
			if i == 3 {
				detail = append(detail, fmt.Sprintf("…and %d more", len(g.Items)-3))
				break
			}
			detail = append(detail, fsutil.Tilde(it.Path)+"  "+ui.Size(it.Size))
		}
		items = append(items, tui.Item{
			Label:    g.Target.Name,
			Sub:      strings.Join(sub, " · "),
			Badge:    g.Target.Category,
			Detail:   strings.Join(detail, "\n"),
			Size:     g.Size,
			Selected: (g.Target.Default || o.All) && (g.Size > 0 || g.Target.SpecialRun != nil),
		})
		picked = append(picked, g)
	}
	if len(items) == 0 {
		ui.OK("Nothing to clean, your machine is already tidy")
		return nil
	}
	if !pick(tui.PickOptions{Icon: "🧹", Title: "Clean", Action: "clean", Items: items,
		Subtitle: "Everything here is rebuilt automatically by the app that made it."}) {
		ui.Info("Cancelled")
		return nil
	}

	var removals []safety.Target
	var specials []clean.Group
	var total int64
	for i, it := range items {
		if !it.Selected {
			continue
		}
		g := picked[i]
		if g.Target.SpecialRun != nil {
			specials = append(specials, g)
			total += max(g.Size, 0)
			continue
		}
		for _, x := range g.Items {
			removals = append(removals, safety.Target{Path: x.Path, Size: x.Size})
			total += max(x.Size, 0)
		}
	}
	if len(removals)+len(specials) == 0 {
		ui.Info("Nothing selected")
		return nil
	}
	if !confirmRemoval(o, len(removals)+len(specials), total, o.Trash) {
		ui.Info("Cancelled")
		return nil
	}
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
	runRemoval("clean", removals, o, o.Trash)
	return nil
}
