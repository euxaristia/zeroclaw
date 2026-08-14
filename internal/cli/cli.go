package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"zeroclaw/internal/agent"
	"zeroclaw/internal/config"
	"zeroclaw/internal/daemon"
	"zeroclaw/internal/env"
	"zeroclaw/internal/tui"
)

// version is the released zeroclaw version. Bump on each release.
const version = "0.1.0"

const usage = `usage: zeroclaw [-a <agent>] <command>

  list                  list all available zeroclaw agent profiles and status
  up                    start environment + zeroclawd
  down                  stop zeroclawd + environment
  status                daemon and environment state
  chat [conversation]   interactive chat (default conversation: main)
  web                   open the web UI in a browser
  exec "<prompt>"       one turn in the main conversation
  race "<prompt>"       benchmark a prompt across multiple zero sessions
  visualizer [--watch]  live TUI dashboard of container, daemon & security metrics
  audit                 run automated security scorecard diagnostics
  give <file>           copy a host file into the agent's ~/incoming
  take <path> [dest]    copy a file out of the agent's home
  beat                  fire a heartbeat turn now
  doctor                diagnose setup
  auth [sync|login]     manage container zero auth (interactive login or sync host credentials)
  reset-container       remove disposable container (preserves volume & home data)
  reset-env --force     destroy the environment and the agent's home
  help, -h, --help [cmd] show usage or help for a command
  version, -v, --version show zeroclaw version

Options:
  -a, --agent <name>    select agent profile (default: default, env: ZEROCLAW_AGENT)`

func parseAgentAndArgs(args []string) (string, []string, error) {
	agentName := os.Getenv("ZEROCLAW_AGENT")
	if agentName == "" {
		agentName = "default"
	}
	var rest []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "-a" || arg == "--agent" {
			if i+1 >= len(args) || args[i+1] == "" {
				return "", nil, fmt.Errorf("flag %s requires a non-empty argument", arg)
			}
			agentName = args[i+1]
			i++
			continue
		} else if strings.HasPrefix(arg, "--agent=") {
			val := strings.TrimPrefix(arg, "--agent=")
			if val == "" {
				return "", nil, fmt.Errorf("flag --agent requires a non-empty argument")
			}
			agentName = val
			continue
		} else if strings.HasPrefix(arg, "-a=") {
			val := strings.TrimPrefix(arg, "-a=")
			if val == "" {
				return "", nil, fmt.Errorf("flag -a requires a non-empty argument")
			}
			agentName = val
			continue
		}
		rest = append(rest, arg)
	}
	if err := config.ValidateAgentName(agentName); err != nil {
		return "", nil, err
	}
	return agentName, rest, nil
}

