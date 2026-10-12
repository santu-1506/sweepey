package main

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"time"

	"github.com/santu-1506/sweepey/internal/ai"
	"github.com/santu-1506/sweepey/internal/apps"
	"github.com/santu-1506/sweepey/internal/fsutil"
	"github.com/santu-1506/sweepey/internal/safety"
	"github.com/santu-1506/sweepey/internal/tui"
	"github.com/santu-1506/sweepey/internal/ui"
)

func discoverApps() ([]apps.App, error) {
	var all []apps.App
	err := task("Discovering installed apps", func(func(tui.Update)) (string, error) {
		var err error
		all, err = apps.Discover()
		return fmt.Sprintf("%d apps", len(apps.Removable(all))), err
	})
	return all, err
}

func cmdUninstall(ctx context.Context, o opts) error {
	header("🚮", "Uninstall")
	all, err := discoverApps()
	if err != nil {
		return err
	}
	removable := apps.Removable(all)
	if o.List {
		for _, a := range removable {
			id := a.BundleID
			if id == "" {
				id = a.Vendor
			}
			fmt.Printf("  %-40s %-14s %s\n", a.Name, a.Version, ui.Dim(id))
		}
		ui.Info("%d apps", len(removable))
		return nil
	}

	app, installed, err := pickApp(removable, strings.Join(o.Args, " "))
	if err != nil {
		return err
	}
	return uninstallApp(ctx, app, installed, all, o)
}

func appItems(list []apps.App) []tui.Item {
	items := make([]tui.Item, len(list))
	for i, a := range list {
		detail := []string{}
		if a.BundleID != "" {
			detail = append(detail, a.BundleID)
		}
		if a.Vendor != "" {
			detail = append(detail, a.Vendor)
		}
		if a.Path != "" {
			detail = append(detail, fsutil.Tilde(a.Path))
		}
		items[i] = tui.Item{Label: a.Name, Sub: a.Version, Detail: strings.Join(detail, "  ·  "), Size: a.Size}
	}
	return items
}

func pickApp(list []apps.App, query string) (apps.App, bool, error) {
	choose := func(cands []apps.App, filter bool) (apps.App, bool, error) {
		i := pickOne(tui.PickOptions{Icon: "🚮", Title: "Choose an app to uninstall", Items: appItems(cands), Filter: filter,
			Subtitle: "Type to search · sweepr finds every file the app left behind"})
		if i < 0 {
			return apps.App{}, false, errors.New("cancelled")
		}
		return cands[i], true, nil
	}
	if query == "" {
		if !ui.Interactive() {
			return apps.App{}, false, errors.New("name the app: sweepr uninstall \"App Name\"")
		}
		if fancy {
			return choose(list, true)
		}
		query = ui.Ask("  App to uninstall (name or part of it; enter to list all):")
		if query == "" {
			return choose(list, false)
		}
	}
	matches := apps.Find(list, query)
	switch len(matches) {
	case 0:
		ui.Warn("No installed app matches %q", query)
		if confirm(fmt.Sprintf("Search for files left behind by an already-deleted app called %q?", query),
			"Useful when you already dragged the app to the Trash.", true, false) {
			return apps.App{Name: query}, false, nil
		}
		return apps.App{}, false, errors.New("nothing to uninstall")
	case 1:
		return matches[0], true, nil
	}
	if !ui.Interactive() {
		names := make([]string, len(matches))
		for i, m := range matches {
			names[i] = m.Name
		}
		return apps.App{}, false, fmt.Errorf("%q matches several apps: %s", query, strings.Join(names, ", "))
	}
	return choose(matches, false)
}

