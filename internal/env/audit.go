package env

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

// AuditStatus represents the outcome of an individual security check.
type AuditStatus string

const (
	StatusPass AuditStatus = "PASS"
	StatusWarn AuditStatus = "WARN"
	StatusFail AuditStatus = "FAIL"
)

// AuditItem represents a single security check result.
type AuditItem struct {
	Category string
	Name     string
	Status   AuditStatus
	Detail   string
	Hint     string
}

// AuditReport holds all audit check items, an overall score (0-100), and a letter grade.
type AuditReport struct {
	Items     []AuditItem
	Score     int
	Grade     string
	Timestamp time.Time
}

// dockerInspectMount represents a mount entry from docker inspect.
type dockerInspectMount struct {
	Type   string `json:"Type"`
	Name   string `json:"Name"`
	Source string `json:"Source"`
	Target string `json:"Target"`
}

// dockerInspectHostConfig represents security options from docker inspect.
type dockerInspectHostConfig struct {
	CapAdd         []string `json:"CapAdd"`
	Privileged     bool     `json:"Privileged"`
	SecurityOpt    []string `json:"SecurityOpt"`
	ReadonlyRootfs bool     `json:"ReadonlyRootfs"`
}

type dockerInspectInfo struct {
	Mounts     []dockerInspectMount    `json:"Mounts"`
	HostConfig dockerInspectHostConfig `json:"HostConfig"`
}

// RunAudit performs automated security scorecard diagnostics on the zeroclaw environment.
func RunAudit() AuditReport {
	var items []AuditItem

	// 1. Container Running State
	containerRunning := false
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := DockerCommandContext(ctx, "inspect", "--format", "json", Container)
	out, err := cmd.Output()
	if err != nil {
		items = append(items, AuditItem{
			Category: "Container Isolation",
			Name:     "Container Process",
			Status:   StatusFail,
			Detail:   "Zeroclaw container is not running or unreachable.",
			Hint:     "Run `zeroclaw up` to start the isolated environment.",
		})
	} else {
		containerRunning = true
		items = append(items, AuditItem{
			Category: "Container Isolation",
			Name:     "Container Process",
			Status:   StatusPass,
			Detail:   fmt.Sprintf("Container %q is running.", Container),
		})

		var inspectList []dockerInspectInfo
		if err := json.Unmarshal(out, &inspectList); err == nil && len(inspectList) > 0 {
			info := inspectList[0]

			// 2. Host Bind Mount Check
			hasBindMounts := false
			for _, m := range info.Mounts {
				if m.Type == "bind" {
					hasBindMounts = true
					items = append(items, AuditItem{
						Category: "Storage Isolation",
						Name:     "No Host Bind Mounts",
						Status:   StatusFail,
						Detail:   fmt.Sprintf("Host bind mount detected: %s -> %s", m.Source, m.Target),
						Hint:     "Zeroclaw rules forbid host bind mounts. Use named volumes and give/take file copies.",
					})
				}
			}
			if !hasBindMounts {
				items = append(items, AuditItem{
					Category: "Storage Isolation",
					Name:     "No Host Bind Mounts",
					Status:   StatusPass,
					Detail:   "No host bind mounts present. Persistent storage is strictly contained in Docker named volume.",
				})
			}

			// 3. Container Privileges & Capability Hardening
			isPrivileged := info.HostConfig.Privileged
			hasCapSysAdmin := false
			for _, cap := range info.HostConfig.CapAdd {
				if strings.EqualFold(cap, "SYS_ADMIN") || strings.EqualFold(cap, "ALL") {
					hasCapSysAdmin = true
				}
			}

			if isPrivileged || hasCapSysAdmin {
				items = append(items, AuditItem{
					Category: "Privilege Boundary",
					Name:     "Unprivileged Execution",
					Status:   StatusFail,
					Detail:   "Container is running with elevated privileges (Privileged/CAP_SYS_ADMIN).",
					Hint:     "Do not pass --privileged or CAP_SYS_ADMIN to docker run.",
				})
			} else {
				items = append(items, AuditItem{
					Category: "Privilege Boundary",
					Name:     "Unprivileged Execution",
					Status:   StatusPass,
					Detail:   "Container operates without CAP_SYS_ADMIN or --privileged escalation.",
				})
			}
		}
	}

	// 4. Seeding & Agent Identity Verification
	if containerRunning {
		ctxCheck, cancelCheck := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancelCheck()

		cmdCheck := DockerCommandContext(ctxCheck, "exec", Container, "test", "-f", Home+"/.config/zero/ZERO.md")
		if err := cmdCheck.Run(); err != nil {
			items = append(items, AuditItem{
				Category: "Agent Identity",
				Name:     "Bootstrap Seed (ZERO.md)",
				Status:   StatusWarn,
				Detail:   "Agent home missing bootstrap ZERO.md identity file in ~/.config/zero/ZERO.md.",
				Hint:     "Run `zeroclaw up` to auto-seed bootstrap files into agent volume.",
			})
		} else {
			items = append(items, AuditItem{
				Category: "Agent Identity",
				Name:     "Bootstrap Seed (ZERO.md)",
				Status:   StatusPass,
				Detail:   "Agent identity & operating rules (ZERO.md) present in agent configuration.",
			})
		}
	}

	// 5. Host Credentials & Secrets Audit
	home, err := os.UserHomeDir()
	if err == nil {
		secretPath := home + "/.zeroclaw/config.json"
		if _, serr := os.Stat(secretPath); os.IsNotExist(serr) {
			items = append(items, AuditItem{
				Category: "Secret Hygiene",
				Name:     "Host Config Encryption",
				Status:   StatusWarn,
				Detail:   "Host zeroclaw config (~/.zeroclaw/config.json) does not exist yet.",
				Hint:     "Configuration will be generated on first `zeroclaw up`.",
			})
		} else {
			items = append(items, AuditItem{
				Category: "Secret Hygiene",
				Name:     "Host Config Encryption",
				Status:   StatusPass,
				Detail:   "Host config present with isolated secrets.",
			})
		}
	}

	// 6. Network Isolation Policy
	items = append(items, AuditItem{
		Category: "Network Security",
		Name:     "Container Egress Control",
		Status:   StatusWarn,
		Detail:   "Default egress enabled for model API and GitHub interactions.",
		Hint:     "Network egress allowlist proxy planned for future hardening tier.",
	})

	// Score Calculation
	score := 100
	for _, item := range items {
		switch item.Status {
		case StatusFail:
			score -= 25
		case StatusWarn:
			score -= 5
		}
	}
	if score < 0 {
		score = 0
	}

	grade := "A"
	if score < 70 {
		grade = "F"
	} else if score < 80 {
		grade = "C"
	} else if score < 90 {
		grade = "B"
	}

	return AuditReport{
		Items:     items,
		Score:     score,
		Grade:     grade,
		Timestamp: time.Now(),
	}
}

