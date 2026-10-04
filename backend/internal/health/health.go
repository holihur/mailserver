// Package health 采集系统健康指标（CPU / 内存 / 磁盘）并按 80% / 90% 阈值产生告警。
// 仅依赖 Linux /proc 与 statfs；不可用时指标为 0、不产生告警。
package health

import (
	"bufio"
	"os"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// 阈值（百分比）。
const (
	WarnPct     = 80.0
	CriticalPct = 90.0
)

type Alert struct {
	Metric  string  `json:"metric"` // cpu | memory | disk
	Level   string  `json:"level"`  // warn | critical
	Value   float64 `json:"value"`
	Message string  `json:"message"`
}

type Snapshot struct {
	CPU       float64   `json:"cpu"`
	Memory    float64   `json:"memory"`
	Disk      float64   `json:"disk"`
	MemTotal  uint64    `json:"mem_total"`
	MemUsed   uint64    `json:"mem_used"`
	DiskTotal uint64    `json:"disk_total"`
	DiskUsed  uint64    `json:"disk_used"`
	UpdatedAt time.Time `json:"updated_at"`
	Alerts    []Alert   `json:"alerts"`
}

type cpuTimes struct {
	busy, total uint64
}

type Collector struct {
	diskPath string
	mu       sync.RWMutex
	cur      Snapshot
	prev     cpuTimes
	prevAt   time.Time
}

func New(diskPath string) *Collector {
	if strings.TrimSpace(diskPath) == "" {
		diskPath = "/"
	}
	c := &Collector{diskPath: diskPath}
	c.cur = c.sample()
	return c
}

// Start 周期采样（默认 20s）。
func (c *Collector) Start(interval time.Duration) {
	if interval <= 0 {
		interval = 20 * time.Second
	}
	go func() {
		for range time.Tick(interval) {
			s := c.sample()
			c.mu.Lock()
			c.cur = s
			c.mu.Unlock()
		}
	}()
}

// Current 返回最近一次快照。
func (c *Collector) Current() Snapshot {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.cur
}

func (c *Collector) sample() Snapshot {
	s := Snapshot{UpdatedAt: time.Now()}
	s.Memory, s.MemTotal, s.MemUsed = readMemory()
	s.Disk, s.DiskTotal, s.DiskUsed = readDisk(c.diskPath)
	s.CPU = c.readCPU()
	s.Alerts = evaluate(s)
	return s
}

func evaluate(s Snapshot) []Alert {
	var out []Alert
	add := func(metric string, v float64) {
		switch {
		case v >= CriticalPct:
			out = append(out, Alert{Metric: metric, Level: "critical", Value: v,
				Message: metric + " 使用率过高"})
		case v >= WarnPct:
			out = append(out, Alert{Metric: metric, Level: "warn", Value: v,
				Message: metric + " 使用率偏高"})
		}
	}
	add("cpu", s.CPU)
	add("memory", s.Memory)
	add("disk", s.Disk)
	return out
}

// readCPU 通过 /proc/stat 的两次采样差计算 CPU 使用率。
func (c *Collector) readCPU() float64 {
	f, err := os.Open("/proc/stat")
	if err != nil {
		return 0
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	if !sc.Scan() {
		return 0
	}
	fields := strings.Fields(sc.Text()) // cpu user nice system idle iowait irq softirq steal ...
	if len(fields) < 5 || fields[0] != "cpu" {
		return 0
	}
	var vals []uint64
	for _, f := range fields[1:] {
		n, _ := strconv.ParseUint(f, 10, 64)
		vals = append(vals, n)
	}
	var total uint64
	for _, v := range vals {
		total += v
	}
	idle := vals[3]
	if len(vals) > 4 {
		idle += vals[4] // iowait 计入空闲
	}
	busy := total - idle

	now := time.Now()
	prev := c.prev
	c.prev = cpuTimes{busy: busy, total: total}
	c.prevAt = now
	if prev.total == 0 || total <= prev.total {
		return 0
	}
	dTotal := total - prev.total
	dBusy := busy - prev.busy
	if dTotal == 0 {
		return 0
	}
	return float64(dBusy) / float64(dTotal) * 100
}

// readMemory 读取 /proc/meminfo，返回使用率与总量/已用（字节）。
func readMemory() (pct float64, total, used uint64) {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0, 0, 0
	}
	defer f.Close()
	var memTotal, memAvail uint64
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		switch {
		case strings.HasPrefix(line, "MemTotal:"):
			memTotal = parseKB(line)
		case strings.HasPrefix(line, "MemAvailable:"):
			memAvail = parseKB(line)
		}
	}
	if memTotal == 0 {
		return 0, 0, 0
	}
	usedKB := memTotal - memAvail
	return float64(usedKB) / float64(memTotal) * 100, memTotal * 1024, usedKB * 1024
}

func parseKB(line string) uint64 {
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return 0
	}
	n, _ := strconv.ParseUint(fields[1], 10, 64)
	return n
}

// readDisk 通过 statfs 计算磁盘使用率与总量/已用（字节）。
func readDisk(path string) (pct float64, total, used uint64) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return 0, 0, 0
	}
	bsize := uint64(st.Bsize)
	total = st.Blocks * bsize
	free := st.Bfree * bsize
	if total == 0 {
		return 0, 0, 0
	}
	used = total - free
	return float64(used) / float64(total) * 100, total, used
}
