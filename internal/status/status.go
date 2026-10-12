// Package status collects a read-only snapshot of system health.
package status

import (
	"os"
	"runtime"
)

type Disk struct {
	Mount string `json:"mount"`
	Total uint64 `json:"total_bytes"`
	Free  uint64 `json:"free_bytes"`
}

type Info struct {
	OS        string `json:"os"`
	OSVersion string `json:"os_version"`
	Arch      string `json:"arch"`
	Host      string `json:"host"`
	CPUs      int    `json:"cpus"`
	MemTotal  uint64 `json:"memory_total_bytes"`
	MemAvail  uint64 `json:"memory_available_bytes"`
	Disks     []Disk `json:"disks"`
}

func Collect() Info {
	host, _ := os.Hostname()
	info := Info{OS: runtime.GOOS, Arch: runtime.GOARCH, Host: host, CPUs: runtime.NumCPU()}
	info.OSVersion = osVersion()
	info.MemTotal, info.MemAvail = memory()
	info.Disks = disks()
	return info
}

// Disks returns capacity and free space for the local fixed disks.
func Disks() []Disk { return disks() }