// Run dispatches a zeroclaw CLI invocation. Everything except up, doctor, and
// the env file-copy commands is a thin RPC client of zeroclawd.
func Run(args []string) error {
	agentName, args, err := parseAgentAndArgs(args)
	if err != nil {
		return err
	}
	if len(args) == 0 {
		return errors.New(usage)
	}
	switch args[0] {
	case "help", "-h", "--help":
		if len(args) > 1 {
			switch args[1] {
			case "list":
				fmt.Println("usage: zeroclaw list")
				return nil
			case "auth":
				fmt.Println("usage: zeroclaw [-a <agent>] auth [sync|login]")
				return nil
			case "daemon":
				fmt.Println("usage: zeroclaw [-a <agent>] daemon start|run|stop")
				return nil
			case "give":
				fmt.Println("usage: zeroclaw [-a <agent>] give <file>")
				return nil
			case "take":
				fmt.Println("usage: zeroclaw [-a <agent>] take <path> [dest]")
				return nil
			case "exec":
				fmt.Println(`usage: zeroclaw [-a <agent>] exec "<prompt>"`)
				return nil
			case "race":
				fmt.Println(`usage: zeroclaw race "<prompt>"`)
				return nil
			case "reset-container":
				fmt.Println("usage: zeroclaw [-a <agent>] reset-container")
				return nil
			case "reset-env":
				fmt.Println("usage: zeroclaw [-a <agent>] reset-env --force")
				return nil
			}
		}
		fmt.Println(usage)
		return nil
	case "version", "-v", "--version":
		fmt.Println("zeroclaw", version)
		return nil
	case "list":
		return listAgents(os.Stdout)
	case "up":
		if err := env.Up(agentName); err != nil {
			return err
		}
		return daemon.Launch(agentName)
	case "down":
		if err := daemon.Stop(agentName); err != nil {
			fmt.Fprintln(os.Stderr, "warning:", err)
		}
		return env.Down(agentName)
	case "status":
		if info, ok := daemon.Running(agentName); ok {
			fmt.Printf("daemon:    running (pid %d, port %d)\n", info.PID, info.Port)
		} else {
			fmt.Println("daemon:    not running")
		}
		return env.Status(os.Stdout, agentName)
	case "doctor":
		if err := env.Doctor(os.Stdout, agentName); err != nil {
			return err
		}
		_, ok := daemon.Running(agentName)
		if ok {
			fmt.Printf("ok   zeroclawd (%s) responding\n", agentName)
			return nil
		}
		fmt.Printf("FAIL zeroclawd (%s) responding (zeroclaw up)\n", agentName)
		return nil
	case "audit":
		return env.Audit(os.Stdout)
	case "race":
		prompt := strings.TrimSpace(strings.Join(args[1:], " "))
		if prompt == "" {
			return errors.New(`usage: zeroclaw race "<prompt>"`)
		}
		return RunRace(os.Stdout, prompt, []string{"zero", "zero"})
	case "visualizer", "dashboard":
		watch := len(args) > 1 && (args[1] == "--watch" || args[1] == "-w")
		return RunVisualizer(os.Stdout, watch)
	case "exec":
		prompt := strings.TrimSpace(strings.Join(args[1:], " "))
		if prompt == "" {
			return errors.New(`usage: zeroclaw exec "<prompt>"`)
		}
		return execTurn("main", prompt, agentName)
	case "chat":
		conversation := "main"
		if len(args) > 1 {
			conversation = args[1]
		}
		return chat(conversation, agentName)
	case "web":
		return openWeb(agentName)
	case "beat":
		return daemon.Beat(agentName)
	case "give":
		if len(args) != 2 {
			return errors.New("usage: zeroclaw give <file>")
		}
		return env.Give(args[1], agentName)
	case "take":
		if len(args) < 2 || len(args) > 3 {
			return errors.New("usage: zeroclaw take <path> [dest]")
		}
		dest := ""
		if len(args) == 3 {
			dest = args[2]
		}
		return env.Take(args[1], dest, agentName)
	case "reset-container":
		if err := daemon.Stop(agentName); err != nil {
			fmt.Fprintln(os.Stderr, "warning:", err)
		}
		return env.ResetContainer(agentName)
	case "reset-env":
		if len(args) < 2 || args[1] != "--force" {
			return errors.New("reset-env deletes the agent's entire home; rerun as `zeroclaw reset-env --force` if you mean it")
		}
		if err := daemon.Stop(agentName); err != nil {
			fmt.Fprintln(os.Stderr, "warning:", err)
		}
		return env.Reset(agentName)
	case "auth":
		return handleAuth(args, agentName)
	case "daemon":
		if len(args) > 1 && args[1] == "start" {
			return daemon.Launch(agentName)
		}
		if len(args) > 1 && args[1] == "run" {
			return daemon.RunServer(agentName)
		}
		if len(args) > 1 && args[1] == "stop" {
			return daemon.Stop(agentName)
		}
		return errors.New("usage: zeroclaw daemon start|run|stop")
	default:
		return fmt.Errorf("unknown command %q\n%s", args[0], usage)
	}
}

func listAgents(w io.Writer) error {
	summaries, err := env.DiscoverAgents()
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "%-12s %-20s %-12s %-22s %-12s %-20s\n", "AGENT", "CONTAINER", "STATUS", "VOLUME", "VOL STATUS", "DAEMON")
	fmt.Fprintln(w, strings.Repeat("-", 104))
	for _, s := range summaries {
		daemonStatus := "not running"
		if info, ok := daemon.Running(s.Name); ok {
			daemonStatus = fmt.Sprintf("running (pid %d)", info.PID)
		}
		volStatus := "absent"
		if s.VolumePresent {
			volStatus = "present"
		}
		fmt.Fprintf(w, "%-12s %-20s %-12s %-22s %-12s %-20s\n",
			s.Name,
			s.Container,
			s.ContainerStatus,
			s.Volume,
			volStatus,
			daemonStatus,
		)
	}
	return nil
}

