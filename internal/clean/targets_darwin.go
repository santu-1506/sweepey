//go:build darwin

package clean

import (
	"os"
	"path/filepath"
	"time"

	"github.com/santu-1506/sweepey/internal/fsutil"
)

func platformTargets() []Target {
	h := fsutil.Home()
	lib := filepath.Join(h, "Library")
	caches := filepath.Join(lib, "Caches")
	support := filepath.Join(lib, "Application Support")
	chromium := func(base string) []string {
		return both([]string{filepath.Join(base, "*")}, "Cache", "Code Cache", "GPUCache", "Service Worker/CacheStorage")
	}

	return []Target{
		// Browsers
		{ID: "chrome", Category: "Browsers", Name: "Google Chrome cache", Default: true,
			Globs: append([]string{filepath.Join(caches, "Google/Chrome/*")}, chromium(filepath.Join(support, "Google/Chrome"))...)},
		{ID: "edge", Category: "Browsers", Name: "Microsoft Edge cache", Default: true,
			Globs: append([]string{filepath.Join(caches, "Microsoft Edge/*")}, chromium(filepath.Join(support, "Microsoft Edge"))...)},
		{ID: "brave", Category: "Browsers", Name: "Brave cache", Default: true,
			Globs: append([]string{filepath.Join(caches, "BraveSoftware/Brave-Browser/*")}, chromium(filepath.Join(support, "BraveSoftware/Brave-Browser"))...)},
		{ID: "arc", Category: "Browsers", Name: "Arc cache", Default: true, Globs: []string{filepath.Join(caches, "Arc/*")}},
		{ID: "firefox", Category: "Browsers", Name: "Firefox cache", Default: true,
			Globs: []string{filepath.Join(caches, "Firefox/Profiles/*/cache2")}},
		{ID: "safari", Category: "Browsers", Name: "Safari cache", Default: true,
			Globs: []string{filepath.Join(caches, "com.apple.Safari")}, Note: "needs Full Disk Access"},

		// Developer tools
		{ID: "xcode-derived", Category: "Developer", Name: "Xcode DerivedData", Default: true, Contents: true,
			Globs: []string{filepath.Join(lib, "Developer/Xcode/DerivedData")}},
		{ID: "xcode-devicesupport", Category: "Developer", Name: "Xcode device support files", Contents: true,
			Globs: []string{filepath.Join(lib, "Developer/Xcode/*DeviceSupport")}, Note: "re-downloaded when a device connects"},
		{ID: "simulator-caches", Category: "Developer", Name: "iOS Simulator caches", Default: true, Contents: true,
			Globs: []string{filepath.Join(lib, "Developer/CoreSimulator/Caches")}},
		{ID: "npm", Category: "Developer", Name: "npm cache", Default: true, Globs: []string{filepath.Join(h, ".npm/_cacache")}},
		{ID: "yarn", Category: "Developer", Name: "Yarn cache", Default: true, Globs: []string{filepath.Join(caches, "Yarn")}},
		{ID: "pnpm", Category: "Developer", Name: "pnpm store", Default: true, Globs: []string{filepath.Join(lib, "pnpm/store")}},
		{ID: "bun", Category: "Developer", Name: "Bun cache", Default: true, Globs: []string{filepath.Join(h, ".bun/install/cache")}},
		{ID: "pip", Category: "Developer", Name: "pip cache", Default: true, Globs: []string{filepath.Join(caches, "pip")}},
		{ID: "go-build", Category: "Developer", Name: "Go build cache", Default: true, Globs: []string{filepath.Join(caches, "go-build")}},
		{ID: "homebrew", Category: "Developer", Name: "Homebrew downloads", Default: true, Globs: []string{filepath.Join(caches, "Homebrew")}},
		{ID: "cocoapods", Category: "Developer", Name: "CocoaPods cache", Default: true, Globs: []string{filepath.Join(caches, "CocoaPods")}},
		{ID: "gradle", Category: "Developer", Name: "Gradle caches", Default: true, Globs: []string{filepath.Join(h, ".gradle/caches")}},
		{ID: "cargo", Category: "Developer", Name: "Cargo registry cache", Default: true, Globs: []string{filepath.Join(h, ".cargo/registry/cache")}},
		{ID: "jetbrains", Category: "Developer", Name: "JetBrains IDE caches", Default: true, Globs: []string{filepath.Join(caches, "JetBrains")}},
		{ID: "electron", Category: "Developer", Name: "Electron app caches (VS Code, Slack, Discord…)", Default: true,
			Globs: both([]string{filepath.Join(support, "Code"), filepath.Join(support, "Cursor"), filepath.Join(support, "Slack"),
				filepath.Join(support, "discord"), filepath.Join(support, "Microsoft Teams"), filepath.Join(support, "Notion")},
				"Cache", "CachedData", "Code Cache", "GPUCache", "CachedExtensionVSIXs")},

		// System
		{ID: "logs", Category: "System", Name: "User logs and crash reports", Default: true, Contents: true,
			Globs: []string{filepath.Join(lib, "Logs")}},
		{ID: "tmp", Category: "System", Name: "Temporary files older than 2 days", Default: true, Contents: true,
			MinAge: 48 * time.Hour, Globs: []string{os.TempDir()}, Skip: []string{"com.apple."}},
		{ID: "saved-state", Category: "System", Name: "Saved window state", Contents: true,
			Globs: []string{filepath.Join(lib, "Saved Application State")}, Note: "apps forget open windows"},
		{ID: "trash", Category: "System", Name: "Trash", Default: true, Contents: true,
			Globs: []string{filepath.Join(h, ".Trash")}, Note: "permanently empties the Trash"},

		// Must stay last: it picks up whatever the specific targets above did not claim.
		{ID: "app-caches", Category: "Apps", Name: "Other app caches", Default: true, Contents: true,
			Globs: []string{caches}, Skip: []string{"com.apple.", "CloudKit", "familycircled", "FamilyCircle"},
			Note: "apps rebuild these on next launch"},
	}
}