func uninstallApp(ctx context.Context, app apps.App, installed bool, all []apps.App, o opts) error {
	info := [][2]string{{"version", app.Version}, {"bundle ID", app.BundleID}, {"publisher", app.Vendor},
		{"location", fsutil.Tilde(app.Path)}}
	if !installed {
		info = append(info, [2]string{"status", "not installed: searching leftovers by name"})
	}
	if fancy {
		fmt.Println(tui.Card("🚮 "+app.Name, info))
	} else {
		ui.Title("Uninstalling " + app.Name)
		for _, r := range info {
			if r[1] != "" {
				ui.Info("%-10s %s", r[0], r[1])
			}
		}
	}

	if installed && apps.IsRunning(app) {
		ui.Warn("%s is running", app.Name)
		if !o.DryRun && confirm("Quit "+app.Name+" now?", "It will be asked to quit normally so it can save its work.", true, false) {
			_ = apps.Quit(app)
			for i := 0; i < 10 && apps.IsRunning(app); i++ {
				time.Sleep(500 * time.Millisecond)
			}
			if apps.IsRunning(app) {
				ui.Warn("%s is still running; some files may be recreated after removal", app.Name)
			}
		}
	}

	// On Windows the vendor's uninstaller removes services, drivers and its
	// registry entries properly; sweepr then hunts down what it left.
	if runtime.GOOS == "windows" && installed && app.UninstallCmd != "" {
		if o.DryRun {
			ui.Info("Would run the app's uninstaller: %s", ui.Dim(app.UninstallCmd))
		} else if confirm("Run "+app.Name+"'s own uninstaller first?", "Recommended: it removes services and drivers cleanly.", true, false) {
			if err := apps.RunUninstaller(app, o.Quiet); err != nil {
				ui.Warn("Uninstaller reported: %v", err)
			} else {
				ui.OK("Uninstaller finished")
			}
		}
	}

	var client *ai.Client
	var terms []string
	if o.AI {
		client = ai.New()
		aiNotice(client)
		var suggested []ai.Term
		_ = task("Asking Claude where "+app.Name+" hides its files", func(func(tui.Update)) (string, error) {
			var err error
			suggested, err = client.SuggestTerms(ctx, appInfo(app, installed, all))
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("%d extra names to search for", len(suggested)), nil
		})
		for _, t := range suggested {
			terms = append(terms, t.Term)
			fmt.Println(tui.Bullet(t.Term, t.Why))
		}
	}

	var cands []apps.Candidate
	_ = task("Hunting for leftovers", func(func(tui.Update)) (string, error) {
		cands = apps.Leftovers(app, all, terms)
		var total int64
		for _, c := range cands {
			total += max(c.Size, 0)
		}
		return fmt.Sprintf("%s · %s", itemsText(len(cands)), ui.Size(total)), nil
	})
	if len(cands) == 0 {
		ui.OK("No files left behind by %s", app.Name)
		return nil
	}
	if client != nil {
		reviewCandidates(ctx, cands, func(items []ai.Item) ([]ai.Verdict, error) {
			return client.ReviewLeftovers(ctx, appInfo(app, installed, all), items)
		})
	}

	if err := pickAndRemove("uninstall", "🚮", "Uninstall "+app.Name,
		"High-confidence matches are selected. Review medium ones before adding them.", cands, o); err != nil {
		return err
	}
	if receipts := apps.Receipts(app); len(receipts) > 0 && !o.DryRun {
		ui.Info("Installer receipts are still registered. To forget them:")
		for _, r := range receipts {
			ui.Info("  sudo pkgutil --forget %s", r)
		}
	}
	return nil
}

func candidateItems(cands []apps.Candidate) []tui.Item {
	items := make([]tui.Item, len(cands))
	for i, c := range cands {
		label := fsutil.Tilde(c.Path)
		badge := c.Confidence.String()
		if c.Kind == safety.Registry {
			label = "⌘ " + label
		}
		if c.SharedWith != "" {
			badge = "shared"
		}
		detail := []string{c.Confidence.String() + " confidence: " + c.Reason}
		if c.SharedWith != "" {
			detail = append(detail, "also matches "+c.SharedWith+", so it is not selected")
		}
		if c.AINote != "" {
			detail = append(detail, c.AINote)
		}
		items[i] = tui.Item{Label: label, Badge: badge, AI: c.AIVerdict, Detail: strings.Join(detail, "\n"),
			Size: c.Size, Selected: c.Selected}
	}
	return items
}

