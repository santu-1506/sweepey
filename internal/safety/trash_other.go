//go:build !darwin && !windows

package safety

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/santu-1506/sweepey/internal/fsutil"
)

// moveToTrash follows the freedesktop.org Trash spec so files can be
// restored from the desktop's trash view.
func moveToTrash(paths []string) map[string]error {
	errs := make(map[string]error, len(paths))
	base := filepath.Join(fsutil.Home(), ".local", "share", "Trash")
	files, info := filepath.Join(base, "files"), filepath.Join(base, "info")
	_ = os.MkdirAll(files, 0o700)
	_ = os.MkdirAll(info, 0o700)
	for _, p := range paths {
		name := filepath.Base(p)
		if fsutil.Exists(filepath.Join(files, name)) {
			name = fmt.Sprintf("%s.%d", name, time.Now().UnixNano())
		}
		meta := fmt.Sprintf("[Trash Info]\nPath=%s\nDeletionDate=%s\n",
			(&url.URL{Path: p}).EscapedPath(), time.Now().Format("2006-01-02T15:04:05"))
		if err := os.WriteFile(filepath.Join(info, name+".trashinfo"), []byte(meta), 0o600); err != nil {
			errs[p] = err
			continue
		}
		errs[p] = os.Rename(p, filepath.Join(files, name))
	}
	return errs
}

func removeRegistryKey(string) error { return errors.New("registry keys only exist on Windows") }

func AdminRemove(paths []string) error {
	for _, p := range paths {
		if err := Validate(p); err != nil {
			return err
		}
	}
	cmd := exec.Command("sudo", append([]string{"rm", "-rf", "--"}, paths...)...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}
