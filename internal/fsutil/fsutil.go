// Package fsutil has the filesystem helpers shared by every scanner.
package fsutil

import (
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// Home returns the user's home directory, or "" if it cannot be determined.
func Home() string {
	h, _ := os.UserHomeDir()
	return h
}

// Tilde shortens paths under the home directory to ~/... for display and for
// anything sent to the AI service.
func Tilde(p string) string {
	h := Home()
	if h != "" && SamePath(p, h) {
		return "~"
	}
	if h != "" && HasPathPrefix(p, h) {
		return "~" + p[len(h):]
	}
	return p
}

// Expand turns a leading ~ into the home directory.
func Expand(p string) string {
	if p == "~" {
		return Home()
	}
	if strings.HasPrefix(p, "~/") || strings.HasPrefix(p, `~\`) {
		return filepath.Join(Home(), p[2:])
	}
	return p
}

// CaseInsensitive reports whether the default filesystem ignores case
// (APFS and NTFS do by default).
func CaseInsensitive() bool { return runtime.GOOS == "darwin" || runtime.GOOS == "windows" }

// SamePath compares two cleaned paths with the platform's case rules.
func SamePath(a, b string) bool {
	a, b = filepath.Clean(a), filepath.Clean(b)
	if CaseInsensitive() {
		return strings.EqualFold(a, b)
	}
	return a == b
}

// HasPathPrefix reports whether p is strictly inside dir.
func HasPathPrefix(p, dir string) bool {
	p, dir = filepath.Clean(p), filepath.Clean(dir)
	if !strings.HasSuffix(dir, string(filepath.Separator)) {
		dir += string(filepath.Separator)
	}
	if len(p) < len(dir) {
		return false
	}
	if CaseInsensitive() {
		return strings.EqualFold(p[:len(dir)], dir)
	}
	return p[:len(dir)] == dir
}

func Exists(p string) bool {
	_, err := os.Lstat(p)
	return err == nil
}

func IsDir(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.IsDir()
}

// Size returns the apparent size of a file or directory tree. Symlinks are
// counted as links, never followed, and unreadable parts are skipped. A
// directory that cannot be read at all (macOS privacy protection, missing
// rights) reports -1, shown as "?".
func Size(path string) int64 {
	info, err := os.Lstat(path)
	if err != nil {
		return 0
	}
	if !info.IsDir() {
		return info.Size()
	}
	var total int64
	unreadable := false
	_ = filepath.WalkDir(path, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if p == path {
				unreadable = true
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if fi, err := d.Info(); err == nil {
			total += fi.Size()
		}
		return nil
	})
	if unreadable {
		return -1
	}
	return total
}

// Sizes measures many paths in parallel; results line up with the input.
func Sizes(paths []string) []int64 {
	out := make([]int64, len(paths))
	sem := make(chan struct{}, runtime.NumCPU()*2)
	var wg sync.WaitGroup
	for i, p := range paths {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, p string) {
			defer wg.Done()
			out[i] = Size(p)
			<-sem
		}(i, p)
	}
	wg.Wait()
	return out
}

// NewestChild returns the most recent modification time among the direct
// children of dir, skipping names in ignore.
func NewestChild(dir string, ignore map[string]bool) time.Time {
	var newest time.Time
	entries, err := os.ReadDir(dir)
	if err != nil {
		return newest
	}
	for _, e := range entries {
		if ignore[e.Name()] {
			continue
		}
		if fi, err := e.Info(); err == nil && fi.ModTime().After(newest) {
			newest = fi.ModTime()
		}
	}
	return newest
}