// pickAndRemove shows the candidates, lets the user adjust the selection and
// removes what is left selected (to the Trash unless --permanent).
func pickAndRemove(op, icon, title, subtitle string, cands []apps.Candidate, o opts) error {
	items := candidateItems(cands)
	if !pick(tui.PickOptions{Icon: icon, Title: title, Subtitle: subtitle, Items: items, Action: "remove"}) {
		ui.Info("Cancelled")
		return nil
	}
	var targets []safety.Target
	var total int64
	for i, it := range items {
		if it.Selected {
			targets = append(targets, safety.Target{Path: cands[i].Path, Kind: cands[i].Kind, Size: cands[i].Size})
			total += max(cands[i].Size, 0)
		}
	}
	if len(targets) == 0 {
		ui.Info("Nothing selected")
		return nil
	}
	trash := !o.Permanent
	if !confirmRemoval(o, len(targets), total, trash) {
		ui.Info("Cancelled")
		return nil
	}
	runRemoval(op, targets, o, trash)
	return nil
}

func appInfo(app apps.App, installed bool, all []apps.App) ai.AppInfo {
	info := ai.AppInfo{Name: app.Name, BundleID: app.BundleID, Executable: app.Executable, Vendor: app.Vendor,
		Version: app.Version, Installed: installed}
	vendor := ""
	if parts := strings.Split(app.BundleID, "."); len(parts) >= 3 {
		vendor = parts[0] + "." + parts[1] + "."
	}
	for _, o := range all {
		if o.Name == app.Name {
			continue
		}
		if (vendor != "" && strings.HasPrefix(o.BundleID, vendor)) || (app.Vendor != "" && o.Vendor == app.Vendor) {
			info.SameVendor = append(info.SameVendor, o.Name)
		}
	}
	return info
}

const aiBatch = 150

// reviewCandidates sends candidates to the AI in batches and applies its
// verdicts. A "remove" verdict only pre-selects an item when the model is
// confident and no other installed app matches it; "keep" always deselects.
func reviewCandidates(ctx context.Context, cands []apps.Candidate, review func([]ai.Item) ([]ai.Verdict, error)) {
	var items []ai.Item
	var index []int
	for i, c := range cands {
		if c.Primary {
			continue
		}
		kind := "file"
		if c.Kind == safety.Registry {
			kind = "registry key"
		} else if fsutil.IsDir(c.Path) {
			kind = "folder"
		}
		items = append(items, ai.Item{ID: len(items), Path: fsutil.Tilde(c.Path), Kind: kind, SizeBytes: c.Size,
			Heuristic: c.Confidence.String(), Reason: c.Reason, SharedWith: c.SharedWith})
		index = append(index, i)
	}
	if len(items) > 2*aiBatch {
		ui.Warn("AI reviews the first %d items; the rest keep heuristic defaults", 2*aiBatch)
		items, index = items[:2*aiBatch], index[:2*aiBatch]
	}
	for start := 0; start < len(items); start += aiBatch {
		if ctx.Err() != nil {
			return
		}
		end := min(start+aiBatch, len(items))
		var verdicts []ai.Verdict
		err := task(fmt.Sprintf("Claude is reviewing %s", itemsText(end-start)), func(func(tui.Update)) (string, error) {
			var err error
			verdicts, err = review(items[start:end])
			if err != nil {
				return "", err
			}
			counts := map[string]int{}
			for _, v := range verdicts {
				counts[v.Decision]++
			}
			return fmt.Sprintf("%d remove · %d keep · %d unsure", counts["remove"], counts["keep"], counts["unsure"]), nil
		})
		if err != nil {
			ui.Warn("AI review skipped: %v", err)
			return
		}
		for _, v := range verdicts {
			if v.ID < start || v.ID >= end {
				continue
			}
			c := &cands[index[v.ID]]
			c.AIVerdict = v.Decision
			c.AINote = fmt.Sprintf("✦ AI %s (%.0f%%): %s", v.Decision, v.Confidence*100, v.Reason)
			switch v.Decision {
			case "remove":
				if v.Confidence >= 0.75 && c.SharedWith == "" {
					c.Selected = true
				}
			case "keep":
				c.Selected = false
			}
		}
	}
}

var aiNoticeShown bool

func aiNotice(c *ai.Client) {
	if aiNoticeShown {
		return
	}
	aiNoticeShown = true
	ui.Info("%s", ui.Dim("✦ AI uses "+c.Model()+". Only paths, names and sizes are sent, never file contents."))
}
