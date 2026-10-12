// Package clean finds rebuildable junk: caches, logs, temp files, browser and
// developer caches. Targets are declared per OS in targets_*.go.
package clean

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/santu-1506/sweepey/internal/config"
	"github.com/santu-1506/sweepey/internal/fsutil"
	"github.com/santu-1506/sweepey/internal/safety"
)

// Target describes one kind of junk.
type Target struct {
	ID       string
	Category string
	Name     string
	Globs    []string      // paths, glob patterns allowed
	Contents bool          // remove what is inside each match, not the match itself
	MinAge   time.Duration // leave entries modified more recently than this
	Skip     []string      // entry-name prefixes to leave alone
	Default  bool          // selected without asking
	Admin    bool          // usually needs elevated rights
	Note     string

	// Special targets (e.g. the Windows Recycle Bin) are not plain files.
	SpecialSize func() int64
	SpecialRun  func() error
}

type Item struct {
	Path string
	Size int64
}

type Group struct {
	Target Target
	Items  []Item
	Size   int64
	Err    error
}

// Scan expands every target. Earlier targets claim their paths first, so a
// specific target (Chrome cache) wins over a general one (all app caches)
// and nothing is counted twice.
func Scan(targets []Target) []Group { return ScanProgress(targets, nil) }

// ScanProgress is Scan with a callback before each target is measured.
func ScanProgress(targets []Target, progress func(done, total int, name string)) []Group {
	wl := config.Whitelist()
	var claimed []string
	var groups []Group
	for i, t := range targets {
		if progress != nil {
			progress(i, len(targets), t.Name)
		}
		g := Group{Target: t}
		if t.SpecialSize != nil {
			g.Size = t.SpecialSize()
			groups = append(groups, g)
			continue
		}
		var paths []string
		for _, pattern := range t.Globs {
			matches, _ := filepath.Glob(pattern)
			for _, m := range matches {
				if !t.Contents {
					paths = append(paths, m)
					continue
				}
				entries, err := os.ReadDir(m)
				if err != nil {
					if g.Err == nil && !os.IsNotExist(err) {
						g.Err = err
					}
					continue
				}
				for _, e := range entries {
					if skipName(e.Name(), t.Skip) {
						continue
					}
					if t.MinAge > 0 {
						if fi, err := e.Info(); err != nil || time.Since(fi.ModTime()) < t.MinAge {
							continue
						}
					}
					paths = append(paths, filepath.Join(m, e.Name()))
				}
			}
		}
		var kept []string
		for _, p := range paths {
			if config.Whitelisted(p, wl) || overlaps(p, claimed) || safety.Validate(p) != nil {
				continue
			}
			kept = append(kept, p)
		}
		sizes := fsutil.Sizes(kept)
		for i, p := range kept {
			g.Items = append(g.Items, Item{Path: p, Size: sizes[i]})
			g.Size += max(sizes[i], 0)
		}
		sort.Slice(g.Items, func(i, j int) bool { return g.Items[i].Size > g.Items[j].Size })
		claimed = append(claimed, kept...)
		groups = append(groups, g)
	}
	return groups
}

func skipName(name string, prefixes []string) bool {
	for _, p := range prefixes {
		if strings.HasPrefix(name, p) {
			return true
		}
	}
	return name == ".DS_Store" || name == "desktop.ini"
}

func overlaps(p string, claimed []string) bool {
	for _, c := range claimed {
		if fsutil.SamePath(p, c) || fsutil.HasPathPrefix(p, c) || fsutil.HasPathPrefix(c, p) {
			return true
		}
	}
	return false
}

// Targets returns the cleanup targets for the running OS.
func Targets() []Target { return platformTargets() }

// both builds glob patterns for the cartesian product of dirs and leaf names.
func both(dirs []string, leaves ...string) []string {
	var out []string
	for _, d := range dirs {
		for _, l := range leaves {
			out = append(out, filepath.Join(d, l))
		}
	}
	return out
}
