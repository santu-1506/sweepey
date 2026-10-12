//go:build darwin

package apps

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/santu-1506/sweepey/internal/fsutil"
)

func discover() ([]App, error) {
	h := fsutil.Home()
	dirs := []string{"/Applications", "/Applications/Utilities", filepath.Join(h, "Applications"),
		"/System/Applications", "/System/Applications/Utilities"}
	var bundles []string
	for _, d := range dirs {
		entries, err := os.ReadDir(d)
		if err != nil {
			continue
		}
		for _, e := range entries {
			p := filepath.Join(d, e.Name())
			if strings.HasSuffix(e.Name(), ".app") {
				bundles = append(bundles, p)
				continue
			}
			// Suites install into a folder: /Applications/Adobe Photoshop 2025/Adobe Photoshop 2025.app
			if e.IsDir() && d == "/Applications" && e.Name() != "Utilities" {
				inner, _ := filepath.Glob(filepath.Join(p, "*.app"))
				bundles = append(bundles, inner...)
			}
		}
	}

	out := make([]App, len(bundles))
	sem := make(chan struct{}, runtime.NumCPU())
	var wg sync.WaitGroup
	for i, b := range bundles {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, b string) {
			defer wg.Done()
			out[i] = readBundle(b)
			<-sem
		}(i, b)
	}
	wg.Wait()
	return out, nil
}

func readBundle(path string) App {
	info := readPlist(filepath.Join(path, "Contents", "Info.plist"))
	a := App{
		Name:       strings.TrimSuffix(filepath.Base(path), ".app"),
		BundleName: info["CFBundleName"],
		BundleID:   info["CFBundleIdentifier"],
		Executable: info["CFBundleExecutable"],
		Version:    info["CFBundleShortVersionString"],
		Path:       path,
	}
	a.System = strings.HasPrefix(path, "/System/") || a.BundleID == "com.apple.Safari"
	return a
}

var plistKeys = []string{"CFBundleIdentifier", "CFBundleName", "CFBundleExecutable", "CFBundleShortVersionString"}

// readPlist converts Info.plist to JSON with plutil. Plists holding <date> or
// <data> values cannot become JSON, so fall back to extracting key by key.
func readPlist(file string) map[string]string {
	out := map[string]string{}
	if data, err := exec.Command("plutil", "-convert", "json", "-o", "-", file).Output(); err == nil {
		var raw map[string]any
		if json.Unmarshal(data, &raw) == nil {
			for _, k := range plistKeys {
				if s, ok := raw[k].(string); ok {
					out[k] = s
				}
			}
			return out
		}
	}
	for _, k := range plistKeys {
		if v, err := exec.Command("plutil", "-extract", k, "raw", "-o", "-", file).Output(); err == nil {
			out[k] = strings.TrimSpace(string(v))
		}
	}
	return out
}

func searchRoots() []root {
	h := fsutil.Home()
	lib := filepath.Join(h, "Library")
	var roots []root
	for _, d := range []string{"Application Support", "Caches", "Preferences", "Preferences/ByHost",
		"Saved Application State", "Containers", "Group Containers", "Logs", "HTTPStorages", "WebKit",
		"Cookies", "Application Scripts", "Internet Plug-Ins", "PreferencePanes", "QuickLook", "Services",
		"Spotlight", "Screen Savers", "Application Support/CrashReporter"} {
		roots = append(roots, root{Dir: filepath.Join(lib, d)})
	}
	roots = append(roots,
		root{Dir: filepath.Join(lib, "LaunchAgents"), ReadContents: true},
		root{Dir: filepath.Join(lib, "Logs/DiagnosticReports"), CrashLogs: true},
	)
	for _, d := range []string{"Application Support", "Caches", "Preferences", "Logs", "Internet Plug-Ins",
		"PreferencePanes", "QuickLook", "Screen Savers", "Services", "Audio/Plug-Ins/Components",
		"Audio/Plug-Ins/VST", "Audio/Plug-Ins/VST3", "Audio/Plug-Ins/HAL", "PrivilegedHelperTools"} {
		roots = append(roots, root{Dir: filepath.Join("/Library", d)})
	}
	roots = append(roots,
		root{Dir: "/Library/LaunchAgents", ReadContents: true},
		root{Dir: "/Library/LaunchDaemons", ReadContents: true},
		root{Dir: os.TempDir()},
		root{Dir: filepath.Join(filepath.Dir(filepath.Clean(os.TempDir())), "C")},
		root{Dir: h, DotOnly: true, MaxConf: Medium},
		root{Dir: filepath.Join(h, ".config"), MaxConf: Medium},
	)
	return roots
}

func platformExtras(App, *matcher, []named) []Candidate { return nil }

func orphanRoots() []root {
	lib := filepath.Join(fsutil.Home(), "Library")
	var roots []root
	for _, d := range []string{"Application Support", "Caches", "Preferences", "Containers", "Group Containers",
		"Saved Application State", "HTTPStorages", "WebKit", "Logs", "Application Scripts", "Cookies", "LaunchAgents"} {
		roots = append(roots, root{Dir: filepath.Join(lib, d)})
	}
	return roots
}

// orphanCheck only considers reverse-DNS names (com.vendor.app): plain
// folder names are too ambiguous to call orphaned without more context.
func orphanCheck(n string, _ bool, ms []named) (Confidence, string) {
	if !looksLikeBundleID(n) || strings.HasPrefix(n, "com.apple.") || strings.HasPrefix(n, "apple.") ||
		strings.HasPrefix(n, "systemgroup.") {
		return None, ""
	}
	if ownedByInstalled(n, ms) {
		return None, ""
	}
	parts := strings.SplitN(n, ".", 3)
	vendor := parts[0] + "." + parts[1] + "."
	for _, x := range ms {
		if x.m.bundle != "" && strings.HasPrefix(x.m.bundle, vendor) {
			return Low, "same vendor as installed " + x.name
		}
	}
	return Medium, "no installed app uses this ID"
}

// IsRunning reports whether any process runs from inside the bundle.
func IsRunning(a App) bool {
	if a.Path == "" {
		return false
	}
	return exec.Command("pgrep", "-f", a.Path+"/Contents/MacOS/").Run() == nil
}

// Quit asks the app to quit normally, so it can save its state.
func Quit(a App) error {
	if a.BundleID != "" {
		return exec.Command("osascript", "-e", `tell application id "`+a.BundleID+`" to quit`).Run()
	}
	return exec.Command("osascript", "-e", `tell application "`+a.Name+`" to quit`).Run()
}

// RunUninstaller is a no-op on macOS: apps are removed by deleting the bundle.
func RunUninstaller(App, bool) error { return nil }

// Receipts lists installer package receipts that belong to the app.
func Receipts(a App) []string {
	out, err := exec.Command("pkgutil", "--pkgs").Output()
	if err != nil {
		return nil
	}
	m := newMatcher(a, nil)
	var found []string
	for _, line := range strings.Split(string(out), "\n") {
		id := strings.TrimSpace(line)
		if id == "" || strings.HasPrefix(id, "com.apple.") {
			continue
		}
		n := strings.ToLower(id)
		if (m.bundle != "" && strings.HasPrefix(n, m.bundle)) || func() bool { c, _ := m.match(n); return c == High }() {
			found = append(found, id)
		}
	}
	return found
}
