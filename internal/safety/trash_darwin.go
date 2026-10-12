//go:build darwin

package safety

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"github.com/santu-1506/sweepey/internal/fsutil"
)

func moveToTrash(paths []string) map[string]error {
	errs := make(map[string]error, len(paths))
	trash := filepath.Join(fsutil.Home(), ".Trash")
	for _, p := range paths {
		dest := filepath.Join(trash, filepath.Base(p))
		if fsutil.Exists(dest) {
			dest = fmt.Sprintf("%s %s", dest, time.Now().Format("15.04.05.000"))
		}
		err := os.Rename(p, dest)
		if errors.Is(err, syscall.EXDEV) {
			err = fmt.Errorf("%s is on another volume; re-run with --permanent to delete it", p)
		}
		errs[p] = err
	}
	return errs
}

func removeRegistryKey(string) error { return errors.New("registry keys only exist on Windows") }

// AdminRemove deletes root-owned leftovers with sudo after the user agreed.
// Every path is validated again right before the privileged call.
func AdminRemove(paths []string) error {
	for _, p := range paths {
		if err := Validate(p); err != nil {
			return err
		}
	}
	args := append([]string{"rm", "-rf", "--"}, paths...)
	cmd := exec.Command("sudo", args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}
