//go:build !darwin && !windows

package clean

import (
	"os"
	"path/filepath"
	"time"

	"github.com/santu-1506/sweepey/internal/fsutil"
)

func platformTargets() []Target {
	h := fsutil.Home()
	cache := filepath.Join(h, ".cache")
	return []Target{
		{ID: "npm", Category: "Developer", Name: "npm cache", Default: true, Globs: []string{filepath.Join(h, ".npm/_cacache")}},
		{ID: "gradle", Category: "Developer", Name: "Gradle caches", Default: true, Globs: []string{filepath.Join(h, ".gradle/caches")}},
		{ID: "cargo", Category: "Developer", Name: "Cargo registry cache", Default: true, Globs: []string{filepath.Join(h, ".cargo/registry/cache")}},
		{ID: "tmp", Category: "System", Name: "Temporary files older than 2 days", Default: true, Contents: true,
			MinAge: 48 * time.Hour, Globs: []string{os.TempDir()}},
		{ID: "trash", Category: "System", Name: "Trash", Default: true, Contents: true,
			Globs: []string{filepath.Join(h, ".local/share/Trash/files"), filepath.Join(h, ".local/share/Trash/info")}},
		{ID: "user-cache", Category: "Apps", Name: "User caches (~/.cache)", Default: true, Contents: true, Globs: []string{cache}},
	}
}
