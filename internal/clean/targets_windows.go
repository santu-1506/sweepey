//go:build windows

package clean

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/santu-1506/sweepey/internal/fsutil"
	"github.com/santu-1506/sweepey/internal/safety"
)

func platformTargets() []Target {
	h := fsutil.Home()
	local := os.Getenv("LOCALAPPDATA")
	roam := os.Getenv("APPDATA")
	pd := os.Getenv("ProgramData")
	win := os.Getenv("SystemRoot")
	if win == "" {
		win = `C:\Windows`
	}
	chromium := func(userData string) []string {
		return both([]string{filepath.Join(userData, "*")}, "Cache", "Code Cache", "GPUCache", `Service Worker\CacheStorage`)
	}

	return []Target{
		// Browsers
		{ID: "chrome", Category: "Browsers", Name: "Google Chrome cache", Default: true,
			Globs: chromium(filepath.Join(local, `Google\Chrome\User Data`))},
		{ID: "edge", Category: "Browsers", Name: "Microsoft Edge cache", Default: true,
			Globs: chromium(filepath.Join(local, `Microsoft\Edge\User Data`))},
		{ID: "brave", Category: "Browsers", Name: "Brave cache", Default: true,
			Globs: chromium(filepath.Join(local, `BraveSoftware\Brave-Browser\User Data`))},
		{ID: "firefox", Category: "Browsers", Name: "Firefox cache", Default: true,
			Globs: []string{filepath.Join(local, `Mozilla\Firefox\Profiles\*\cache2`)}},

		// Developer tools
		{ID: "npm", Category: "Developer", Name: "npm cache", Default: true, Globs: []string{filepath.Join(local, `npm-cache\_cacache`)}},
		{ID: "yarn", Category: "Developer", Name: "Yarn cache", Default: true, Globs: []string{filepath.Join(local, `Yarn\Cache`)}},
		{ID: "pnpm", Category: "Developer", Name: "pnpm store", Default: true, Globs: []string{filepath.Join(local, `pnpm\store`)}},
		{ID: "pip", Category: "Developer", Name: "pip cache", Default: true, Globs: []string{filepath.Join(local, `pip\Cache`)}},
		{ID: "go-build", Category: "Developer", Name: "Go build cache", Default: true, Globs: []string{filepath.Join(local, "go-build")}},
		{ID: "nuget", Category: "Developer", Name: "NuGet HTTP cache", Default: true, Globs: []string{filepath.Join(local, `NuGet\v3-cache`)}},
		{ID: "gradle", Category: "Developer", Name: "Gradle caches", Default: true, Globs: []string{filepath.Join(h, `.gradle\caches`)}},
		{ID: "cargo", Category: "Developer", Name: "Cargo registry cache", Default: true, Globs: []string{filepath.Join(h, `.cargo\registry\cache`)}},
		{ID: "electron", Category: "Developer", Name: "Electron app caches (VS Code, Slack, Discord…)", Default: true,
			Globs: both([]string{filepath.Join(roam, "Code"), filepath.Join(roam, "Cursor"), filepath.Join(roam, "Slack"),
				filepath.Join(roam, "discord"), filepath.Join(roam, "Notion")},
				"Cache", "CachedData", "Code Cache", "GPUCache", "CachedExtensionVSIXs")},

		// System
		{ID: "tmp", Category: "System", Name: "Temp files older than 1 day", Default: true, Contents: true,
			MinAge: 24 * time.Hour, Globs: []string{os.TempDir()}},
		{ID: "crashdumps", Category: "System", Name: "Crash dumps", Default: true, Contents: true,
			Globs: []string{filepath.Join(local, "CrashDumps")}},
		{ID: "wer", Category: "System", Name: "Windows Error Reporting files", Default: true, Contents: true,
			Globs: []string{filepath.Join(local, `Microsoft\Windows\WER\ReportArchive`), filepath.Join(local, `Microsoft\Windows\WER\ReportQueue`),
				filepath.Join(pd, `Microsoft\Windows\WER\ReportArchive`), filepath.Join(pd, `Microsoft\Windows\WER\ReportQueue`)}},
		{ID: "inetcache", Category: "System", Name: "Internet cache", Default: true, Contents: true,
			Globs: []string{filepath.Join(local, `Microsoft\Windows\INetCache`)}},
		{ID: "shader", Category: "System", Name: "GPU shader caches", Default: true, Contents: true,
			Globs: []string{filepath.Join(local, "D3DSCache"), filepath.Join(local, `NVIDIA\DXCache`), filepath.Join(local, `NVIDIA\GLCache`),
				filepath.Join(local, `AMD\DxCache`)}},
		{ID: "win-temp", Category: "System", Name: "Windows temp folder", Contents: true, Admin: true,
			MinAge: 24 * time.Hour, Globs: []string{filepath.Join(win, "Temp")}, Note: "needs Administrator"},
		{ID: "win-update", Category: "System", Name: "Windows Update downloads", Contents: true, Admin: true,
			Globs: []string{filepath.Join(win, `SoftwareDistribution\Download`)}, Note: "needs Administrator"},
		{ID: "recycle-bin", Category: "System", Name: "Recycle Bin", Default: true, Note: "permanently empties it",
			SpecialSize: recycleBinSize, SpecialRun: emptyRecycleBin},
	}
}

func recycleBinSize() int64 {
	out, err := safety.PowerShell(`$s = 0; (New-Object -ComObject Shell.Application).Namespace(10).Items() | ForEach-Object { $s += [int64]$_.ExtendedProperty('Size') }; $s`)
	if err != nil {
		return -1
	}
	n, err := strconv.ParseInt(strings.TrimSpace(out), 10, 64)
	if err != nil {
		return -1
	}
	return n
}

func emptyRecycleBin() error {
	_, err := safety.PowerShell(`Clear-RecycleBin -Force -ErrorAction SilentlyContinue`)
	return err
}
