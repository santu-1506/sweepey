// Package safety is the only code in sweepr that deletes anything. Every
// path goes through Validate first, whatever scanner (or AI) proposed it.
package safety

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/santu-1506/sweepey/internal/fsutil"
)

// protectedRoots may never be deleted, nor may any of their ancestors.
// Their contents are fair game (that is where caches live).
func protectedRoots() []string {
	h := fsutil.Home()
	roots := []string{h, os.TempDir()}
	for _, d := range []string{"Desktop", "Documents", "Downloads", "Pictures", "Music", "Videos", "Movies", "Public"} {
		roots = append(roots, filepath.Join(h, d))
	}
	switch runtime.GOOS {
	case "darwin":
		lib := filepath.Join(h, "Library")
		for _, d := range []string{"Application Support", "Caches", "Preferences", "Preferences/ByHost", "Logs",
			"Containers", "Group Containers", "Saved Application State", "LaunchAgents", "HTTPStorages",
			"WebKit", "Cookies", "Application Scripts", "Developer", "Internet Plug-Ins", "Fonts"} {
			roots = append(roots, filepath.Join(lib, d))
		}
		for _, d := range []string{"Application Support", "Caches", "Preferences", "LaunchAgents", "LaunchDaemons",
			"PrivilegedHelperTools", "Logs", "Internet Plug-Ins", "Audio/Plug-Ins/Components",
			"Audio/Plug-Ins/VST", "Audio/Plug-Ins/VST3", "Fonts", "Extensions", "Frameworks"} {
			roots = append(roots, filepath.Join("/Library", d))
		}
		// Per-user cache dir that sits next to $TMPDIR (…/T) as …/C.
		roots = append(roots, filepath.Join(filepath.Dir(filepath.Clean(os.TempDir())), "C"))
		roots = append(roots, "/Applications", "/Applications/Utilities", filepath.Join(h, "Applications"),
			filepath.Join(h, ".Trash"), "/Users/Shared", "/Volumes", "/opt/homebrew", "/usr/local")
	case "windows":
		for _, env := range []string{"SystemDrive", "ProgramFiles", "ProgramFiles(x86)", "ProgramData",
			"APPDATA", "LOCALAPPDATA", "PUBLIC", "TEMP"} {
			if v := os.Getenv(env); v != "" {
				roots = append(roots, v)
			}
		}
		if v := os.Getenv("SystemDrive"); v != "" {
			roots = append(roots, v+`\`, filepath.Join(v+`\`, "Users"))
		}
		local := os.Getenv("LOCALAPPDATA")
		roam := os.Getenv("APPDATA")
		pd := os.Getenv("ProgramData")
		roots = append(roots,
			filepath.Join(h, "AppData"), filepath.Join(h, "AppData", "LocalLow"),
			filepath.Join(local, "Programs"), filepath.Join(local, "Microsoft"), filepath.Join(roam, "Microsoft"),
			filepath.Join(roam, `Microsoft\Windows\Start Menu\Programs`),
			filepath.Join(pd, `Microsoft\Windows\Start Menu\Programs`),
			filepath.Join(local, "Packages"))
	default:
		roots = append(roots, filepath.Join(h, ".cache"), filepath.Join(h, ".config"),
			filepath.Join(h, ".local"), filepath.Join(h, ".local/share"))
	}
	return roots
}

// protectedTrees may not be touched at any depth, except for the listed
// exceptions (which are maintenance folders the OS expects to be emptied).
func protectedTrees() (trees, exceptions []string) {
	switch runtime.GOOS {
	case "darwin":
		return []string{"/System", "/usr", "/bin", "/sbin", "/etc", "/private/etc", "/private/var/db",
				"/dev", "/cores", "/Library/Apple", "/Library/Keychains"},
			nil
	case "windows":
		win := os.Getenv("SystemRoot")
		if win == "" {
			win = `C:\Windows`
		}
		return []string{win},
			[]string{filepath.Join(win, "Temp"), filepath.Join(win, `SoftwareDistribution\Download`)}
	default:
		return []string{"/bin", "/boot", "/dev", "/etc", "/lib", "/lib64", "/proc", "/sbin", "/sys", "/usr", "/var/lib"}, nil
	}
}

// protectedFragments guard user data that tends to sit next to app data.
var protectedFragments = []string{
	"/Library/Keychains", "/Library/Mobile Documents", "/Library/CloudStorage", "/Library/Mail/",
	"/Library/Messages", "/Library/Photos", "com.apple.TCC", "/.ssh", "/.gnupg",
	`\Microsoft\Credentials`, `\Microsoft\Protect`, `\Microsoft\Crypto`, `\.ssh`,
}

var ErrProtected = errors.New("protected location")

// Validate refuses anything that is not clearly safe to remove.
func Validate(p string) error {
	if strings.TrimSpace(p) == "" {
		return errors.New("empty path")
	}
	if !filepath.IsAbs(p) {
		return fmt.Errorf("refusing relative path %q", p)
	}
	clean := filepath.Clean(p)
	rest := strings.Trim(filepath.ToSlash(strings.TrimPrefix(clean, filepath.VolumeName(clean))), "/")
	if rest == "" || !strings.Contains(rest, "/") {
		return fmt.Errorf("%w: %s is too close to the filesystem root", ErrProtected, clean)
	}
	for _, r := range protectedRoots() {
		if r == "" {
			continue
		}
		if fsutil.SamePath(clean, r) {
			return fmt.Errorf("%w: %s", ErrProtected, clean)
		}
		if fsutil.HasPathPrefix(r, clean) {
			return fmt.Errorf("%w: %s contains %s", ErrProtected, clean, r)
		}
	}
	trees, exceptions := protectedTrees()
	for _, t := range trees {
		if fsutil.SamePath(clean, t) || fsutil.HasPathPrefix(clean, t) {
			allowed := false
			for _, e := range exceptions {
				if fsutil.HasPathPrefix(clean, e) {
					allowed = true
				}
			}
			if !allowed {
				return fmt.Errorf("%w: %s is inside %s", ErrProtected, clean, t)
			}
		}
	}
	lower := strings.ToLower(clean)
	for _, f := range protectedFragments {
		if strings.Contains(lower, strings.ToLower(f)) {
			return fmt.Errorf("%w: %s holds personal data", ErrProtected, clean)
		}
	}
	return nil
}
