package backends

import (
	"context"
	"encoding/json"
	"fmt"
	"runtime"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/load"
	"github.com/shirou/gopsutil/v4/mem"
)

type SysinfoBackend struct{}

func init() {
	registerBackend("sysinfo", &SysinfoBackend{})
}

func (s *SysinfoBackend) Actions() []string {
	return []string{"os", "cpu", "memory", "disk", "load", "uptime"}
}

func (s *SysinfoBackend) Run(ctx context.Context, action string, params map[string]string) (*Result, error) {
	switch action {
	case "os":
		return s.osInfo(ctx)
	case "cpu":
		return s.cpuInfo(ctx)
	case "memory":
		return s.memoryInfo(ctx)
	case "disk":
		mount := params["mount"]
		if mount == "" {
			mount = "/"
		}
		return s.diskInfo(ctx, mount)
	case "load":
		return s.loadInfo(ctx)
	case "uptime":
		return s.uptimeInfo(ctx)
	default:
		return nil, fmt.Errorf("sysinfo: unknown action %q", action)
	}
}

func (s *SysinfoBackend) osInfo(ctx context.Context) (*Result, error) {
	info, err := host.InfoWithContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("sysinfo os: %w", err)
	}
	out := map[string]any{
		"hostname":       info.Hostname,
		"os":             info.OS,
		"arch":           runtime.GOARCH,
		"kernel":         info.KernelVersion,
		"uptime_seconds": info.Uptime,
		"boot_time":      info.BootTime,
	}
	return jsonResult(out)
}

func (s *SysinfoBackend) cpuInfo(ctx context.Context) (*Result, error) {
	info, err := cpu.InfoWithContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("sysinfo cpu: %w", err)
	}
	pct, err := cpu.PercentWithContext(ctx, 0, false)
	if err != nil {
		return nil, fmt.Errorf("sysinfo cpu percent: %w", err)
	}
	model := ""
	if len(info) > 0 {
		model = info[0].ModelName
	}
	usagePct := 0.0
	if len(pct) > 0 {
		usagePct = pct[0]
	}
	out := map[string]any{
		"count":         runtime.NumCPU(),
		"model":         model,
		"usage_percent": usagePct,
	}
	return jsonResult(out)
}

func (s *SysinfoBackend) memoryInfo(ctx context.Context) (*Result, error) {
	v, err := mem.VirtualMemoryWithContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("sysinfo memory: %w", err)
	}
	sw, err := mem.SwapMemoryWithContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("sysinfo swap: %w", err)
	}
	out := map[string]any{
		"total":      v.Total,
		"used":       v.Used,
		"free":       v.Free,
		"available":  v.Available,
		"swap_total": sw.Total,
		"swap_used":  sw.Used,
	}
	return jsonResult(out)
}

func (s *SysinfoBackend) diskInfo(ctx context.Context, mount string) (*Result, error) {
	usage, err := disk.UsageWithContext(ctx, mount)
	if err != nil {
		return nil, fmt.Errorf("sysinfo disk %s: %w", mount, err)
	}
	out := map[string]any{
		"mount":         usage.Path,
		"device":        usage.Fstype,
		"total":         usage.Total,
		"used":          usage.Used,
		"free":          usage.Free,
		"usage_percent": usage.UsedPercent,
		"fs_type":       usage.Fstype,
	}
	return jsonResult(out)
}

func (s *SysinfoBackend) loadInfo(ctx context.Context) (*Result, error) {
	avg, err := load.AvgWithContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("sysinfo load: %w", err)
	}
	out := map[string]any{
		"load1":  avg.Load1,
		"load5":  avg.Load5,
		"load15": avg.Load15,
	}
	return jsonResult(out)
}

func (s *SysinfoBackend) uptimeInfo(ctx context.Context) (*Result, error) {
	info, err := host.InfoWithContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("sysinfo uptime: %w", err)
	}
	out := map[string]any{
		"uptime_seconds": info.Uptime,
		"boot_time":      info.BootTime,
	}
	return jsonResult(out)
}

// jsonResult marshals v to indented JSON and wraps it in a Result.
func jsonResult(v any) (*Result, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("marshaling result: %w", err)
	}
	return &Result{Output: truncateOutput(string(data))}, nil
}
