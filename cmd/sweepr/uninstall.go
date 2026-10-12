package main

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/santu-1506/sweepey/internal/ai"
	"github.com/santu-1506/sweepey/internal/apps"
	"github.com/santu-1506/sweepey/internal/fsutil"
	"github.com/santu-1506/sweepey/internal/safety"
	"github.com/santu-1506/sweepey/internal/ui"
)

func cmdUninstall(ctx context.Context, o opts) error {
	ui.Title("Finding installed apps")
	all, err := apps.Discover()
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

func pickApp(list []apps.App, query string) (apps.App, bool, error) {
	if query == "" {
		if !ui.Interactive() {
			return apps.App{}, false, errors.New("name the app: sweepr uninstall \"App Name\"")
		}
		query = ui.Ask("  App to uninstall (name or part of it; enter to list all):")
		if query == "" {
			a, err := chooseOne(list)
			return a, true, err
		}
	}
	matches := apps.Find(list, query)
	switch len(matches) {
	case 0:
		ui.Warn("No installed app matches %q", query)
		if ui.Confirm(fmt.Sprintf("Search for files left behind by an already-deleted app called %q?", query), true) {
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
	a, err := chooseOne(matches)
	return a, true, err
}

func chooseOne(list []apps.App) (apps.App, error) {
	if len(list) == 0 {
		return apps.App{}, errors.New("no apps found")
	}
	fmt.Println()
	for i, a := range list {
		fmt.Printf("  %3d. %-40s %s\n", i+1, a.Name, ui.Dim(a.Version))
	}
	for {
		in := ui.Ask("\n  Number (q to cancel):")
		if in == "q" || in == "" {
			return apps.App{}, errors.New("cancelled")
		}
		n, err := strconv.Atoi(in)
		if err == nil && n >= 1 && n <= len(list) {
			return list[n-1], nil
		}
		ui.Warn("Enter a number between 1 and %d", len(list))
	}
}

func uninstallApp(ctx context.Context, app apps.App, installed bool, all []apps.App, o opts) error {
	ui.Title("Uninstalling " + app.Name)
	if app.Version != "" {
		ui.Info("Version   %s", app.Version)
	}
	if app.BundleID != "" {
		ui.Info("Bundle ID %s", app.BundleID)
	}
	if app.Vendor != "" {
		ui.Info("Publisher %s", app.Vendor)
	}
	if app.Path != "" {
		ui.Info("Location  %s", fsutil.Tilde(app.Path))
	}

	if installed && apps.IsRunning(app) {
		ui.Warn("%s is running", app.Name)
		if !o.DryRun && ui.Confirm("Quit it now?", true) {
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
		} else if ui.Confirm("Run "+app.Name+"'s own uninstaller first? (recommended)", true) {
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
		ui.Info("Asking Claude which other names %s uses on disk…", app.Name)
		ts, err := client.SuggestTerms(ctx, appInfo(app, installed, all))
		if err != nil {
			ui.Warn("AI suggestions skipped: %v", err)
		}
		for _, t := range ts {
			terms = append(terms, t.Term)
			ui.Info("  + %s %s", t.Term, ui.Dim("("+t.Why+")"))
		}
	}

	ui.Info("Searching for related files…")
	cands := apps.Leftovers(app, all, terms)
	if len(cands) == 0 {
		ui.OK("No files left behind by %s", app.Name)
		return nil
	}
	if client != nil {
		reviewCandidates(ctx, cands, func(items []ai.Item) ([]ai.Verdict, error) {
			ui.Info("Claude is reviewing %d items…", len(items))
			return client.ReviewLeftovers(ctx, appInfo(app, installed, all), items)
		})
	}

	if err := pickAndRemove("uninstall", cands, o); err != nil {
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

// pickAndRemove shows the candidates, lets the user adjust the selection and
// removes what is left selected (to the Trash unless --permanent).
func pickAndRemove(op string, cands []apps.Candidate, o opts) error {
	rows := make([]ui.Row, len(cands))
	for i, c := range cands {
		label := fsutil.Tilde(c.Path)
		if c.Kind == safety.Registry {
			label = ui.Blue("[registry] ") + label
		}
		note := c.Confidence.String() + ": " + c.Reason
		if c.SharedWith != "" {
			note += " · " + ui.Yellow("also matches "+c.SharedWith)
		}
		if c.AINote != "" {
			note += " · " + ui.Cyan(c.AINote)
		}
		rows[i] = ui.Row{Label: label, Note: note, Size: c.Size, Selected: c.Selected}
	}
	if !ui.Choose(rows) {
		ui.Info("Cancelled")
		return nil
	}
	var targets []safety.Target
	var total int64
	for i, r := range rows {
		if r.Selected {
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
		batch := items[start:end]
		verdicts, err := review(batch)
		if err != nil {
			ui.Warn("AI review skipped: %v", err)
			return
		}
		for _, v := range verdicts {
			if v.ID < start || v.ID >= end {
				continue
			}
			c := &cands[index[v.ID]]
			c.AINote = fmt.Sprintf("AI %s %.0f%%: %s", v.Decision, v.Confidence*100, v.Reason)
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
	ui.Info(ui.Dim("AI review uses " + c.Model() + ". Only paths, names and sizes are sent — never file contents."))
}
