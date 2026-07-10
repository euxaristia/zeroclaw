# zeroclaw: agent guidelines

This file guides agents (and humans) working on **zeroclaw**, a prototype
autonomous personal agent in the spirit of OpenClaw and Hermes Agent, with
**zero** (the Go CLI coding harness at `../zero`) as the brain and tool
runtime. Zeroclaw is not a fork of zero: it is a host-side orchestrator that
gives zero a body: an isolated persistent environment, an always-on loop,
channels, memory, and schedules.

Read this whole file before writing code. Where this file says "decide later,"
do not decide it now.

## What zeroclaw is

- An always-on agent service (`zeroclawd`) that runs on the host (this Windows PC
  now, a Mac later) with no terminal attached. The `zeroclaw` CLI is a thin
  client that talks to the daemon over localhost HTTP; closing every terminal
  changes nothing for the agent. Heartbeats, schedules, and chat channels keep
  running.
- The agent is independent of the zero CLI too: zeroclaw owns the agent's
  identity, memory, sessions-to-conversations mapping, channels, and schedules.
  Zero is consumed behind a small harness-driver interface as the first (and for
  the prototype, only) execution backend.
- The agent itself lives inside its own **completely isolated working environment**
  (its own home directory, its own packages, its own filesystem). It never touches
  the host filesystem.
- You talk to it over channels (CLI chat and Telegram).
- It wakes itself up on schedules and heartbeats, works unattended, and keeps
  durable memory and skills across sessions, hermes-style.
- Every model turn and tool execution is delegated to `zero exec` running inside
  the isolated environment, speaking stream-JSON
  (see `../zero/docs/STREAM_JSON_PROTOCOL.md`).

## What zeroclaw is not (non-goals for the prototype)

- Not a new coding agent loop. Zero owns the agentic loop, tools, providers,
  sessions, skills, and permissions. Zeroclaw never reimplements those.
- Not multi-user, not a hosted service, no web UI.
- Not voice, not WhatsApp/Signal/Slack. One chat channel max in the prototype.
- No training/self-improvement pipeline beyond file-based memory and zero skills.

## What we borrow from each parent

| From | We take |
|---|---|
| zero | agent loop, tools, providers, stream-JSON exec, sessions (resume/fork), skills, cron primitives, the inner sandbox |
| Hermes Agent | the "terminal backend" abstraction (local/docker/ssh), persistent agent home, learning loop (memory nudges, skill creation), gateway that serves multiple channels from one process |
| OpenClaw | single always-on gateway daemon, heartbeat prompt pattern, bootstrap identity files in the agent home, lazy session-per-conversation model |

## Isolation design (the big decision, already made)

Layered: **container as the hard boundary, zero's sandbox as the inner guard.**

### Why not zero's sandbox alone

Zero's sandbox (seatbelt / namespaces+Landlock+seccomp / Windows restricted
tokens, see `../zero/internal/sandbox/`) is per-command policy enforcement **on
the host**. It scopes writes and gates network, but the agent still lives on the
real filesystem, host reads are broadly allowed, and one policy bug is host
compromise. For an unattended autonomous agent the host boundary must be a hard
one. Zero's sandbox is excellent as the inner layer, insufficient as the outer one.

### Tier 1 (default): container environment

- One long-lived container per agent, image `zeroclaw-env`, created and supervised
  by the zeroclaw daemon via the `docker` CLI (Docker Desktop on Windows/macOS,
  docker or podman on Linux). No Docker SDK dependency; shell out.
- Image: Debian base + zero binary + git, gh (latest GitHub release, not the
  Debian package), ripgrep, curl, jq, python3, Go (latest, from go.dev), rustup
  (rustc/cargo, stable minimal profile), gcc. Build from `env/Dockerfile` in
  this repo, and keep `env/bootstrap/ZEROCLAW.md`'s "Your tools" list in sync
  with what the image installs.
- A named volume mounted at `/home/zeroclaw` is the agent's entire persistent
  world: zero config and sessions, workspace, memory, skills. Container is
  disposable; the volume is the agent.
- **No host bind mounts. Ever.** File exchange with the host goes through an
  explicit `zeroclaw give <file>` / `zeroclaw take <path>` copy command
  (docker cp) so every transfer is a deliberate act.
- Network: the container gets normal egress (it needs the model API and GitHub).
  Host isolation is the goal of tier 1, not network isolation. An egress
  allowlist proxy is a later hardening item, listed under open questions.
- Secrets: on first `zeroclaw up`, the host zero provider config and encrypted
  credential store (`config.json`, `credentials.enc`, `credentials.enc.secret`)
  are copied once into the agent's volume (`adoptZeroAuth`), so the agent talks
  to the same provider as the host zero install. Never baked into the image;
  never overwritten on later ups.

