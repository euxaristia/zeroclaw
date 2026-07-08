package cli

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strings"

	"zeroclaw/internal/agent"
	"zeroclaw/internal/daemon"
	"zeroclaw/internal/env"
)

// version is the released zeroclaw version. Bump on each release.
const version = "0.1.0"

const usage = `usage: zeroclaw <command>

  up                    start environment + zeroclawd
  down                  stop zeroclawd + environment
  status                daemon and environment state
  chat [conversation]   interactive chat (default conversation: main)
  exec "<prompt>"       one turn in the main conversation
  give <file>           copy a host file into the agent's ~/incoming
  take <path> [dest]    copy a file out of the agent's home
  beat                  fire a heartbeat turn now
  doctor                diagnose setup
  reset-env --force     destroy the environment and the agent's home
  daemon run|stop       run zeroclawd in the foreground / stop it`

// Run dispatches a zeroclaw CLI invocation. Everything except up, doctor, and
// the env file-copy commands is a thin RPC client of zeroclawd.
func Run(args []string) error {
	if len(args) == 0 {
		return errors.New(usage)
	}
	switch args[0] {
	case "help", "--help":
		fmt.Println(usage)
		return nil
	case "version", "--version":
		fmt.Println("zeroclaw", version)
		return nil
	case "up":
		if err := env.Up(); err != nil {
			return err
		}
		return daemon.Launch()
	case "down":
		if err := daemon.Stop(); err != nil {
			fmt.Fprintln(os.Stderr, "warning:", err)
		}
		return env.Down()
	case "status":
		if info, ok := daemon.Running(); ok {
			fmt.Printf("daemon:    running (pid %d, port %d)\n", info.PID, info.Port)
		} else {
			fmt.Println("daemon:    not running")
		}
		return env.Status(os.Stdout)
	case "doctor":
		if err := env.Doctor(os.Stdout); err != nil {
			return err
		}
		_, ok := daemon.Running()
		if ok {
			fmt.Println("ok   zeroclawd responding")
			return nil
		}
		fmt.Println("FAIL zeroclawd responding (zeroclaw up)")
		return nil
	case "exec":
		prompt := strings.TrimSpace(strings.Join(args[1:], " "))
		if prompt == "" {
			return errors.New(`usage: zeroclaw exec "<prompt>"`)
		}
		return execTurn("main", prompt)
	case "chat":
		conversation := "main"
		if len(args) > 1 {
			conversation = args[1]
		}
		return chat(conversation)
	case "beat":
		return daemon.Beat()
	case "give":
		if len(args) != 2 {
			return errors.New("usage: zeroclaw give <file>")
		}
		return env.Give(args[1])
	case "take":
		if len(args) < 2 || len(args) > 3 {
			return errors.New("usage: zeroclaw take <path> [dest]")
		}
		dest := ""
		if len(args) == 3 {
			dest = args[2]
		}
		return env.Take(args[1], dest)
	case "reset-env":
		if len(args) < 2 || args[1] != "--force" {
			return errors.New("reset-env deletes the agent's entire home; rerun as `zeroclaw reset-env --force` if you mean it")
		}
		if err := daemon.Stop(); err != nil {
			fmt.Fprintln(os.Stderr, "warning:", err)
		}
		return env.Reset()
	case "daemon":
		if len(args) > 1 && args[1] == "run" {
			return daemon.RunServer()
		}
		if len(args) > 1 && args[1] == "stop" {
			return daemon.Stop()
		}
		return errors.New("usage: zeroclaw daemon run|stop")
	default:
		return fmt.Errorf("unknown command %q\n%s", args[0], usage)
	}
}

func renderEvent(ev agent.Event) {
	switch ev.Type {
	case "run_start":
		fmt.Fprintf(os.Stderr, "[session %s | %s %s]\n", ev.SessionID, ev.Provider, ev.Model)
	case "text":
		fmt.Print(ev.Delta)
	case "tool_call":
		fmt.Fprintf(os.Stderr, "[tool %s]\n", ev.Name)
	case "error":
		fmt.Fprintf(os.Stderr, "[error %s: %s]\n", ev.Code, ev.Message)
	}
}

func execTurn(conversation, prompt string) error {
	trailer, err := turnStream(conversation, prompt, renderEvent)
	if err != nil {
		return err
	}
	fmt.Println()
	fmt.Fprintf(os.Stderr, "[done status=%s session=%s]\n", trailer.Status, trailer.SessionID)
	return nil
}

func chat(conversation string) error {
	if _, ok := daemon.Running(); !ok {
		return errors.New("zeroclawd is not running; run `zeroclaw up`")
	}
	fmt.Printf("chatting with zeroclaw (conversation %q; /quit to exit)\n", conversation)
	in := bufio.NewScanner(os.Stdin)
	for {
		fmt.Print("you> ")
		if !in.Scan() {
			fmt.Println()
			return in.Err()
		}
		line := strings.TrimSpace(in.Text())
		if line == "" {
			continue
		}
		if line == "/quit" || line == "/exit" {
			return nil
		}
		fmt.Print("zeroclaw> ")
		if _, err := turnStream(conversation, line, renderEvent); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			continue
		}
		fmt.Println()
	}
}
