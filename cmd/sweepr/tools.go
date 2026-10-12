package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/santu-1506/sweepey/internal/ai"
	"github.com/santu-1506/sweepey/internal/analyze"
	"github.com/santu-1506/sweepey/internal/apps"
	"github.com/santu-1506/sweepey/internal/config"
	"github.com/santu-1506/sweepey/internal/fsutil"
	"github.com/santu-1506/sweepey/internal/installer"
	"github.com/santu-1506/sweepey/internal/purge"
	"github.com/santu-1506/sweepey/internal/safety"
	"github.com/santu-1506/sweepey/internal/status"
	"github.com/santu-1506/sweepey/internal/tui"
	"github.com/santu-1506/sweepey/internal/ui"
)

func cmdLeftovers(ctx context.Context, o opts) error {
	header("👻", "Leftovers")
	all, err := discoverApps()
	if err != nil {
		return err
	}
	var cands []apps.Candidate
	_ = task("Looking for data from deleted apps", func(func(tui.Update)) (string, error) {
		cands = apps.Orphans(all)
		var total int64
		for _, c := range cands {
			total += max(c.Size, 0)
		}
		return fmt.Sprintf("%s · %s", itemsText(len(cands)), ui.Size(total)), nil
	})
	if len(cands) == 0 {
		ui.OK("No orphaned app data found")
		return nil
	}
	subtitle := "Nothing is pre-selected. Tip: run with --ai to let Claude pick the safe ones."
	if o.AI {
		client := ai.New()
		aiNotice(client)
		var names []string
		for _, a := range all {
			names = append(names, a.Name)
		}
		reviewCandidates(ctx, cands, func(items []ai.Item) ([]ai.Verdict, error) {
			return client.ReviewOrphans(ctx, names, items)
		})
		subtitle = "✦ Claude pre-selected what it is confident is safe. Check the rest yourself."
	}
	return pickAndRemove("leftovers", "👻", "Leftovers from deleted apps", subtitle, cands, o)
}

func cmdPurge(o opts) error {
	header("📦", "Purge")
	roots := purge.DefaultRoots()
	if len(o.Args) > 0 {
		roots = nil
		for _, a := range o.Args {
			p, _ := filepath.Abs(fsutil.Expand(a))
			roots = append(roots, p)
		}
	}
	var arts []purge.Artifact
	_ = task("Scanning projects for build artifacts", func(report func(tui.Update)) (string, error) {
		arts = purge.ScanProgress(roots, 6, func(dir string) { report(tui.Update{Label: fsutil.Tilde(dir)}) })
		var total int64
		for _, a := range arts {
			total += max(a.Size, 0)
		}
		return fmt.Sprintf("%s · %s", itemsText(len(arts)), ui.Size(total)), nil
	})
	if len(arts) == 0 {
		ui.OK("No build artifacts found in %s", strings.Join(tildeAll(roots), ", "))
		ui.Info("%s", ui.Dim("Add your project folders to "+fsutil.Tilde(filepath.Join(config.Dir(), "purge_paths"))+" or pass them as arguments."))
		return nil
	}
	items := make([]tui.Item, len(arts))
	for i, a := range arts {
		badge, detail := "old", "Rebuilt by your package manager or build tool when needed."
		if a.Recent {
			badge, detail = "recent", "This project changed in the last 7 days, so it is not selected."
		}
		items[i] = tui.Item{Label: fsutil.Tilde(a.Project), Sub: a.Kind, Badge: badge, Size: a.Size, Selected: !a.Recent,
			Detail: fsutil.Tilde(a.Path) + "\n" + detail}
	}
	if !pick(tui.PickOptions{Icon: "📦", Title: "Purge build artifacts", Items: items, Action: "purge",
		Subtitle: "node_modules, target/, .venv… can all be rebuilt. Recently active projects are left unselected."}) {
		ui.Info("Cancelled")
		return nil
	}
	var targets []safety.Target
	var total int64
	for i, it := range items {
		if it.Selected {
			targets = append(targets, safety.Target{Path: arts[i].Path, Size: arts[i].Size})
			total += max(arts[i].Size, 0)
		}
	}
	if len(targets) == 0 {
		ui.Info("Nothing selected")
		return nil
	}
	if !confirmRemoval(o, len(targets), total, false) {
		ui.Info("Cancelled")
		return nil
	}
	runRemoval("purge", targets, o, false)
	return nil
}

func tildeAll(ps []string) []string {
	out := make([]string, len(ps))
	for i, p := range ps {
		out[i] = fsutil.Tilde(p)
	}
	return out
}

