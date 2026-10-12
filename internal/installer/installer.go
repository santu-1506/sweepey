// Package installer finds installer files that are no longer needed once
// the app is installed: .dmg, .pkg, .msi, setup.exe and friends.
package installer

import (
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/santu-1506/sweepey/internal/config"
	"github.com/santu-1506/sweepey/internal/fsutil"
)

type File struct {
	Path    string
	Size    int64
	ModTime time.Time
}

var exts = map[string]bool{
	".dmg": true, ".pkg": true, ".mpkg": true, ".xip": true, ".iso": true,
	".msi": true, ".msix": true, ".msixbundle": true, ".appx": true, ".appxbundle": true,
}

func isInstaller(name string) bool {
	lower := strings.ToLower(name)
	ext := filepath.Ext(lower)
	if exts[ext] {
		return true
	}
	if ext == ".exe" || ext == ".zip" {
		return strings.Contains(lower, "setup") || strings.Contains(lower, "install")
	}
	return false
}

// Scan looks in Downloads and Desktop (and two levels of Documents).
func Scan() []File {
	h := fsutil.Home()
	type loc struct {
		dir   string
		depth int
	}
	locs := []loc{{filepath.Join(h, "Downloads"), 3}, {filepath.Join(h, "Desktop"), 2}, {filepath.Join(h, "Documents"), 2}}
	wl := config.Whitelist()
	var out []File
	for _, l := range locs {
		base := strings.Count(l.dir, string(filepath.Separator))
		_ = filepath.WalkDir(l.dir, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() {
				if p != l.dir && (strings.Count(p, string(filepath.Separator))-base >= l.depth ||
					strings.HasSuffix(d.Name(), ".app")) {
					return fs.SkipDir
				}
				return nil
			}
			if !isInstaller(d.Name()) || config.Whitelisted(p, wl) {
				return nil
			}
			if fi, err := d.Info(); err == nil {
				out = append(out, File{Path: p, Size: fi.Size(), ModTime: fi.ModTime()})
			}
			return nil
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Size > out[j].Size })
	return out
}