### Tier 2: zero's sandbox inside the container

- Zero's permission mode inside the container is full-auto (the container is
  what protects the host; prompting has no user to answer it).
- RESOLVED (2026-07-07): native sandbox nesting does not work under Docker's
  default security profile. Unprivileged user namespaces are blocked, so
  bubblewrap was dropped from the image. Decision: do not weaken tier 1 (no
  CAP_SYS_ADMIN, no seccomp=unconfined) to enable tier 2 native wrapping. Zero
  still enforces write scoping at the tool layer inside the container; that is
  the accepted tier 2 for the prototype.
- RESOLVED (2026-07-09): zero's inner network gate is opened on purpose. Zero
  classifies network-touching shell commands and denies them by default, which
  only strands the agent in here (the container is the boundary and ships gh,
  git, and curl precisely for network work). Seeding fills in
  `sandbox.network=allow` in the adopted config when the setting is absent
  (`allowSandboxNetwork` in `internal/env/docker.go`); an operator's deliberate
  `deny` is kept. Do not regress this to deny.

### Tier 3 (fallback, no Docker present): zero sandbox on host

- Agent home pinned to `~/.zeroclaw/home`, zero sandbox `enforce`, writes limited
  to that home, out-of-workspace deny, permission mode auto (not unsafe).
- `zeroclaw doctor` and startup output must say plainly: "running without hard
  isolation." This tier exists so the prototype runs anywhere, not because it
  meets the isolation goal. Not yet implemented (see status).

## Architecture

```
host                                    container (zeroclaw-env)
+---------------------------+           +----------------------------+
| zeroclaw daemon (Go)      |           |  /home/zeroclaw  (volume)  |
|  - gateway RPC api        |  docker   |   ZERO.md      (identity)  |
|  - channel: Telegram      |  exec     |   MEMORY.md + memory/      |
|  - scheduler (cron+beat)  | --------> |   workspace/               |
|  - session router         |  stream-  |   .config/zero/  sessions, |
|  - harness driver: zero   |  JSON     |     skills, providers      |
|  - env lifecycle (docker) |           |  zero exec (inner sandbox) |
|  - host config + secrets  |           +----------------------------+
+---------------------------+
        ^  localhost HTTP + bearer token
        |
+---------------------------+
| zeroclaw CLI (thin client)|   chat / exec / status / give / take ...
+---------------------------+   CLI chat is just another gateway client
```

- **Daemon** (`zeroclawd`, started by `zeroclaw up` or `zeroclaw daemon start`):
  a standalone service that survives terminal close and client disconnects.
  Supervises the container, owns schedules, hosts channels, and exposes a local
  control plane (localhost HTTP with a bearer token, advertised via an info
  file). Single process, hermes-gateway-style. Autostart at login (Task
  Scheduler / launchd) is a hardening item.
- **CLI as thin client**: every `zeroclaw` command except `up`, `doctor`, and the
  env file-copy commands is an RPC client. `zeroclaw chat` is one more channel
  attached to the gateway, a peer of Telegram, with no privileged path into
  agent internals. If the daemon is not running, clients say so and suggest
  `zeroclaw up`; nothing falls back to in-process execution.
- **Harness driver**: `internal/agent/driver.go` defines the minimal interface
  zeroclaw needs from an execution backend (start or resume a session, send a
  turn, receive an event stream). `zerodriver.go` implements it over `zero exec`
  stream-JSON. All zero-specific knowledge lives inside the driver; nothing
  outside it may shell out to zero or parse its output. The agent's durable
  state (identity, memory, conversation-to-session map, schedules, channel
  config) belongs to zeroclaw, so the harness is swappable.
- **Turn execution** (inside the zero driver): each conversation maps to a zero
  session inside the container. A turn is
  `docker exec -i zeroclaw zero exec --resume <id> --input-format stream-json --output-format stream-json`.
  Attended turns (chat, exec, telegram) pass `--no-completion-gate` so an
  honest handoff back to the operator is not downgraded to INCOMPLETE;
  unattended turns (heartbeats, schedules) keep the gate. The driver parses the
  event stream for progress/tool events and the gateway relays the final
  message to the channel. Continuity is free because zero sessions persist in
  the volume.
- **Heartbeat/autonomy**: scheduler fires prompts into a dedicated session
  ("read HEARTBEAT.md in your home and act on it"), OpenClaw pattern. User-defined
  schedules are interval entries stored in host config.
- **Memory/learning loop**: bootstrap files seeded into the volume on first run
  (ZEROCLAW.md identity + operating rules, MEMORY.md index, HEARTBEAT.md). The
  identity prompt instructs the agent to persist facts and to write zero skills
  after complex tasks, hermes-style. No code needed beyond seeding and prompts.
