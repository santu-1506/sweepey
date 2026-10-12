package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/santu-1506/sweepey/internal/ai"
	"github.com/santu-1506/sweepey/internal/analyze"
	"github.com/santu-1506/sweepey/internal/apps"
	"github.com/santu-1506/sweepey/internal/clean"
	"github.com/santu-1506/sweepey/internal/fsutil"
	"github.com/santu-1506/sweepey/internal/installer"
	"github.com/santu-1506/sweepey/internal/purge"
	"github.com/santu-1506/sweepey/internal/status"
	"github.com/santu-1506/sweepey/internal/tui"
	"github.com/santu-1506/sweepey/internal/ui"
)

// aiAvailable is a cheap check for credentials, used to decide whether to
// offer AI actions at all. The request itself reports real auth errors.
func aiAvailable() bool {
	if os.Getenv("ANTHROPIC_API_KEY") != "" || os.Getenv("ANTHROPIC_AUTH_TOKEN") != "" {
		return true
	}
	return fsutil.Exists(filepath.Join(fsutil.Home(), ".config", "anthropic"))
}

func cmdAI(ctx context.Context, o opts) error {
	if len(o.Args) == 0 {
		return errors.New("usage: sweepr ai advise | sweepr ai explain <path>")
	}
	switch o.Args[0] {
	case "advise", "advice", "plan":
		return aiAdvise(ctx)
	case "explain", "what":
		if len(o.Args) < 2 {
			return errors.New("usage: sweepr ai explain <path>")
		}
		p, err := filepath.Abs(fsutil.Expand(strings.Join(o.Args[1:], " ")))
		if err != nil {
			return err
		}
		explainPath(ctx, p)
		return nil
	}
	return errors.New("usage: sweepr ai advise | sweepr ai explain <path>")
}

func describe(p string) (size int64, isDir bool, children []string, err error) {
	fi, err := os.Lstat(p)
	if err != nil {
		return 0, false, nil, err
	}
	size = fsutil.Size(p)
	if fi.IsDir() {
		if entries, err := analyze.Children(p); err == nil {
			for i, e := range entries {
				if i == 20 {
					break
				}
				children = append(children, e.Name+" ("+ui.Size(e.Size)+")")
			}
		}
	}
	return size, fi.IsDir(), children, nil
}

// explainText asks Claude about a path and formats the answer as lines.
func explainText(ctx context.Context, client *ai.Client, p string) (string, error) {
	size, isDir, children, err := describe(p)
	if err != nil {
		return "", err
	}
	ex, err := client.Explain(ctx, fsutil.Tilde(p), size, isDir, children)
	if err != nil {
		return "", err
	}
	safe := map[string]string{"yes": ui.Green("yes"), "no": ui.Red("no"), "depends": ui.Yellow("depends")}[ex.SafeToClean]
	return fmt.Sprintf("%s %s\n%s %s   %s %s\n%s\n%s %s",
		tui.BoldText("What"), ex.What,
		tui.BoldText("Owner"), ex.Owner, tui.BoldText("Safe to delete"), safe,
		tui.Dim(ex.Explanation),
		tui.BoldText("How:"), ex.HowToRemove), nil
}

func explainPath(ctx context.Context, p string) {
	client := ai.New()
	aiNotice(client)
	var text string
	err := task("Asking Claude about "+fsutil.Tilde(p), func(func(tui.Update)) (string, error) {
		var err error
		text, err = explainText(ctx, client, p)
		return "", err
	})
	if err != nil {
		if !fancy {
			ui.Fail("%v", err)
		}
		return
	}
	if fancy {
		fmt.Println(tui.Panel(tui.BoldGradient("✨ "+fsutil.Tilde(p), tui.Neon...), text, 96, 0, tui.Neon...))
		return
	}
	fmt.Println()
	for _, line := range strings.Split(text, "\n") {
		ui.Info("%s", line)
	}
}

