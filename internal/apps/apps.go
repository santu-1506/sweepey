// Package apps discovers installed applications and finds every file (and,
// on Windows, registry key) an application left behind.
package apps

import (
	"sort"
	"strings"
)

// App is an installed application, normalized across platforms.
type App struct {
	Name       string // display name
	BundleName string // macOS CFBundleName, often the folder name apps use
	BundleID   string // macOS CFBundleIdentifier
	Executable string
	Vendor     string // Windows Publisher, or derived from the bundle ID
	Version    string
	Path       string // .app bundle (macOS) or install folder (Windows)
	Size       int64
	System     bool // ships with the OS; never offered for removal

	// Windows only
	UninstallCmd      string
	QuietUninstallCmd string
	RegistryKey       string
}

// Discover lists installed apps for the running OS, sorted by name.
func Discover() ([]App, error) {
	list, err := discover()
	sort.Slice(list, func(i, j int) bool { return strings.ToLower(list[i].Name) < strings.ToLower(list[j].Name) })
	return list, err
}

// Removable filters out OS components.
func Removable(all []App) []App {
	var out []App
	for _, a := range all {
		if !a.System {
			out = append(out, a)
		}
	}
	return out
}

// Find returns apps matching query: an exact (case-insensitive) name match
// wins outright, otherwise every app whose name or bundle ID contains it.
func Find(all []App, query string) []App {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return nil
	}
	cq := compact(q)
	var exact, partial []App
	for _, a := range all {
		name := strings.ToLower(a.Name)
		switch {
		case name == q || compact(name) == cq || strings.ToLower(a.BundleID) == q:
			exact = append(exact, a)
		case strings.Contains(name, q) || strings.Contains(compact(name), cq) || strings.Contains(strings.ToLower(a.BundleID), q):
			partial = append(partial, a)
		}
	}
	if len(exact) > 0 {
		return exact
	}
	return partial
}