func cmdInstaller(o opts) error {
	header("💿", "Installers")
	var files []installer.File
	_ = task("Looking for installer files", func(func(tui.Update)) (string, error) {
		files = installer.Scan()
		var total int64
		for _, f := range files {
			total += f.Size
		}
		return fmt.Sprintf("%s · %s", itemsText(len(files)), ui.Size(total)), nil
	})
	if len(files) == 0 {
		ui.OK("No installer files found")
		return nil
	}
	week := time.Now().Add(-7 * 24 * time.Hour)
	items := make([]tui.Item, len(files))
	for i, f := range files {
		badge := "old"
		if !f.ModTime.Before(week) {
			badge = "recent"
		}
		items[i] = tui.Item{Label: fsutil.Tilde(f.Path), Sub: age(f.ModTime), Badge: badge, Size: f.Size,
			Selected: f.ModTime.Before(week), Detail: "Downloaded " + f.ModTime.Format("2 Jan 2006") + " · " + age(f.ModTime)}
	}
	if !pick(tui.PickOptions{Icon: "💿", Title: "Installer files", Items: items, Action: "remove",
		Subtitle: "Disk images and setup files you no longer need once the app is installed."}) {
		ui.Info("Cancelled")
		return nil
	}
	var targets []safety.Target
	var total int64
	for i, it := range items {
		if it.Selected {
			targets = append(targets, safety.Target{Path: files[i].Path, Size: files[i].Size})
			total += files[i].Size
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
	runRemoval("installer", targets, o, trash)
	return nil
}

func age(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < 48*time.Hour:
		return "downloaded recently"
	case d < 60*24*time.Hour:
		return fmt.Sprintf("%d days old", int(d.Hours()/24))
	case d < 2*365*24*time.Hour:
		return fmt.Sprintf("%d months old", int(d.Hours()/24/30))
	}
	return fmt.Sprintf("%d years old", int(d.Hours()/24/365))
}

func trashOne(o opts) func(string, int64) error {
	return func(path string, size int64) error {
		if err := safety.Validate(path); err != nil {
			return err
		}
		res := safety.Remover{DryRun: o.DryRun, Trash: true, Op: "analyze"}.Remove([]safety.Target{{Path: path, Size: size}})
		return res[0].Err
	}
}

func cmdAnalyze(ctx context.Context, o opts) error {
	dir := fsutil.Home()
	if len(o.Args) > 0 {
		dir, _ = filepath.Abs(fsutil.Expand(o.Args[0]))
	}
	if !fsutil.IsDir(dir) {
		return fmt.Errorf("%s is not a folder", dir)
	}
	if fancy {
		var explain func(string) (string, error)
		if aiAvailable() {
			client := ai.New()
			explain = func(p string) (string, error) { return explainText(ctx, client, p) }
		}
		return tui.Explore(tui.ExploreOptions{Start: dir, TrashName: trashName(), Trash: trashOne(o),
			CanTrash: safety.Validate, Reveal: analyze.Reveal, Explain: explain})
	}
	for {
		ui.Title("Measuring " + fsutil.Tilde(dir))
		entries, err := analyze.Children(dir)
		if err != nil {
			return err
		}
		var total int64
		for _, e := range entries {
			total += max(e.Size, 0)
		}
		ui.Info("Total %s in %d entries", ui.Bold(ui.Size(total)), len(entries))
		fmt.Println()
		n := min(o.Top, len(entries))
		for i := 0; i < n; i++ {
			e := entries[i]
			frac := 0.0
			if total > 0 && e.Size > 0 {
				frac = float64(e.Size) / float64(total)
			}
			name := e.Name
			if e.IsDir {
				name = ui.Bold(name + string(filepath.Separator))
			}
			fmt.Printf("  %3d. %9s %5.1f%% %-24s %s\n", i+1, ui.Size(e.Size), frac*100, ui.Cyan(strings.Repeat("█", int(frac*24+0.5))), name)
		}
		if !ui.Interactive() {
			return nil
		}
		in := ui.Ask("\n  " + ui.Dim("N=open folder · u=up · r N=reveal · t N=move to "+trashName()+" · e N=AI explain · q=quit:"))
		cmd, arg, _ := strings.Cut(in, " ")
		if arg == "" {
			arg = cmd
		}
		pickN := func() (analyze.Entry, bool) {
			k, err := strconv.Atoi(strings.TrimSpace(arg))
			if err != nil || k < 1 || k > n {
				ui.Warn("Pick a number between 1 and %d", n)
				return analyze.Entry{}, false
			}
			return entries[k-1], true
		}
		switch cmd {
		case "q", "":
			return nil
		case "u", "..":
			dir = filepath.Dir(dir)
		case "r":
			if e, ok := pickN(); ok {
				_ = analyze.Reveal(e.Path)
			}
		case "t":
			if e, ok := pickN(); ok && ui.Confirm(fmt.Sprintf("Move %s (%s) to the %s?", e.Name, ui.Size(e.Size), trashName()), false) {
				if err := trashOne(o)(e.Path, e.Size); err != nil {
					ui.Fail("%v", err)
				} else {
					ui.OK("Moved %s to the %s", e.Name, trashName())
				}
			}
		case "e":
			if e, ok := pickN(); ok {
				explainPath(ctx, e.Path)
			}
		default:
			if e, ok := pickN(); ok {
				if e.IsDir {
					dir = e.Path
				} else {
					ui.Warn("%s is a file", e.Name)
				}
			}
		}
	}
}

func cmdStatus(o opts) error {
	if o.JSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(status.Collect())
	}
	info := status.Collect()
	if fancy {
		return tui.Dashboard(info.OSVersion)
	}
	ui.Title("System status")
	ui.Info("%-8s %s (%s)", "System", info.OSVersion, info.Arch)
	ui.Info("%-8s %s", "Host", info.Host)
	ui.Info("%-8s %d cores", "CPU", info.CPUs)
	if info.MemTotal > 0 {
		used := info.MemTotal - min(info.MemAvail, info.MemTotal)
		ui.Info("%-8s %s  %s used of %s", "Memory", meter(used, info.MemTotal), ui.Size(int64(used)), ui.Size(int64(info.MemTotal)))
	}
	for _, d := range info.Disks {
		used := d.Total - d.Free
		ui.Info("%-8s %s  %s free of %s  %s", "Disk", meter(used, d.Total), ui.Bold(ui.Size(int64(d.Free))), ui.Size(int64(d.Total)), ui.Dim(d.Mount))
		if d.Total > 0 && float64(d.Free)/float64(d.Total) < 0.1 {
			ui.Warn("Less than 10%% free on %s: try `sweepr clean` and `sweepr ai advise`", d.Mount)
		}
	}
	return nil
}

