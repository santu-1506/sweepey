package main

import (
	"fmt"
	"runtime"
	"strconv"
	"time"

	"github.com/santu-1506/sweepey/internal/fsutil"
	"github.com/santu-1506/sweepey/internal/safety"
	"github.com/santu-1506/sweepey/internal/tui"
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
	if trash {
		return confirm(fmt.Sprintf("Move %s (%s) to the %s?", itemsText(count), ui.Size(total), trashName()),
			"You can restore them from the "+trashName()+".", true, false)
	}
	return confirm(fmt.Sprintf("Permanently delete %s (%s)?", itemsText(count), ui.Size(total)),
		"This cannot be undone.", false, true)
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
	start := time.Now()
	r := safety.Remover{DryRun: o.DryRun, Trash: trash, Op: op}
	var results []safety.Result
	work := func(report func(tui.Update)) (string, error) {
		const chunk = 25
		for i := 0; i < len(targets); i += chunk {
			end := min(i+chunk, len(targets))
			report(tui.Update{Label: fsutil.Tilde(targets[i].Path), Done: i, Total: len(targets)})
			results = append(results, r.Remove(targets[i:end])...)
		}
		return "", nil
	}
	if o.DryRun {
		_, _ = work(func(tui.Update) {})
	} else {
		title := "Removing"
		if trash {
			title = "Moving to the " + trashName()
		}
		_ = task(title, work)
	}
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

	verb := "freed"
	switch {
	case o.DryRun:
		verb = "would be freed (dry run)"
	case trash:
		verb = "moved to the " + trashName()
	}
	if fancy {
		var notes []string
		if o.DryRun {
			notes = append(notes, "Nothing was changed. Run again without --dry-run to apply.")
		}
		if len(adminPaths) > 0 {
			notes = append(notes, fmt.Sprintf("%d need administrator rights (see below)", len(adminPaths)))
		}
		tui.ShowResult(tui.Result{Verb: verb, Bytes: freed, Items: ok, Failed: failed + admin,
			Elapsed: time.Since(start), Notes: notes})
	} else if o.DryRun {
		ui.OK("Dry run: would free %s across %s. Nothing was changed.", ui.Bold(ui.Size(freed)), itemsText(ok))
		if admin+failed > 0 {
			ui.Warn("%d items would be refused or need administrator rights", admin+failed)
		}
	} else if ok == 0 && failed+admin > 0 {
		ui.Fail("Nothing was removed")
	} else {
		ui.OK("%s %s (%s)", ui.Bold(ui.Size(freed)), verb, itemsText(ok))
	}

	if len(adminPaths) == 0 || o.DryRun {
		return
	}
	ui.Warn("%s need administrator rights:", itemsText(len(adminPaths)))
	for _, p := range adminPaths {
		ui.Info("  %s", fsutil.Tilde(p))
	}
	if runtime.GOOS == "windows" {
		ui.Info("Re-run sweepr from an Administrator terminal to remove them.")
		return
	}
	if ui.Interactive() && confirm("Remove them permanently with sudo?", "You will be asked for your password.", false, true) {
		if err := safety.AdminRemove(adminPaths); err != nil {
			ui.Fail("%v", err)
		} else {
			ui.OK("Removed with administrator rights")
		}
	}
}
