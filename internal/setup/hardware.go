package setup

import (
	"fmt"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
)

// HardwareInfo represents detected system hardware
type HardwareInfo struct {
	OS         string
	Arch       string
	CPUModel   string
	CPUCores   int
	TotalRAMGB int
	FreeRAMGB  int
	DiskFreeGB int
}

// DetectHardware detects system hardware specs
func DetectHardware() (*HardwareInfo, error) {
	info := &HardwareInfo{
		OS:       runtime.GOOS,
		Arch:     runtime.GOARCH,
		CPUCores: runtime.NumCPU(),
	}

	// Detect RAM
	switch runtime.GOOS {
	case "linux":
		info.TotalRAMGB = detectRAMLinux()
		info.FreeRAMGB = detectFreeRAMLinux()
		info.CPUModel = detectCPULinux()
		info.DiskFreeGB = detectDiskLinux()
	case "darwin":
		info.TotalRAMGB = detectRAMMac()
		info.FreeRAMGB = detectFreeRAMMac()
		info.CPUModel = detectCPUMac()
		info.DiskFreeGB = detectDiskMac()
	default:
		// Windows or unknown - use estimates
		info.TotalRAMGB = 8 // Assume 8GB
		info.FreeRAMGB = 4
		info.CPUModel = "Unknown"
		info.DiskFreeGB = 100
	}

	return info, nil
}

// CanRunLocalLLM checks if system meets minimum requirements for local LLM
func (h *HardwareInfo) CanRunLocalLLM() bool {
	// Minimum: 8GB total RAM, 4GB free, 10GB disk space
	return h.TotalRAMGB >= 8 && h.FreeRAMGB >= 4 && h.DiskFreeGB >= 10
}

// RecommendedModel returns recommended model based on available RAM
func (h *HardwareInfo) RecommendedModel() string {
	if h.TotalRAMGB >= 16 {
		return "llama3 (8B)" // Full model
	} else if h.TotalRAMGB >= 8 {
		return "llama3.2 (3B)" // Smaller model
	} else {
		return "Remote LLM recommended" // Not enough RAM
	}
}

// Format returns human-readable hardware summary
func (h *HardwareInfo) Format() string {
	return fmt.Sprintf("%s/%s • %s • %d cores • %dGB RAM (%dGB free) • %dGB disk free",
		h.OS, h.Arch, h.CPUModel, h.CPUCores, h.TotalRAMGB, h.FreeRAMGB, h.DiskFreeGB)
}

// Linux detection
func detectRAMLinux() int {
	cmd := exec.Command("bash", "-c", "grep MemTotal /proc/meminfo | awk '{print $2}'")
	output, err := cmd.Output()
	if err != nil {
		return 0
	}
	kb, _ := strconv.Atoi(strings.TrimSpace(string(output)))
	return kb / 1024 / 1024 // KB to GB
}

func detectFreeRAMLinux() int {
	cmd := exec.Command("bash", "-c", "grep MemAvailable /proc/meminfo | awk '{print $2}'")
	output, err := cmd.Output()
	if err != nil {
		return 0
	}
	kb, _ := strconv.Atoi(strings.TrimSpace(string(output)))
	return kb / 1024 / 1024
}

func detectCPULinux() string {
	cmd := exec.Command("bash", "-c", "grep 'model name' /proc/cpuinfo | head -1 | cut -d: -f2")
	output, err := cmd.Output()
	if err != nil {
		return "Unknown CPU"
	}
	cpu := strings.TrimSpace(string(output))
	// Shorten long CPU names
	if len(cpu) > 40 {
		cpu = cpu[:37] + "..."
	}
	return cpu
}

func detectDiskLinux() int {
	cmd := exec.Command("bash", "-c", "df -BG ~ | tail -1 | awk '{print $4}'")
	output, err := cmd.Output()
	if err != nil {
		return 0
	}
	gb := strings.TrimSuffix(strings.TrimSpace(string(output)), "G")
	val, _ := strconv.Atoi(gb)
	return val
}

// macOS detection
func detectRAMMac() int {
	cmd := exec.Command("sysctl", "-n", "hw.memsize")
	output, err := cmd.Output()
	if err != nil {
		return 0
	}
	bytes, _ := strconv.ParseInt(strings.TrimSpace(string(output)), 10, 64)
	return int(bytes / 1024 / 1024 / 1024)
}

func detectFreeRAMMac() int {
	cmd := exec.Command("bash", "-c", "vm_stat | grep 'Pages free' | awk '{print $3}' | tr -d '.'")
	output, err := cmd.Output()
	if err != nil {
		return 0
	}
	pages, _ := strconv.Atoi(strings.TrimSpace(string(output)))
	return (pages * 4096) / 1024 / 1024 / 1024 // Pages to GB (4KB pages)
}

func detectCPUMac() string {
	cmd := exec.Command("sysctl", "-n", "machdep.cpu.brand_string")
	output, err := cmd.Output()
	if err != nil {
		return "Unknown CPU"
	}
	cpu := strings.TrimSpace(string(output))
	if len(cpu) > 40 {
		cpu = cpu[:37] + "..."
	}
	return cpu
}

func detectDiskMac() int {
	cmd := exec.Command("bash", "-c", "df -g ~ | tail -1 | awk '{print $4}'")
	output, err := cmd.Output()
	if err != nil {
		return 0
	}
	val, _ := strconv.Atoi(strings.TrimSpace(string(output)))
	return val
}
