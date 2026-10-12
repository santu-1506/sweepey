//go:build windows

package status

import (
	"os/exec"
	"strings"
	"syscall"
	"unsafe"
)

var (
	k32                  = syscall.NewLazyDLL("kernel32.dll")
	procGetDiskFreeSpace = k32.NewProc("GetDiskFreeSpaceExW")
	procGetLogicalDrives = k32.NewProc("GetLogicalDrives")
	procGetDriveType     = k32.NewProc("GetDriveTypeW")
	procMemStatus        = k32.NewProc("GlobalMemoryStatusEx")
)

const driveFixed = 3

func disks() []Disk {
	mask, _, _ := procGetLogicalDrives.Call()
	var out []Disk
	for i := 0; i < 26; i++ {
		if mask&(1<<uint(i)) == 0 {
			continue
		}
		root := string(rune('A'+i)) + `:\`
		p, _ := syscall.UTF16PtrFromString(root)
		if t, _, _ := procGetDriveType.Call(uintptr(unsafe.Pointer(p))); t != driveFixed {
			continue
		}
		var free, total, totalFree uint64
		r, _, _ := procGetDiskFreeSpace.Call(uintptr(unsafe.Pointer(p)),
			uintptr(unsafe.Pointer(&free)), uintptr(unsafe.Pointer(&total)), uintptr(unsafe.Pointer(&totalFree)))
		if r != 0 {
			out = append(out, Disk{Mount: root, Total: total, Free: free})
		}
	}
	return out
}

type memoryStatusEx struct {
	Length               uint32
	MemoryLoad           uint32
	TotalPhys            uint64
	AvailPhys            uint64
	TotalPageFile        uint64
	AvailPageFile        uint64
	TotalVirtual         uint64
	AvailVirtual         uint64
	AvailExtendedVirtual uint64
}

func memory() (total, avail uint64) {
	var m memoryStatusEx
	m.Length = uint32(unsafe.Sizeof(m))
	if r, _, _ := procMemStatus.Call(uintptr(unsafe.Pointer(&m))); r == 0 {
		return 0, 0
	}
	return m.TotalPhys, m.AvailPhys
}

func osVersion() string {
	out, err := exec.Command("cmd", "/c", "ver").Output()
	if err != nil {
		return "Windows"
	}
	return strings.TrimSpace(string(out))
}
