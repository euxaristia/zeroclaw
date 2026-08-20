package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"zeroclaw/internal/agent"
	"zeroclaw/internal/daemon"
	"zeroclaw/internal/env"
)

type dockerStats struct {
	Container string `json:"Container"`
	Name      string `json:"Name"`
	CPUPerc   string `json:"CPUPerc"`
	MemUsage  string `json:"MemUsage"`
	MemPerc   string `json:"MemPerc"`
	NetIO     string `json:"NetIO"`
	BlockIO   string `json:"BlockIO"`
	PIDs      string `json:"PIDs"`
}

func fetchContainerStats() dockerStats {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	cmd := env.DockerCommandContext(ctx, "stats", "--no-stream", "--format", "{{json .}}", env.Container)
	out, err := cmd.Output()
	if err != nil {
		return dockerStats{
			Container: env.Container,
			CPUPerc:   "N/A",
			MemUsage:  "N/A",
			MemPerc:   "N/A",
		}
	}

	var stats dockerStats
	_ = json.Unmarshal(out, &stats)
	return stats
}

func renderGauge(percStr string, width int) string {
	var perc float64
	fmt.Sscanf(strings.TrimSuffix(percStr, "%"), "%f", &perc)
	if perc > 100 {
		perc = 100
	}
	filled := int((perc / 100.0) * float64(width))
	if filled > width {
		filled = width
	}
	bar := strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
	if perc > 85 {
		return red(bar)
	}
	return green(bar)
}

// RunVisualizer renders an interactive or live TUI dashboard displaying agent, container, and backend health.
func RunVisualizer(w io.Writer, watch bool) error {
	for {
		if watch {
			// Clear screen ansi sequence for live update mode
			fmt.Fprint(w, "\033[H\033[2J")
		}

		fmt.Fprintf(w, "┌────────────────────────────────────────────────────────────────────────┐\n")
		fmt.Fprintf(w, "│ %s              │\n", badge(" ZEROCLAW SYSTEM DASHBOARD ")+" "+boldInk("Live Agent Telemetry"))
		fmt.Fprintf(w, "└────────────────────────────────────────────────────────────────────────┘\n\n")

		// 1. Daemon Status Card
		info, daemonRunning := daemon.Running()
		daemonMark := red("OFFLINE")
		if daemonRunning {
			daemonMark = green(fmt.Sprintf("ONLINE (PID %d, Port %d)", info.PID, info.Port))
		}

		fmt.Fprintln(w, boldInk("┌─ [DAEMON STATUS]"))
		fmt.Fprintf(w, "│ State      : %s\n", daemonMark)

		// Fetch Conversations
		convCount := 0
		if daemonRunning {
			if conversations, err := getConversations(info); err == nil {
				convCount = len(conversations)
			}
		}
		fmt.Fprintf(w, "│ Active Convs: %d\n", convCount)
		fmt.Fprintln(w, "└─────────────────────────────────────────────────────────")

		// 2. Container & Hardware Resource Gauges
		cStats := fetchContainerStats()
		fmt.Fprintln(w, "\n"+boldInk("┌─ [CONTAINER RESOURCES] ("+env.Container+")"))
		fmt.Fprintf(w, "│ CPU Usage  : %-7s [%s]\n", cStats.CPUPerc, renderGauge(cStats.CPUPerc, 25))
		fmt.Fprintf(w, "│ Memory     : %-15s [%s]\n", cStats.MemUsage+" ("+cStats.MemPerc+")", renderGauge(cStats.MemPerc, 25))
		if cStats.NetIO != "" {
			fmt.Fprintf(w, "│ Network I/O: %s\n", cStats.NetIO)
		}
		fmt.Fprintln(w, "└─────────────────────────────────────────────────────────")

		// 3. Execution Backend Health
		fmt.Fprintln(w, "\n"+boldInk("┌─ [EXECUTION BACKENDS]"))
		healthResults := agent.Doctor(env.Container)
		for _, h := range healthResults {
			mark := green("✓ OK  ")
			if !h.OK {
				mark = red("✗ FAIL")
			}
			fmt.Fprintf(w, "│ %s %s\n", mark, h.Name)
			if !h.OK && h.Hint != "" {
				fmt.Fprintf(w, "│       %s\n", faint("Hint: "+h.Hint))
			}
		}
		fmt.Fprintln(w, "└─────────────────────────────────────────────────────────")

		// 4. Security Audit Scorecard Summary
		auditReport := env.RunAudit()
		scoreMark := green(fmt.Sprintf("%d/100 (Grade %s)", auditReport.Score, auditReport.Grade))
		if auditReport.Score < 80 {
			scoreMark = red(fmt.Sprintf("%d/100 (Grade %s)", auditReport.Score, auditReport.Grade))
		}
		fmt.Fprintln(w, "\n"+boldInk("┌─ [SECURITY ISOLATION]"))
		fmt.Fprintf(w, "│ Security Score: %s\n", scoreMark)
		fmt.Fprintf(w, "│ Storage Mode  : %s\n", faint("Docker Named Volume (/home/zeroclaw)"))
		fmt.Fprintln(w, "└─────────────────────────────────────────────────────────")

		if !watch {
			break
		}

		time.Sleep(2 * time.Second)
	}

	return nil
}

func getConversations(info daemon.Info) (map[string]string, error) {
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("http://127.0.0.1:%d/conversations", info.Port), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+info.Token)
	resp, err := streamClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var convs map[string]string
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&convs); err != nil {
		return nil, err
	}
	return convs, nil
}
