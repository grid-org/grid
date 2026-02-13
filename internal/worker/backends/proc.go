package backends

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/shirou/gopsutil/v4/process"
)

const maxCmdlineLen = 200

type ProcBackend struct{}

func init() {
	registerBackend("proc", &ProcBackend{})
}

func (p *ProcBackend) Actions() []string {
	return []string{"list", "top", "find", "count"}
}

func (p *ProcBackend) Run(ctx context.Context, action string, params map[string]string) (*Result, error) {
	switch action {
	case "list":
		return p.list(ctx, params)
	case "top":
		return p.top(ctx, params)
	case "find":
		return p.findAction(ctx, params)
	case "count":
		return p.count(ctx)
	default:
		return nil, fmt.Errorf("proc: unknown action %q", action)
	}
}

type procInfo struct {
	PID    int32   `json:"pid"`
	Name   string  `json:"name"`
	User   string  `json:"user"`
	CPU    float64 `json:"cpu_percent"`
	Mem    float32 `json:"mem_percent"`
	Status string  `json:"status"`
}

func getProcessInfo(ctx context.Context, proc *process.Process) procInfo {
	name, _ := proc.NameWithContext(ctx)
	user, _ := proc.UsernameWithContext(ctx)
	cpuPct, _ := proc.CPUPercentWithContext(ctx)
	memPct, _ := proc.MemoryPercentWithContext(ctx)
	statuses, _ := proc.StatusWithContext(ctx)
	status := ""
	if len(statuses) > 0 {
		status = statuses[0]
	}

	return procInfo{
		PID:    proc.Pid,
		Name:   name,
		User:   user,
		CPU:    cpuPct,
		Mem:    memPct,
		Status: status,
	}
}

func (p *ProcBackend) list(ctx context.Context, params map[string]string) (*Result, error) {
	procs, err := process.ProcessesWithContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("proc list: %w", err)
	}

	filterUser := params["user"]
	filterName := params["name"]

	var result []procInfo
	for _, proc := range procs {
		info := getProcessInfo(ctx, proc)

		if filterUser != "" && info.User != filterUser {
			continue
		}
		if filterName != "" && !strings.Contains(strings.ToLower(info.Name), strings.ToLower(filterName)) {
			continue
		}

		result = append(result, info)
		if len(result) >= 500 {
			break
		}
	}

	data, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("proc list: %w", err)
	}
	return &Result{Output: truncateOutput(string(data))}, nil
}

func (p *ProcBackend) top(ctx context.Context, params map[string]string) (*Result, error) {
	count := 10
	if c := params["count"]; c != "" {
		fmt.Sscanf(c, "%d", &count)
		if count < 1 {
			count = 1
		}
		if count > 100 {
			count = 100
		}
	}

	procs, err := process.ProcessesWithContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("proc top: %w", err)
	}

	var infos []procInfo
	for _, proc := range procs {
		infos = append(infos, getProcessInfo(ctx, proc))
	}

	sort.Slice(infos, func(i, j int) bool {
		return infos[i].CPU > infos[j].CPU
	})

	if len(infos) > count {
		infos = infos[:count]
	}

	data, err := json.Marshal(infos)
	if err != nil {
		return nil, fmt.Errorf("proc top: %w", err)
	}
	return &Result{Output: truncateOutput(string(data))}, nil
}

func (p *ProcBackend) findAction(ctx context.Context, params map[string]string) (*Result, error) {
	name, err := requireParam("proc", "find", "name", params)
	if err != nil {
		return nil, err
	}

	procs, err := process.ProcessesWithContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("proc find: %w", err)
	}

	type findResult struct {
		PID     int32  `json:"pid"`
		Name    string `json:"name"`
		User    string `json:"user"`
		Cmdline string `json:"cmdline"`
	}

	var result []findResult
	nameLower := strings.ToLower(name)
	for _, proc := range procs {
		pName, _ := proc.NameWithContext(ctx)
		if !strings.Contains(strings.ToLower(pName), nameLower) {
			continue
		}

		user, _ := proc.UsernameWithContext(ctx)
		cmdline, _ := proc.CmdlineWithContext(ctx)
		if len(cmdline) > maxCmdlineLen {
			cmdline = cmdline[:maxCmdlineLen] + "..."
		}

		result = append(result, findResult{
			PID:     proc.Pid,
			Name:    pName,
			User:    user,
			Cmdline: cmdline,
		})
	}

	data, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("proc find: %w", err)
	}
	return &Result{Output: truncateOutput(string(data))}, nil
}

func (p *ProcBackend) count(ctx context.Context) (*Result, error) {
	procs, err := process.ProcessesWithContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("proc count: %w", err)
	}

	total := len(procs)
	running, sleeping, zombie := 0, 0, 0
	for _, proc := range procs {
		statuses, _ := proc.StatusWithContext(ctx)
		if len(statuses) == 0 {
			continue
		}
		switch statuses[0] {
		case process.Running:
			running++
		case process.Sleep:
			sleeping++
		case process.Zombie:
			zombie++
		}
	}

	out := map[string]any{
		"total":    total,
		"running":  running,
		"sleeping": sleeping,
		"zombie":   zombie,
	}
	return jsonResult(out)
}
