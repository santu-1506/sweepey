//go:build windows

package safety

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/santu-1506/sweepey/internal/config"
)

// moveToTrash sends everything to the Recycle Bin in a single PowerShell
// call, so a hundred leftovers do not mean a hundred process launches.
func moveToTrash(paths []string) map[string]error {
	errs := make(map[string]error, len(paths))
	list, err := os.CreateTemp("", "sweepr-recycle-*.txt")
	if err != nil {
		for _, p := range paths {
			errs[p] = err
		}
		return errs
	}
	defer os.Remove(list.Name())
	_, _ = list.WriteString(strings.Join(paths, "\n"))
	list.Close()

	script := `[Console]::OutputEncoding = [Text.Encoding]::UTF8
Add-Type -AssemblyName Microsoft.VisualBasic
foreach ($p in Get-Content -LiteralPath '` + psQuote(list.Name()) + `' -Encoding UTF8) {
  if (-not $p) { continue }
  try {
    if (Test-Path -LiteralPath $p -PathType Container) {
      [Microsoft.VisualBasic.FileIO.FileSystem]::DeleteDirectory($p, 'OnlyErrorDialogs', 'SendToRecycleBin')
    } else {
      [Microsoft.VisualBasic.FileIO.FileSystem]::DeleteFile($p, 'OnlyErrorDialogs', 'SendToRecycleBin')
    }
    "OK` + "`t" + `$p"
  } catch { "ERR` + "`t" + `$p` + "`t" + `$($_.Exception.Message)" }
}`
	out, runErr := PowerShell(script)
	for _, line := range strings.Split(out, "\n") {
		parts := strings.SplitN(strings.TrimRight(line, "\r"), "\t", 3)
		if len(parts) < 2 {
			continue
		}
		switch parts[0] {
		case "OK":
			errs[parts[1]] = nil
		case "ERR":
			msg := "could not recycle"
			if len(parts) == 3 {
				msg = parts[2]
			}
			errs[parts[1]] = errors.New(msg)
		}
	}
	for _, p := range paths {
		if _, seen := errs[p]; !seen {
			if runErr == nil {
				runErr = errors.New("no result from PowerShell")
			}
			errs[p] = runErr
		}
	}
	return errs
}

func psQuote(s string) string { return strings.ReplaceAll(s, "'", "''") }

// PowerShell runs a script without loading the user's profile.
func PowerShell(script string) (string, error) {
	out, err := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass",
		"-Command", script).CombinedOutput()
	return string(out), err
}

// removeRegistryKey exports a .reg backup before deleting, so a mistake can
// be undone by double-clicking the backup.
func removeRegistryKey(key string) error {
	dir := filepath.Join(config.Dir(), "registry-backups")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	name := strings.NewReplacer(`\`, "_", ":", "", " ", "_").Replace(key)
	if len(name) > 120 {
		name = name[len(name)-120:]
	}
	backup := filepath.Join(dir, time.Now().Format("20060102-150405")+"-"+name+".reg")
	if out, err := exec.Command("reg", "export", key, backup, "/y").CombinedOutput(); err != nil {
		return fmt.Errorf("backup failed, key left alone: %s", strings.TrimSpace(string(out)))
	}
	if out, err := exec.Command("reg", "delete", key, "/f").CombinedOutput(); err != nil {
		return errors.New(strings.TrimSpace(string(out)))
	}
	return nil
}

func AdminRemove([]string) error {
	return errors.New("re-run sweepr from an Administrator terminal to remove these items")
}
