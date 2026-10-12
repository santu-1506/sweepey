package main

import (
	"context"
	"errors"
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
	"github.com/santu-1506/sweepey/internal/ui"
)

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

func explainPath(ctx context.Context, p string) {
	fi, err := os.Lstat(p)
	if err != nil {
		ui.Fail("%v", err)
		return
	}
	client := ai.New()
	aiNotice(client)
	size := fsutil.Size(p)
	var children []string
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
	ui.Info("Asking Claude about %s…", fsutil.Tilde(p))
	ex, err := client.Explain(ctx, fsutil.Tilde(p), size, fi.IsDir(), children)
	if err != nil {
		ui.Fail("%v", err)
		return
	}
	safe := map[string]string{"yes": ui.Green("yes"), "no": ui.Red("no"), "depends": ui.Yellow("depends")}[ex.SafeToClean]
	ui.Title(fsutil.Tilde(p) + "  " + ui.Size(size))
	ui.Info("%s  %s", ui.Bold("What"), ex.What)
	ui.Info("%s  %s", ui.Bold("Owner"), ex.Owner)
	ui.Info("%s  %s", ui.Bold("Safe to delete"), safe)
	ui.Info("")
	ui.Info("%s", ex.Explanation)
	ui.Info("")
	ui.Info("%s %s", ui.Bold("How to remove:"), ex.HowToRemove)
}

// aiAdvise gathers a snapshot of the machine (sizes and names only) and asks
// Claude for a prioritized cleanup plan.
func aiAdvise(ctx context.Context) error {
	client := ai.New()
	ui.Title("Building a snapshot of this machine")
	aiNotice(client)

	type sized struct {
		Path string `json:"path"`
		Size int64  `json:"size_bytes"`
	}
	snap := map[string]any{"status": status.Collect()}

	ui.Info("Measuring cleanable junk…")
	var junk []map[string]any
	for _, g := range clean.Scan(clean.Targets()) {
		if g.Size > 0 {
			junk = append(junk, map[string]any{"category": g.Target.Category, "name": g.Target.Name,
				"size_bytes": g.Size, "items": len(g.Items), "selected_by_default": g.Target.Default})
		}
	}
	snap["sweepr_clean_would_free"] = junk

	ui.Info("Measuring your home folder…")
	if entries, err := analyze.Children(fsutil.Home()); err == nil {
		var top []sized
		for i, e := range entries {
			if i == 15 {
				break
			}
			top = append(top, sized{fsutil.Tilde(e.Path), e.Size})
		}
		snap["largest_in_home"] = top
	}

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

	ui.Info("Looking for project build artifacts…")
	arts := purge.Scan(purge.DefaultRoots(), 6)
	var artTotal int64
	var artTop []sized
	for i, a := range arts {
		artTotal += max(a.Size, 0)
		if i < 5 {
			artTop = append(artTop, sized{fsutil.Tilde(a.Path), a.Size})
		}
	}
	snap["project_build_artifacts"] = map[string]any{"count": len(arts), "total_bytes": artTotal, "largest": artTop}

	ui.Info("Measuring installed apps…")
	if all, err := apps.Discover(); err == nil {
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
	}

	ui.Info("Asking Claude for a plan…")
	advice, err := client.Advise(ctx, snap)
	if err != nil {
		return err
	}
	sort.SliceStable(advice.Recommendations, func(i, j int) bool {
		return advice.Recommendations[i].Priority < advice.Recommendations[j].Priority
	})
	ui.Title("Cleanup plan")
	ui.Info("%s", advice.Summary)
	for i, r := range advice.Recommendations {
		risk := map[string]string{"low": ui.Green("low risk"), "medium": ui.Yellow("medium risk"), "high": ui.Red("high risk")}[r.Risk]
		ui.Info("")
		ui.Info("%s %s  %s · %s", ui.Bold(strconv.Itoa(i+1)+"."), ui.Bold(r.Title), ui.Cyan(r.Savings), risk)
		ui.Info("   %s", r.Detail)
		if r.Command != "" {
			ui.Info("   %s", ui.Dim("$ ")+r.Command)
		}
	}
	return nil
}
