// Package analyze measures what is taking space inside a directory.
package analyze

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"

	"github.com/santu-1506/sweepey/internal/fsutil"
)

type Entry struct {
	Path  string
	Name  string
	Size  int64
	IsDir bool
}

// Children returns the direct children of dir with their total sizes,
// largest first.
func Children(dir string) ([]Entry, error) {
	des, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	entries := make([]Entry, 0, len(des))
	paths := make([]string, 0, len(des))
	for _, d := range des {
		p := filepath.Join(dir, d.Name())
		entries = append(entries, Entry{Path: p, Name: d.Name(), IsDir: d.IsDir()})
		paths = append(paths, p)
	}
	for i, s := range fsutil.Sizes(paths) {
		entries[i].Size = s
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Size > entries[j].Size })
	return entries, nil
}

// Reveal shows the path in Finder / Explorer / the file manager.
func Reveal(p string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", "-R", p).Run()
	case "windows":
		// explorer.exe returns exit code 1 even on success.
		_ = exec.Command("explorer", "/select,", p).Run()
		return nil
	default:
		return exec.Command("xdg-open", filepath.Dir(p)).Run()
	}
}
