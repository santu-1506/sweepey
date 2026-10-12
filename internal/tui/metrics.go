package tui

import (
	"sort"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/load"
	"github.com/shirou/gopsutil/v4/mem"
	"github.com/shirou/gopsutil/v4/net"
	"github.com/shirou/gopsutil/v4/process"

	"github.com/santu-1506/sweepey/internal/status"
)

// Proc is one row of the process table.
type Proc struct {
	PID  int32
	Name string
	CPU  float64
	RSS  uint64
}

// Sample is one snapshot of live system metrics.
type Sample struct {
	CPU                 float64
	Cores               []float64
	MemPct              float64
	MemUsed, MemTotal   uint64
	SwapUsed, SwapTotal uint64
	Disks               []status.Disk
	RxRate, TxRate      float64
	Load                [3]float64
	Uptime              time.Duration
	Procs               []Proc
	ProcCount           int

	// Copies of the sampler's history, safe to read from the UI goroutine.
	CPUHist, MemHist, RxHist, TxHist []float64
}

// Sampler keeps the previous readings needed to turn counters into rates.
type Sampler struct {
	procs   bool
	prevNet *net.IOCountersStat
	prevAt  time.Time
	prevCPU map[int32]float64
	diskAt  time.Time
	disks   []status.Disk

	CPUHist, MemHist, RxHist, TxHist []float64 // owned by the sampling goroutine
}

const histLen = 240

func NewSampler(withProcs bool) *Sampler {
	_, _ = cpu.Percent(0, true) // prime the per-core counters
	return &Sampler{procs: withProcs, prevCPU: map[int32]float64{}}
}

func push(h []float64, v float64) []float64 {
	h = append(h, v)
	if len(h) > histLen {
		h = h[len(h)-histLen:]
	}
	return h
}

func (s *Sampler) Sample() Sample {
	now := time.Now()
	var out Sample
	if per, err := cpu.Percent(0, true); err == nil && len(per) > 0 {
		out.Cores = per
		sum := 0.0
		for _, v := range per {
			sum += v
		}
		out.CPU = sum / float64(len(per))
	}
	if v, err := mem.VirtualMemory(); err == nil {
		out.MemPct, out.MemUsed, out.MemTotal = v.UsedPercent, v.Used, v.Total
	}
	if sw, err := mem.SwapMemory(); err == nil {
		out.SwapUsed, out.SwapTotal = sw.Used, sw.Total
	}
	if s.disks == nil || now.Sub(s.diskAt) > 5*time.Second {
		s.disks, s.diskAt = status.Disks(), now
	}
	out.Disks = s.disks
	dt := now.Sub(s.prevAt).Seconds()
	if n, err := net.IOCounters(false); err == nil && len(n) > 0 {
		cur := n[0]
		if s.prevNet != nil && dt > 0 {
			if cur.BytesRecv >= s.prevNet.BytesRecv {
				out.RxRate = float64(cur.BytesRecv-s.prevNet.BytesRecv) / dt
			}
			if cur.BytesSent >= s.prevNet.BytesSent {
				out.TxRate = float64(cur.BytesSent-s.prevNet.BytesSent) / dt
			}
		}
		s.prevNet = &cur
	}
	if l, err := load.Avg(); err == nil {
		out.Load = [3]float64{l.Load1, l.Load5, l.Load15}
	}
	if u, err := host.Uptime(); err == nil {
		out.Uptime = time.Duration(u) * time.Second
	}
	if s.procs {
		out.Procs, out.ProcCount = s.topProcs(dt)
	}
	s.prevAt = now
	s.CPUHist = push(s.CPUHist, out.CPU)
	s.MemHist = push(s.MemHist, out.MemPct)
	s.RxHist = push(s.RxHist, out.RxRate)
	s.TxHist = push(s.TxHist, out.TxRate)
	out.CPUHist = append([]float64(nil), s.CPUHist...)
	out.MemHist = append([]float64(nil), s.MemHist...)
	out.RxHist = append([]float64(nil), s.RxHist...)
	out.TxHist = append([]float64(nil), s.TxHist...)
	return out
}

// topProcs computes per-process CPU from the change in CPU time since the
// previous sample, then fills in names and memory for the busiest ones.
func (s *Sampler) topProcs(dt float64) ([]Proc, int) {
	ps, err := process.Processes()
	if err != nil {
		return nil, 0
	}
	type pc struct {
		p   *process.Process
		cpu float64
	}
	next := make(map[int32]float64, len(ps))
	var list []pc
	for _, p := range ps {
		t, err := p.Times()
		if err != nil {
			continue
		}
		total := t.User + t.System
		next[p.Pid] = total
		if prev, ok := s.prevCPU[p.Pid]; ok && dt > 0 {
			list = append(list, pc{p, max(0, (total-prev)/dt*100)})
		}
	}
	s.prevCPU = next
	sort.Slice(list, func(i, j int) bool { return list[i].cpu > list[j].cpu })
	if len(list) > 14 {
		list = list[:14]
	}
	out := make([]Proc, 0, len(list))
	for _, x := range list {
		name, _ := x.p.Name()
		var rss uint64
		if mi, err := x.p.MemoryInfo(); err == nil && mi != nil {
			rss = mi.RSS
		}
		out = append(out, Proc{PID: x.p.Pid, Name: name, CPU: x.cpu, RSS: rss})
	}
	return out, len(ps)
}

// Health scores the machine 0-100 from CPU, memory, swap and disk pressure.
func Health(s Sample) (int, string, string) {
	score := 100.0
	if s.CPU > 50 {
		score -= (s.CPU - 50) * 0.5
	}
	if s.MemPct > 70 {
		score -= (s.MemPct - 70) * 0.9
	}
	if s.SwapTotal > 0 && s.SwapUsed > 2<<30 {
		score -= 6
	}
	for _, d := range s.Disks {
		if d.Total > 0 {
			free := float64(d.Free) / float64(d.Total) * 100
			if free < 20 {
				score -= (20 - free) * 1.8
			}
		}
	}
	n := int(max(0, min(100, score)))
	switch {
	case n >= 85:
		return n, "Excellent", Mint
	case n >= 70:
		return n, "Good", Cyan
	case n >= 50:
		return n, "Fair", Yellow
	}
	return n, "Poor", Red
}