func meter(used, total uint64) string {
	if total == 0 {
		return ""
	}
	frac := float64(used) / float64(total)
	filled := int(frac*20 + 0.5)
	bar := strings.Repeat("█", filled) + strings.Repeat("░", 20-filled)
	switch {
	case frac > 0.9:
		bar = ui.Red(bar)
	case frac > 0.75:
		bar = ui.Yellow(bar)
	default:
		bar = ui.Green(bar)
	}
	return fmt.Sprintf("%s %3.0f%%", bar, frac*100)
}

func cmdHistory(o opts) error {
	entries, err := safety.ReadLog()
	if err != nil {
		return err
	}
	if o.JSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(entries)
	}
	header("📜", "History")
	if len(entries) == 0 {
		ui.Info("Nothing removed yet")
		return nil
	}
	var freed int64
	for _, e := range entries {
		if e.OK {
			freed += e.Size
		}
	}
	start := max(0, len(entries)-40)
	for _, e := range entries[start:] {
		mark := ui.Green("✓")
		if !e.OK {
			mark = ui.Red("✗")
		}
		fmt.Printf("  %s %s %-10s %-8s %9s  %s\n", mark, ui.Dim(e.Time.Local().Format("2006-01-02 15:04")), e.Op, e.Mode,
			ui.Size(e.Size), fsutil.Tilde(e.Path))
	}
	total := ui.Bold(ui.Size(freed))
	if fancy {
		total = tui.BoldGradient(ui.Size(freed), tui.Neon...)
	}
	fmt.Printf("\n  %d operations · %s freed in total · log: %s\n", len(entries), total, fsutil.Tilde(safety.LogPath()))
	return nil
}

func cmdWhitelist(o opts) error {
	file := filepath.Join(config.Dir(), "whitelist")
	if len(o.Args) >= 2 && o.Args[0] == "add" {
		p := fsutil.Expand(o.Args[1])
		if !strings.ContainsAny(p, "*?[") {
			p, _ = filepath.Abs(p)
		}
		if err := config.AddWhitelist(fsutil.Tilde(p)); err != nil {
			return err
		}
		ui.OK("sweepr will never touch %s", fsutil.Tilde(p))
		return nil
	}
	header("🛡", "Whitelist")
	list := config.Whitelist()
	if len(list) == 0 {
		ui.Info("Empty. Add with: sweepr whitelist add <path or glob>")
	}
	for _, p := range list {
		ui.Info("%s", fsutil.Tilde(p))
	}
	ui.Info("%s", ui.Dim("File: "+fsutil.Tilde(file)))
	return nil
}