- **CLI surface**: `zeroclaw up | down | status | chat | exec "<prompt>" | give | take | beat | doctor | reset-env | daemon start|run|stop`.

## Stack

- Go 1.26+ for everything: one module, one binary. `zeroclaw` is the CLI;
  `zeroclawd` is the same binary relaunched with a `daemon run` subcommand, the
  way zero's internal/daemon launcher works. os/exec drives docker. Nothing
  imports zero's code; it stays an untouched sibling project consumed as a
  binary inside the container image (cross-compiled for linux/amd64 into
  `env/bin/zero`, which is untracked).
- Dependencies: stdlib only. Telegram long polling is plain net/http against
  the Bot API, so no bot library is needed. Any exception requires explicit
  approval before adding it.
- Follow zero's Go conventions (internal/ packages, table tests). The TS-specific
  style rules in the global CLAUDE.md do not apply here.

## Repo layout

```
zeroclaw/
  AGENTS.md              this file
  go.mod
  cmd/zeroclaw/main.go   entrypoint; dispatches CLI commands and `daemon run`
  internal/
    cli/                 thin client: command dispatch, chat REPL, event
                         rendering (style.go), RPC calls to zeroclawd
    daemon/              zeroclawd: supervisor, scheduler, RPC server, launch
    env/                 container lifecycle: docker.go, seeding, doctor checks
    agent/               driver.go (harness interface, zero-agnostic),
                         zerodriver.go (zero exec + stream-json),
                         sessions.go (conversation-to-session map)
    channels/            telegram.go (net/http long polling)
    config/              host config (~/.zeroclaw/config.json) + secrets
  env/
    Dockerfile
    LICENSE.zero         MIT notice for the bundled zero binary
    SOUL.md              operator-curated identity/values; baked into the image
                         at /opt/zeroclaw/SOUL.md (root-owned, outside the
                         agent-writable home) so the agent can read but never
                         edit it
    bootstrap/           ZEROCLAW.md, MEMORY.md, HEARTBEAT.md seeds
```

## Status

The original milestones M0 (walking skeleton), M1 (conversations + daemon/client
split), M2 (autonomy: heartbeats, schedules, memory), and M3 (Telegram channel)
are done and shipped. Remaining hardening items, none started:

- Tier 3 fallback (run without Docker, clearly labeled as soft isolation).
- Egress allowlist proxy.
- Autostart at login (Task Scheduler / launchd).
- Mac run.

## Ground rules for working on this repo

- Never bind-mount host paths into the container.
- Zero is consumed as a released binary in the image; never modify the zero repo
  from this project. Changes zero needs go through its own repo and PR process.
- Minimal deps, early returns, no em dashes in any authored text. Go idioms over
  the TS style rules in the global CLAUDE.md.
- No git actions of any kind without explicit consent.
- Verify before committing. You MUST run all Go code quality and security checks before committing code or completing your task:
  1. **Formatting**: Run `go fmt ./...` to format code.
  2. **Vetting**: Run `go vet ./...` to check for common mistakes.
  3. **Linting**: Run `golangci-lint run` to inspect code style and quality.
  4. **Vulnerability Scanning**: Run `govulncheck ./...` to check for security vulnerabilities.
  If any of these tools (`golangci-lint` or `govulncheck`) are not installed or are unavailable in the path when you attempt to run them, do not ignore the check. You must prompt the user with instructions to install them (e.g., `go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest` or `go install golang.org/x/vuln/cmd/govulncheck@latest`) and ask for confirmation/action before proceeding.
- Changes to the environment need a real end-to-end check (a turn through
  `zeroclaw exec`, or a command inside the container), not just tests.
- Remember the two-step rollout: `zeroclaw up` only builds the image when it is
  absent, and seeding never overwrites existing files in the agent's home, so
  image or bootstrap changes need an explicit rebuild
  (`docker rm -f zeroclaw && docker rmi zeroclaw-env && zeroclaw up`) and, for
  prompt changes, a refresh of `~/.config/zero/ZERO.md` in the container.

## Open questions (answer before or during the work that hits them)

1. Egress allowlist proxy: worth it, or note-and-defer?
2. Should zeroclaw expose zero's TUI directly (`docker exec -it zeroclaw zero`)
   as a power-user escape hatch? (Cheap, probably yes.)
3. One agent or multiple named agents (`zeroclaw -a work`)? Prototype assumes one.
4. Resident agent process inside the container (a long-lived loop that zeroclawd
   delivers messages to) instead of per-turn `zero exec` invocations? Revisit if
   per-turn latency or in-environment background work demands it; zero's daemon
   package and ACP support are the candidate mechanisms. Per-turn exec stays the
   prototype default because zero sessions already give continuity.
