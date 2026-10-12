//go:build !windows

package status

import (
	"bufio"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"syscall"

	"github.com/santu-1506/sweepey/internal/fsutil"
)

func statfs(path string) (Disk, bool) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return Disk{}, false
	}
	bs := uint64(st.Bsize)
	return Disk{Mount: path, Total: st.Blocks * bs, Free: st.Bavail * bs}, true
}

func disks() []Disk {
	var out []Disk
	for _, p := range []string{"/", fsutil.Home()} {
		if d, ok := statfs(p); ok {
			if len(out) > 0 && out[0].Total == d.Total && out[0].Free == d.Free {
				continue // home is on the root volume
			}
			out = append(out, d)
		}
	}
	return out
}

func osVersion() string {
	if runtime.GOOS == "darwin" {
		if out, err := exec.Command("sw_vers", "-productVersion").Output(); err == nil {
			return "macOS " + strings.TrimSpace(string(out))
		}
	}
	if f, err := os.Open("/etc/os-release"); err == nil {
		defer f.Close()
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			if v, ok := strings.CutPrefix(sc.Text(), "PRETTY_NAME="); ok {
				return strings.Trim(v, `"`)
			}
		}
	}
	return runtime.GOOS
}

func memory() (total, avail uint64) {
	if runtime.GOOS == "darwin" {
		if out, err := exec.Command("sysctl", "-n", "hw.memsize").Output(); err == nil {
			total, _ = strconv.ParseUint(strings.TrimSpace(string(out)), 10, 64)
		}
		// Available ≈ free + inactive + speculative + purgeable pages.
		out, err := exec.Command("vm_stat").Output()
		if err != nil {
			return total, 0
		}
		page := uint64(16384)
		lines := strings.Split(string(out), "\n")
		if len(lines) > 0 {
			if i := strings.Index(lines[0], "page size of "); i >= 0 {
				f := strings.Fields(lines[0][i+len("page size of "):])
				if len(f) > 0 {
					if v, err := strconv.ParseUint(f[0], 10, 64); err == nil {
						page = v
					}
				}
			}
		}
		for _, l := range lines {
			for _, k := range []string{"Pages free:", "Pages inactive:", "Pages speculative:", "Pages purgeable:"} {
				if v, ok := strings.CutPrefix(l, k); ok {
					n, _ := strconv.ParseUint(strings.TrimSuffix(strings.TrimSpace(v), "."), 10, 64)
					avail += n * page
				}
			}
		}
		return total, avail
	}
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0, 0
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) < 2 {
			continue
		}
		v, _ := strconv.ParseUint(fields[1], 10, 64)
		switch fields[0] {
		case "MemTotal:":
			total = v * 1024
		case "MemAvailable:":
			avail = v * 1024
		}
	}
	return total, avail
}