// aiAdvise gathers a snapshot of the machine (sizes and names only) and asks
// Claude for a prioritized cleanup plan.
func aiAdvise(ctx context.Context) error {
	header("✨", "AI cleanup advisor")
	client := ai.New()
	aiNotice(client)

	type sized struct {
		Path string `json:"path"`
		Size int64  `json:"size_bytes"`
	}
	snap := map[string]any{}

	_ = task("Reading system status", func(func(tui.Update)) (string, error) {
		snap["status"] = status.Collect()
		return "", nil
	})
	_ = task("Measuring cleanable junk", func(report func(tui.Update)) (string, error) {
		var junk []map[string]any
		var total int64
		for _, g := range clean.ScanProgress(clean.Targets(), func(i, n int, name string) {
			report(tui.Update{Label: name, Done: i, Total: n})
		}) {
			if g.Size > 0 {
				total += g.Size
				junk = append(junk, map[string]any{"category": g.Target.Category, "name": g.Target.Name,
					"size_bytes": g.Size, "items": len(g.Items), "selected_by_default": g.Target.Default})
			}
		}
		snap["sweepr_clean_would_free"] = junk
		return ui.Size(total), nil
	})
	_ = task("Measuring your home folder", func(func(tui.Update)) (string, error) {
		entries, err := analyze.Children(fsutil.Home())
		if err != nil {
			return "", err
		}
		var top []sized
		for i, e := range entries {
			if i == 15 {
				break
			}
			top = append(top, sized{fsutil.Tilde(e.Path), e.Size})
		}
		snap["largest_in_home"] = top
		return "", nil
	})
	_ = task("Finding installers and build artifacts", func(report func(tui.Update)) (string, error) {
		files := installer.Scan()
		var instTotal int64
		var instTop []sized
		for i, f := range files {
			instTotal += f.Size
			if i < 5 {
				instTop = append(instTop, sized{fsutil.Tilde(f.Path), f.Size})
			}
		}
		snap["installer_files"] = map[string]any{"count": len(files), "total_bytes": instTotal, "largest": instTop}
		arts := purge.ScanProgress(purge.DefaultRoots(), 6, func(d string) { report(tui.Update{Label: fsutil.Tilde(d)}) })
		var artTotal int64
		var artTop []sized
		for i, a := range arts {
			artTotal += max(a.Size, 0)
			if i < 5 {
				artTop = append(artTop, sized{fsutil.Tilde(a.Path), a.Size})
			}
		}
		snap["project_build_artifacts"] = map[string]any{"count": len(arts), "total_bytes": artTotal, "largest": artTop}
		return "", nil
	})
	_ = task("Measuring installed apps", func(func(tui.Update)) (string, error) {
		all, err := apps.Discover()
		if err != nil {
			return "", err
		}
		list := apps.Removable(all)
		paths := make([]string, 0, len(list))
		for _, a := range list {
			paths = append(paths, a.Path)
		}
		if len(paths) > 0 && list[0].Size == 0 {
			for i, s := range fsutil.Sizes(paths) {
				list[i].Size = s
			}
		}
		sort.Slice(list, func(i, j int) bool { return list[i].Size > list[j].Size })
		var appTop []map[string]any
		for i, a := range list {
			if i == 25 {
				break
			}
			appTop = append(appTop, map[string]any{"name": a.Name, "size_bytes": a.Size})
		}
		snap["largest_apps"] = appTop
		snap["installed_app_count"] = len(list)
		return fmt.Sprintf("%d apps", len(list)), nil
	})

	var advice ai.Advice
	if err := task("Claude is building your plan", func(func(tui.Update)) (string, error) {
		var err error
		advice, err = client.Advise(ctx, snap)
		return "", err
	}); err != nil {
		return err
	}
	sort.SliceStable(advice.Recommendations, func(i, j int) bool {
		return advice.Recommendations[i].Priority < advice.Recommendations[j].Priority
	})

	if !fancy {
		ui.Title("Cleanup plan")
		ui.Info("%s", advice.Summary)
		for i, r := range advice.Recommendations {
			ui.Info("")
			ui.Info("%s %s  %s · %s risk", ui.Bold(strconv.Itoa(i+1)+"."), ui.Bold(r.Title), ui.Cyan(r.Savings), r.Risk)
			ui.Info("   %s", r.Detail)
			if r.Command != "" {
				ui.Info("   $ %s", r.Command)
			}
		}
		return nil
	}
	var b strings.Builder
	b.WriteString(tui.Dim(advice.Summary) + "\n")
	for i, r := range advice.Recommendations {
		b.WriteString("\n" + tui.BoldGradient(fmt.Sprintf("%d  %s", i+1, r.Title), tui.Neon...) + "   " +
			tui.BoldText(r.Savings) + "  " + tui.Risk(r.Risk) + "\n")
		b.WriteString("   " + r.Detail + "\n")
		if r.Command != "" {
			b.WriteString("   " + tui.Dim("$ ") + ui.Cyan(r.Command) + "\n")
		}
	}
	fmt.Println(tui.Panel(tui.BoldGradient("✨ your cleanup plan", tui.Neon...), b.String(), 100, 0, tui.Neon...))
	return nil
}
