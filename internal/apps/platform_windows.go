//go:build windows

package apps

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"

	"github.com/santu-1506/sweepey/internal/fsutil"
	"github.com/santu-1506/sweepey/internal/safety"
)

type uninstallEntry struct {
	DisplayName          string `json:"DisplayName"`
	DisplayVersion       any    `json:"DisplayVersion"`
	Publisher            string `json:"Publisher"`
	InstallLocation      string `json:"InstallLocation"`
	UninstallString      string `json:"UninstallString"`
	QuietUninstallString string `json:"QuietUninstallString"`
	DisplayIcon          string `json:"DisplayIcon"`
	EstimatedSize        any    `json:"EstimatedSize"`
	SystemComponent      any    `json:"SystemComponent"`
	ParentKeyName        string `json:"ParentKeyName"`
	ReleaseType          string `json:"ReleaseType"`
	PSPath               string `json:"PSPath"`
}

// discover reads the same Uninstall registry keys that "Apps & features" uses.
func discover() ([]App, error) {
	script := `[Console]::OutputEncoding = [Text.Encoding]::UTF8
$paths = 'HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall\*',
         'HKLM:\SOFTWARE\WOW6432Node\Microsoft\Windows\CurrentVersion\Uninstall\*',
         'HKCU:\Software\Microsoft\Windows\CurrentVersion\Uninstall\*'
@(Get-ItemProperty -Path $paths -ErrorAction SilentlyContinue | Where-Object { $_.DisplayName } |
  Select-Object DisplayName, DisplayVersion, Publisher, InstallLocation, UninstallString, QuietUninstallString,
                DisplayIcon, EstimatedSize, SystemComponent, ParentKeyName, ReleaseType, PSPath) | ConvertTo-Json -Compress -Depth 2`
	out, err := safety.PowerShell(script)
	if err != nil {
		return nil, errors.New("could not read installed programs: " + strings.TrimSpace(out))
	}
	out = strings.TrimSpace(out)
	var entries []uninstallEntry
	if strings.HasPrefix(out, "{") {
		var one uninstallEntry
		err = json.Unmarshal([]byte(out), &one)
		entries = []uninstallEntry{one}
	} else if out != "" {
		err = json.Unmarshal([]byte(out), &entries)
	}
	if err != nil {
		return nil, err
	}

	var apps []App
	seen := map[string]bool{}
	for _, e := range entries {
		if e.ParentKeyName != "" || e.ReleaseType != "" || asInt(e.SystemComponent) == 1 {
			continue
		}
		key := strings.ToLower(e.DisplayName + "|" + str(e.DisplayVersion))
		if seen[key] {
			continue
		}
		seen[key] = true
		a := App{
			Name:              e.DisplayName,
			Vendor:            e.Publisher,
			Version:           str(e.DisplayVersion),
			UninstallCmd:      e.UninstallString,
			QuietUninstallCmd: e.QuietUninstallString,
			RegistryKey:       regPath(e.PSPath),
			Size:              int64(asInt(e.EstimatedSize)) * 1024,
		}
		a.Path = installDir(e)
		if exe := iconExe(e.DisplayIcon); exe != "" {
			a.Executable = strings.TrimSuffix(filepath.Base(exe), filepath.Ext(exe))
		}
		apps = append(apps, a)
	}
	return apps, nil
}

func str(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	}
	return ""
}

func asInt(v any) int {
	if f, ok := v.(float64); ok {
		return int(f)
	}
	return 0
}

func regPath(ps string) string {
	p := strings.TrimPrefix(ps, `Microsoft.PowerShell.Core\Registry::`)
	p = strings.Replace(p, "HKEY_LOCAL_MACHINE", "HKLM", 1)
	return strings.Replace(p, "HKEY_CURRENT_USER", "HKCU", 1)
}

func iconExe(icon string) string {
	icon = strings.Trim(icon, `" `)
	if i := strings.LastIndex(icon, ","); i > 0 {
		icon = icon[:i]
	}
	icon = strings.Trim(icon, `" `)
	if strings.EqualFold(filepath.Ext(icon), ".exe") {
		return icon
	}
	return ""
}

// installDir prefers InstallLocation and otherwise guesses from the icon,
// but only accepts a folder that is safe to remove and not inside Windows.
func installDir(e uninstallEntry) string {
	win := os.Getenv("SystemRoot")
	for _, c := range []string{strings.Trim(e.InstallLocation, `" `), filepath.Dir(iconExe(e.DisplayIcon))} {
		if c == "" || c == "." {
			continue
		}
		c = filepath.Clean(c)
		if win != "" && fsutil.HasPathPrefix(c, win) {
			continue
		}
		if safety.Validate(c) == nil && fsutil.IsDir(c) {
			return c
		}
	}
	return ""
}

func searchRoots() []root {
	h := fsutil.Home()
	roam, local, pd := os.Getenv("APPDATA"), os.Getenv("LOCALAPPDATA"), os.Getenv("ProgramData")
	roots := []root{
		{Dir: roam}, {Dir: local}, {Dir: filepath.Join(local, "Programs")}, {Dir: filepath.Join(h, "AppData", "LocalLow")},
		{Dir: pd}, {Dir: os.Getenv("ProgramFiles")}, {Dir: os.Getenv("ProgramFiles(x86)")},
		{Dir: filepath.Join(roam, `Microsoft\Windows\Start Menu\Programs`)},
		{Dir: filepath.Join(pd, `Microsoft\Windows\Start Menu\Programs`)},
		{Dir: filepath.Join(h, "Desktop"), Ext: ".lnk"},
		{Dir: filepath.Join(os.Getenv("PUBLIC"), "Desktop"), Ext: ".lnk"},
		{Dir: os.TempDir(), MaxConf: Medium},
		{Dir: h, DotOnly: true, MaxConf: Medium},
	}
	var out []root
	for _, r := range roots {
		if r.Dir != "" && r.Dir != "." {
			out = append(out, r)
		}
	}
	return out
}

