package main

import (
	"runtime"
	"strconv"

	"github.com/santu-1506/sweepey/internal/fsutil"
	"github.com/santu-1506/sweepey/internal/safety"
	"github.com/santu-1506/sweepey/internal/ui"
)

func trashName() string {
	if runtime.GOOS == "windows" {
		return "Recycle Bin"
	}
	return "Trash"
}

// confirmRemoval asks before anything is removed. Dry runs never ask.
func confirmRemoval(o opts, count int, total int64, trash bool) bool {
	if o.DryRun {
		return true
	}
	verb := "Permanently delete"
	if trash {
		verb = "Move to " + trashName() + ":"
	}
	return ui.Confirm(ui.Bold(verb)+" "+itemsText(count)+" ("+ui.Size(total)+")?", false)
}

func itemsText(n int) string {
	if n == 1 {
		return "1 item"
	}
	return strconv.Itoa(n) + " items"
}

// runRemoval removes targets through the safety layer, reports the outcome,
// and offers an elevated retry for root-owned leftovers.
func runRemoval(op string, targets []safety.Target, o opts, trash bool) {
	r := safety.Remover{DryRun: o.DryRun, Trash: trash, Op: op}
	results := r.Remove(targets)
	freed, ok, failed, admin := safety.Summary(results)

	shown := 0
	var adminPaths []string
	for _, res := range results {
		switch {
		case res.Err == nil:
		case res.NeedsAdmin:
			adminPaths = append(adminPaths, res.Path)
		default:
			if shown < 15 {
				ui.Fail("%s: %v", fsutil.Tilde(res.Path), res.Err)
			}
			shown++
		}
	}
	if shown > 15 {
		ui.Fail("…and %d more", shown-15)
	}

	if o.DryRun {
		ui.OK("Dry run: would free %s across %s. Nothing was changed.", ui.Bold(ui.Size(freed)), itemsText(ok))
		if admin+failed > 0 {
			ui.Warn("%d items would be refused or need administrator rights", admin+failed)
		}
		return
	}

	switch {
	case ok == 0 && failed+admin > 0:
		ui.Fail("Nothing was removed")
	case trash:
		ui.OK("Moved %s (%s) to the %s", itemsText(ok), ui.Bold(ui.Size(freed)), trashName())
	default:
		ui.OK("Freed %s (%s)", ui.Bold(ui.Size(freed)), itemsText(ok))
	}

	if len(adminPaths) > 0 {
		ui.Warn("%s need administrator rights:", itemsText(len(adminPaths)))
		for _, p := range adminPaths {
			ui.Info("  %s", fsutil.Tilde(p))
		}
		if runtime.GOOS == "windows" {
			ui.Info("Re-run sweepr from an Administrator terminal to remove them.")
			return
		}
		if ui.Interactive() && ui.Confirm("Remove them permanently with sudo?", false) {
			if err := safety.AdminRemove(adminPaths); err != nil {
				ui.Fail("%v", err)
			} else {
				ui.OK("Removed with administrator rights")
			}
		}
	}
}
