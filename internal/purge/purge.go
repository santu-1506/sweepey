// Package purge finds rebuildable build output inside code projects:
// node_modules, Rust target/, Python virtualenvs, Gradle and Xcode output…
package purge

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/santu-1506/sweepey/internal/config"
	"github.com/santu-1506/sweepey/internal/fsutil"
)

// rule: a directory called Name is an artifact when its parent holds one of
// the marker files (no markers = always an artifact).
type rule struct {
	Name    string
	Markers []string
}

var rules = []rule{
	{"node_modules", nil},
	{"target", []string{"Cargo.toml", "pom.xml"}},
	{".build", []string{"Package.swift"}},
	{"build", []string{"build.gradle", "build.gradle.kts", "package.json", "CMakeLists.txt", "pubspec.yaml"}},
	{"dist", []string{"package.json", "pyproject.toml", "setup.py"}},
	{".next", []string{"package.json"}},
	{".nuxt", []string{"package.json"}},
	{".svelte-kit", []string{"package.json"}},
	{".turbo", []string{"package.json"}},
	{".parcel-cache", []string{"package.json"}},
	{".angular", []string{"angular.json"}},
	{".venv", []string{"pyproject.toml", "requirements.txt", "setup.py", "Pipfile"}},
	{"venv", []string{"pyproject.toml", "requirements.txt", "setup.py", "Pipfile"}},
	{".tox", []string{"tox.ini", "pyproject.toml"}},
	{".gradle", []string{"build.gradle", "build.gradle.kts", "settings.gradle", "settings.gradle.kts"}},
	{"Pods", []string{"Podfile"}},
	{".dart_tool", []string{"pubspec.yaml"}},
	{"obj", []string{"*.csproj", "*.fsproj", "*.sln"}},
	{"zig-cache", []string{"build.zig"}},
	{".zig-cache", []string{"build.zig"}},
}

type Artifact struct {
	Path    string
	Project string
	Kind    string
	Size    int64
	Recent  bool // project touched in the last 7 days; not selected by default
}

// DefaultRoots are the usual places people keep code, plus purge_paths.
func DefaultRoots() []string {
	h := fsutil.Home()
	var roots []string
	for _, d := range []string{"Projects", "projects", "GitHub", "github", "dev", "Developer", "code", "Code",
		"src", "workspace", "repos", "source/repos"} {
		roots = append(roots, filepath.Join(h, d))
	}
	return append(roots, config.PurgePaths()...)
}

// Scan walks the roots (up to maxDepth levels) and returns artifacts sorted
// by size. Matched artifacts are not descended into.
func Scan(roots []string, maxDepth int) []Artifact {
	var out []Artifact
	seenRoot := map[string]bool{}
	wl := config.Whitelist()
	for _, r := range roots {
		real, err := filepath.EvalSymlinks(r)
		if err != nil || !fsutil.IsDir(real) {
			continue
		}
		key := real
		if fsutil.CaseInsensitive() {
			key = strings.ToLower(real)
		}
		if seenRoot[key] {
			continue
		}
		seenRoot[key] = true
		base := strings.Count(real, string(filepath.Separator))
		_ = filepath.WalkDir(real, func(p string, d fs.DirEntry, err error) error {
			if err != nil || !d.IsDir() {
				return nil
			}
			if strings.Count(p, string(filepath.Separator))-base > maxDepth {
				return fs.SkipDir
			}
			name := d.Name()
			if name == ".git" || name == ".hg" || name == ".svn" {
				return fs.SkipDir
			}
			for _, rl := range rules {
				if name != rl.Name || !hasMarker(filepath.Dir(p), rl.Markers) {
					continue
				}
				if !config.Whitelisted(p, wl) {
					out = append(out, Artifact{Path: p, Project: filepath.Dir(p), Kind: name})
				}
				return fs.SkipDir
			}
			return nil
		})
	}

	paths := make([]string, len(out))
	for i, a := range out {
		paths[i] = a.Path
	}
	sizes := fsutil.Sizes(paths)
	week := time.Now().Add(-7 * 24 * time.Hour)
	for i := range out {
		out[i].Size = sizes[i]
		ignore := map[string]bool{out[i].Kind: true, ".DS_Store": true}
		newest := fsutil.NewestChild(out[i].Project, ignore)
		out[i].Recent = newest.IsZero() || newest.After(week)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Size > out[j].Size })
	return out
}

func hasMarker(dir string, markers []string) bool {
	if len(markers) == 0 {
		return true
	}
	for _, m := range markers {
		if strings.ContainsAny(m, "*?[") {
			if found, _ := filepath.Glob(filepath.Join(dir, m)); len(found) > 0 {
				return true
			}
			continue
		}
		if _, err := os.Stat(filepath.Join(dir, m)); err == nil {
			return true
		}
	}
	return false
}
