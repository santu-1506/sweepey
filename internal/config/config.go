// Package config reads sweepr's user settings: the whitelist of paths that
// must never be touched and extra roots for project purging.
package config

import (
	"bufio"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/santu-1506/sweepey/internal/fsutil"
)

// Dir is ~/.config/sweepr on macOS/Linux and %APPDATA%\sweepr on Windows.
func Dir() string {
	if runtime.GOOS == "windows" {
		if d := os.Getenv("APPDATA"); d != "" {
			return filepath.Join(d, "sweepr")
		}
	}
	return filepath.Join(fsutil.Home(), ".config", "sweepr")
}

func readLines(name string) []string {
	f, err := os.Open(filepath.Join(Dir(), name))
	if err != nil {
		return nil
	}
	defer f.Close()
	var out []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		out = append(out, fsutil.Expand(line))
	}
	return out
}

func appendLine(name, line string) error {
	if err := os.MkdirAll(Dir(), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(Dir(), name), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(line + "\n")
	return err
}

// Whitelist returns the protected path patterns (one per line, ~ and globs allowed).
func Whitelist() []string { return readLines("whitelist") }

func AddWhitelist(pattern string) error { return appendLine("whitelist", pattern) }

// PurgePaths returns extra project roots for `sweepr purge`.
func PurgePaths() []string { return readLines("purge_paths") }

// Whitelisted reports whether path matches a pattern or sits inside a
// whitelisted directory.
func Whitelisted(path string, patterns []string) bool {
	for _, p := range patterns {
		if ok, _ := filepath.Match(p, path); ok {
			return true
		}
		if fsutil.SamePath(path, p) || fsutil.HasPathPrefix(path, p) {
			return true
		}
	}
	return false
}