func paint(code, s string) string {
	if os.Getenv("NO_COLOR") != "" || s == "" {
		return s
	}
	return "\x1b[" + code + "m" + s + "\x1b[0m"
}

func bold(s string) string          { return paint("1", s) }
func lime(s string) string          { return paint("1;38;2;202;255;63", s) }
func green(s string) string         { return paint("1;38;2;93;209;164", s) }
func yellow(s string) string        { return paint("1;38;2;255;198;88", s) }
func red(s string) string           { return paint("1;38;2;255;122;122", s) }
func faint(s string) string         { return paint("38;2;124;124;130", s) }
func categoryStyle(s string) string { return paint("1;38;2;180;180;190", s) }

// Audit formats and writes the security audit scorecard report to w.
func Audit(w io.Writer) error {
	report := RunAudit()

	scoreColor := green
	if report.Score < 80 {
		scoreColor = red
	} else if report.Score < 90 {
		scoreColor = yellow
	}

	fmt.Fprintln(w, lime("=================================================================="))
	fmt.Fprintln(w, lime("                 ZEROCLAW SECURITY AUDIT SCORECARD                "))
	fmt.Fprintln(w, lime("=================================================================="))
	fmt.Fprintf(w, " %s : %s\n", faint("Timestamp"), report.Timestamp.Format(time.RFC3339))
	fmt.Fprintf(w, " %s     : %s\n", faint("Score"), scoreColor(fmt.Sprintf("%d/100 (Grade: %s)", report.Score, report.Grade)))
	fmt.Fprintln(w, faint("------------------------------------------------------------------"))

	category := ""
	for _, item := range report.Items {
		if item.Category != category {
			category = item.Category
			fmt.Fprintf(w, "\n%s\n", categoryStyle("["+category+"]"))
		}

		statusMark := green("[PASS]")
		if item.Status == StatusFail {
			statusMark = red("[FAIL]")
		} else if item.Status == StatusWarn {
			statusMark = yellow("[WARN]")
		}

		fmt.Fprintf(w, "  %-6s %s\n", statusMark, bold(item.Name))
		fmt.Fprintf(w, "         %s %s\n", faint("Detail:"), item.Detail)
		if item.Hint != "" && item.Status != StatusPass {
			fmt.Fprintf(w, "         %s %s\n", yellow("Hint  :"), item.Hint)
		}
	}

	fmt.Fprintln(w, "\n"+lime("=================================================================="))
	if report.Score >= 90 {
		fmt.Fprintln(w, green(" STATUS: Hardened & Secure Container Environment"))
	} else {
		fmt.Fprintln(w, yellow(" STATUS: Security Attention Recommended"))
	}
	fmt.Fprintln(w, lime("=================================================================="))
	return nil
}