// platformExtras finds registry keys under HKCU/HKLM Software that belong to
// the app, using the same name matching as for folders.
func platformExtras(a App, m *matcher, others []named) []Candidate {
	var out []Candidate
	if a.RegistryKey != "" && exec.Command("reg", "query", a.RegistryKey).Run() == nil {
		out = append(out, Candidate{Path: a.RegistryKey, Kind: safety.Registry, Confidence: High, Reason: "uninstall entry still registered"})
	}
	script := `[Console]::OutputEncoding = [Text.Encoding]::UTF8
foreach ($r in 'HKCU:\Software', 'HKLM:\SOFTWARE', 'HKLM:\SOFTWARE\WOW6432Node') {
  Get-ChildItem -Path $r -ErrorAction SilentlyContinue | Where-Object { $_.PSChildName -notin 'Classes','Microsoft','Policies','WOW6432Node','Wow6432Node','RegisteredApplications','Clients' } | ForEach-Object {
    "K1` + "`t" + `" + $_.Name
    $parent = $_.Name
    Get-ChildItem -LiteralPath $_.PSPath -ErrorAction SilentlyContinue | ForEach-Object { "K2` + "`t" + `$parent` + "`t" + `" + $_.PSChildName }
  }
}`
	text, err := safety.PowerShell(script)
	if err != nil {
		return out
	}
	children := map[string][]string{}
	var level1 []string
	for _, line := range strings.Split(text, "\n") {
		parts := strings.Split(strings.TrimRight(line, "\r"), "\t")
		switch {
		case len(parts) == 2 && parts[0] == "K1":
			level1 = append(level1, shortHive(parts[1]))
		case len(parts) == 3 && parts[0] == "K2":
			p := shortHive(parts[1])
			children[p] = append(children[p], parts[2])
		}
	}
	for _, key := range level1 {
		leaf := baseName(key)
		n := normalize(leaf)
		if conf, reason := m.match(n); conf > None {
			out = append(out, Candidate{Path: key, Kind: safety.Registry, Confidence: conf, Reason: "registry: " + reason})
			continue
		}
		if !m.isVendor(n) {
			continue
		}
		for _, child := range children[key] {
			if conf, reason := m.match(normalize(child)); conf > None {
				out = append(out, Candidate{Path: key + `\` + child, Kind: safety.Registry, Confidence: conf,
					Reason: "registry: " + reason + " (under vendor key)"})
			}
		}
	}
	return out
}

func shortHive(k string) string {
	k = strings.Replace(k, "HKEY_LOCAL_MACHINE", "HKLM", 1)
	return strings.Replace(k, "HKEY_CURRENT_USER", "HKCU", 1)
}

func orphanRoots() []root {
	h := fsutil.Home()
	return []root{{Dir: os.Getenv("APPDATA")}, {Dir: os.Getenv("LOCALAPPDATA")},
		{Dir: filepath.Join(h, "AppData", "LocalLow")}, {Dir: os.Getenv("ProgramData")}}
}

var windowsOwned = map[string]bool{
	"microsoft": true, "packages": true, "temp": true, "programs": true, "comms": true, "connecteddevicesplatform": true,
	"d3dscache": true, "crashdumps": true, "publishers": true, "virtualstore": true, "history": true,
	"placeholdertileassetsfolder": true, "packagecache": true, "ssh": true, "usoshared": true, "usoprivate": true,
	"softwaredistribution": true, "windows": true, "windowsholographicdevices": true, "desktopini": true,
	"regidcommicrosoft": true, "nvidia": true, "nvidiacorporation": true, "intel": true, "amd": true, "ati": true,
	"applicationdata": true, "startmenu": true, "templates": true, "documents": true, "desktop": true,
	"identities": true, "peerdistrepub": true, "devicesync": true, "dbg": true, "chocolatey": true,
	"chocolateyhttpcache": true, "mozillamaintenanceservice": true, "ntuser": true, "iconcache": true,
}

func orphanCheck(n string, isDir bool, ms []named) (Confidence, string) {
	c := compact(n)
	if !isDir || windowsOwned[c] || strings.HasPrefix(c, "regid") || strings.HasPrefix(n, "{") || len(c) < 3 {
		return None, ""
	}
	if ownedByInstalled(n, ms) {
		return None, ""
	}
	return Low, "no installed program matches this folder"
}

func IsRunning(App) bool { return false }
func Quit(App) error     { return nil }

var msiInstall = regexp.MustCompile(`(?i)msiexec(\.exe)?\s+/i`)

// RunUninstaller starts the program's own uninstaller and waits for it, so
// services, drivers and registry entries are removed properly before the
// leftover scan.
func RunUninstaller(a App, quiet bool) error {
	cmdline := a.UninstallCmd
	if quiet && a.QuietUninstallCmd != "" {
		cmdline = a.QuietUninstallCmd
	}
	if cmdline == "" {
		return errors.New("no uninstaller registered")
	}
	cmdline = msiInstall.ReplaceAllString(cmdline, "MsiExec.exe /X")
	cmd := exec.Command("cmd.exe")
	cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: `cmd.exe /s /c "` + cmdline + `"`}
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}

func Receipts(App) []string { return nil }
