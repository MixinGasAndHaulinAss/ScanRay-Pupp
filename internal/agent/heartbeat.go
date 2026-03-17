package agent

import (
	"os/exec"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/host"
	"github.com/shirou/gopsutil/v3/mem"
)

var ansiRegex = regexp.MustCompile(`\x1b\[[0-9;]*m`)

type SystemInfo struct {
	OS       string `json:"os"`
	Arch     string `json:"arch"`
	Hostname string `json:"hostname"`
	IP       string `json:"ip"`
}

type HealthMetrics struct {
	CPUPercent  float64 `json:"cpu_percent"`
	MemPercent  float64 `json:"mem_percent"`
	DiskPercent float64 `json:"disk_percent"`
	Uptime      uint64  `json:"uptime_seconds"`
}

func CollectSystemInfo() SystemInfo {
	info := SystemInfo{
		OS:   runtime.GOOS,
		Arch: runtime.GOARCH,
	}
	if h, err := host.Info(); err == nil {
		info.Hostname = h.Hostname
		info.OS = h.Platform + " " + h.PlatformVersion
	}
	return info
}

func CollectHealthMetrics() HealthMetrics {
	m := HealthMetrics{}

	if cpuPct, err := cpu.Percent(time.Second, false); err == nil && len(cpuPct) > 0 {
		m.CPUPercent = cpuPct[0]
	}
	if vm, err := mem.VirtualMemory(); err == nil {
		m.MemPercent = vm.UsedPercent
	}
	if du, err := disk.Usage("/"); err == nil {
		m.DiskPercent = du.UsedPercent
	}
	if h, err := host.Info(); err == nil {
		m.Uptime = h.Uptime
	}
	return m
}

func GetBinaryVersion(binPath string) string {
	out, err := exec.Command(binPath, "-version").CombinedOutput()
	if err != nil {
		return "unknown"
	}
	clean := ansiRegex.ReplaceAllString(string(out), "")
	clean = strings.Map(func(r rune) rune {
		if r == '[' || r == ']' {
			return -1
		}
		return r
	}, clean)
	lines := strings.Split(strings.TrimSpace(clean), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.Contains(strings.ToLower(line), "version") {
			parts := strings.Fields(line)
			for _, p := range parts {
				if strings.HasPrefix(p, "v") && len(p) > 1 && p[1] >= '0' && p[1] <= '9' {
					return p
				}
			}
		}
	}
	if len(lines) > 0 {
		return strings.TrimSpace(lines[0])
	}
	return "unknown"
}