// renderer prints one turn's driver events in zero's visual language: muted
// reasoning, accent-marked tool heads with faint result summaries, plain ink
// for the reply. It tracks the previous event so blocks of different kinds
// get a separating blank line and a mid-line reasoning stream is terminated
// before anything else prints. Text deltas stay on stdout, decorations on
// stderr, as before.
type renderer struct {
	midReasoning bool
	last         string
	textBuf      string
}

func (r *renderer) flushText() {
	if r.textBuf == "" {
		return
	}
	fmt.Print(FormatMarkdown(r.textBuf))
	r.textBuf = ""
}

func (r *renderer) event(ev agent.Event) {
	if r.midReasoning && ev.Type != "reasoning" {
		r.midReasoning = false
		fmt.Fprintln(os.Stderr)
	}
	if r.last == "text" && ev.Type != "text" {
		r.flushText()
	}
	switch ev.Type {
	case "run_start":
		fmt.Fprintf(os.Stderr, "%s\n\n", faint(fmt.Sprintf("session %s · %s %s", ev.SessionID, ev.Provider, ev.Model)))
	case "reasoning":
		r.midReasoning = true
		fmt.Fprint(os.Stderr, muted(ev.Delta))
	case "text":
		if r.last == "tool_call" || r.last == "tool_result" {
			fmt.Fprintln(os.Stderr)
		}
		r.textBuf += ev.Delta
		for {
			idx := strings.IndexByte(r.textBuf, '\n')
			if idx == -1 {
				break
			}
			line := r.textBuf[:idx+1]
			fmt.Print(FormatMarkdown(line))
			r.textBuf = r.textBuf[idx+1:]
		}
	case "tool_call":
		if r.last == "text" {
			fmt.Println()
		}
		fmt.Fprintf(os.Stderr, "%s %s\n", accent("⏺"), boldInk(ev.Name))
	case "tool_result":
		if ev.Display.Summary != "" {
			fmt.Fprintf(os.Stderr, "  %s\n", faint(ev.Display.Summary))
		}
	case "error":
		if r.last == "text" {
			fmt.Println()
		}
		fmt.Fprintf(os.Stderr, "%s %s\n", red("✗"), red(ev.Code+": "+ev.Message))
	}
	r.last = ev.Type
}

func execTurn(conversation, prompt string, agentName ...string) error {
	r := &renderer{}
	trailer, err := turnStream(conversation, prompt, r.event, agentName...)
	r.flushText()
	if err != nil {
		return err
	}
	fmt.Println()
	mark := green("✓ " + trailer.Status)
	if trailer.Error != "" {
		mark = red("✗ " + trailer.Status + " " + trailer.Error)
	}
	fmt.Fprintf(os.Stderr, "%s %s\n", mark, faint("session "+trailer.SessionID))
	return nil
}

func chat(conversation string, agentName ...string) error {
	agent := "default"
	if len(agentName) > 0 && agentName[0] != "" {
		agent = agentName[0]
	}
	info, ok := daemon.Running(agent)
	if !ok {
		return fmt.Errorf("zeroclawd is not running for agent %s; run `zeroclaw up`", agent)
	}
	code := tui.Run(context.Background(), tui.Options{
		Conversation: conversation,
		Port:         info.Port,
		Token:        info.Token,
	})
	if code != 0 {
		return fmt.Errorf("chat exited with code %d", code)
	}
	return nil
}

// openWeb launches the web UI: the token travels once in the URL, the page
// stores it client-side, and every request after that goes through the same
// bearer-auth RPC plane as the CLI and Telegram.
func openWeb(agentName string) error {
	info, ok := daemon.Running(agentName)
	if !ok {
		return fmt.Errorf("zeroclawd is not running for agent %s; run `zeroclaw up`", agentName)
	}
	url := fmt.Sprintf("http://127.0.0.1:%d/?token=%s", info.Port, info.Token)
	fmt.Println(url)
	if err := openBrowser(url); err != nil {
		fmt.Fprintln(os.Stderr, "warning: could not open a browser automatically:", err)
	}
	return nil
}
