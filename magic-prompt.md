# zeroclaw: plan / magic prompt

You are building **zeroclaw**, a prototype autonomous personal agent in the spirit of
OpenClaw and Hermes Agent, with **zero** (the Go CLI coding harness at
`../zero`) as the brain and tool runtime. Zeroclaw is not a fork of zero: it is a
host-side orchestrator that gives zero a body: an isolated persistent environment,
an always-on loop, channels, memory, and schedules.

Read this whole file before writing code. Where this file says "decide later," do
not decide it now.

## What zeroclaw is

- An always-on agent service (`zeroclawd`) that runs on the host (this Windows PC
  now, a Mac later) with no terminal attached. The `zeroclaw` CLI is a thin
  client that talks to the daemon over a local socket; closing every terminal
  changes nothing for the agent. Heartbeats, schedules, and chat channels keep
  running.
- The agent is independent of the zero CLI too: zeroclaw owns the agent's
  identity, memory, sessions-to-conversations mapping, channels, and schedules.
  Zero is consumed behind a small harness-driver interface as the first (and for
  the prototype, only) execution backend.
- The agent itself lives inside its own **completely isolated working environment**
  (its own home directory, its own packages, its own filesystem). It never touches
  the host filesystem.
- You talk to it over channels (CLI first, one chat platform second).
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
- Image: small Debian base + zero binary + git + bun + node + ripgrep + curl.
  Build from `env/Dockerfile` in this repo.
- A named volume mounted at `/home/zeroclaw` is the agent's entire persistent
  world: zero config and sessions, workspace, memory, skills. Container is
  disposable; the volume is the agent.
- **No host bind mounts. Ever.** File exchange with the host goes through an
  explicit `zeroclaw give <file>` / `zeroclaw take <path>` copy command
  (docker cp) so every transfer is a deliberate act.
- Network: container gets normal egress (it needs the model API). Host isolation
  is the goal of tier 1, not network isolation. An egress allowlist proxy is a
  later hardening item, listed under open questions.
- Secrets: provider API key injected as env at `docker exec` time from host
  config. Never baked into the image or written into the volume by us.

### Tier 2: zero's sandbox inside the container

- Zero runs with sandbox policy `enforce` inside the container where the platform
  allows it. Defense in depth against prompt-injected destructive commands wrecking
  the agent's own home.
- Zero's permission mode inside the container is full-auto/unsafe (the container is
  what protects the host; prompting has no user to answer it).
- Investigation task, not assumption: whether zero's Linux helper (namespaces,
  Landlock, seccomp) works inside Docker default seccomp/apparmor profiles, and
  what `--security-opt` it needs. If nesting fails, document the degradation and
  run tier 1 only. The re-entrancy guard (`ZERO_SANDBOXED` +
  `ZERO_SANDBOX_BACKEND`) matters here; check `../zero/internal/sandbox/types.go`.

### Tier 3 (fallback, no Docker present): zero sandbox on host

- Agent home pinned to `~/.zeroclaw/home`, zero sandbox `enforce`, writes limited
  to that home, out-of-workspace deny, permission mode auto (not unsafe).
- `zeroclaw doctor` and startup output must say plainly: "running without hard
  isolation." This tier exists so the prototype runs anywhere, not because it
  meets the isolation goal.

## Architecture

```
host                                    container (zeroclaw-env)
+---------------------------+           +----------------------------+
| zeroclaw daemon (Bun/TS)  |           |  /home/zeroclaw  (volume)  |
|  - gateway RPC api        |  docker   |   ZEROCLAW.md  (identity)  |
|  - channel: Telegram      |  exec     |   MEMORY.md + memory/      |
|  - scheduler (cron+beat)  | --------> |   workspace/               |
|  - session router         |  stream-  |   .config/zero/  sessions, |
|  - harness driver: zero   |  JSON     |     skills, providers      |
|  - env lifecycle (docker) |           |  zero exec (inner sandbox) |
|  - host config + secrets  |           +----------------------------+
+---------------------------+
        ^  local socket RPC
        |
+---------------------------+
| zeroclaw CLI (thin client)|   chat / exec / status / give / take ...
+---------------------------+   CLI chat is just another gateway client
```

- **Daemon** (`zeroclawd`, started by `zeroclaw up`): a standalone service that
  survives terminal close and client disconnects. Supervises the container, owns
  schedules, hosts channels, and exposes a local control plane (unix socket on
  Linux/macOS, named pipe on Windows; localhost HTTP is acceptable for the
  prototype if simpler). Single process, hermes-gateway-style. Autostart at
  login (Task Scheduler / launchd) is an M4 item.
- **CLI as thin client**: every `zeroclaw` command except `up` is an RPC client.
  `zeroclaw chat` is one more channel attached to the gateway, a peer of
  Telegram, with no privileged path into agent internals. If the daemon is not
  running, clients say so and suggest `zeroclaw up`; nothing falls back to
  in-process execution.
- **Harness driver**: `internal/agent/driver.go` defines the minimal interface
  zeroclaw needs from an execution backend (start or resume a session, send a
  turn, receive an event stream, list sessions). `zerodriver.go` implements it
  over `zero exec` stream-JSON. All zero-specific knowledge lives inside the
  driver; nothing outside it may shell out to zero or parse its output. The
  agent's durable state (identity, memory, conversation-to-session map,
  schedules, channel config) belongs to zeroclaw, so the harness is swappable.
- **Turn execution** (inside the zero driver): each conversation maps to a zero
  session inside the container. A turn is
  `docker exec -i zeroclaw zero exec --resume <id> --input-format stream-json --output-format stream-json`.
  The driver parses the event stream for progress/tool events and the gateway
  relays the final message to the channel. Continuity is free because zero
  sessions persist in the volume.
- **Heartbeat/autonomy**: scheduler fires prompts into a dedicated session
  ("read HEARTBEAT.md in your home and act on it"), OpenClaw pattern. User-defined
  schedules are natural-language cron entries stored in host config.
- **Memory/learning loop**: bootstrap files seeded into the volume on first run
  (ZEROCLAW.md identity + operating rules, MEMORY.md index, memory/ dir). The
  identity prompt instructs the agent to persist facts and to write zero skills
  after complex tasks, hermes-style. No code needed beyond seeding and prompts.
- **CLI surface**: `zeroclaw up | down | status | chat | exec "<prompt>" | give | take | doctor | reset-env`.
  All of these except `up` are RPC clients of `zeroclawd`.

## Stack

- Go 1.25+ for everything: one module, one binary. `zeroclaw` is the CLI;
  `zeroclawd` is the same binary relaunched with a hidden `daemon run`
  subcommand, the way zero's internal/daemon launcher works. os/exec drives
  docker. Nothing imports zero's code; it stays an untouched sibling project
  consumed as a binary inside the container image.
- New standalone repo at `C:\Users\parse\source\repos\zeroclaw` (this directory).
- Dependencies: stdlib only through M3. Telegram long polling is plain net/http
  against the Bot API, so no bot library is needed. Any exception requires
  explicit approval before adding it.
- Follow zero's Go conventions (internal/ packages, table tests). The TS-specific
  style rules in the global CLAUDE.md do not apply here.

## Repo layout

```
zeroclaw/
  magic-prompt.md        this file
  go.mod
  cmd/zeroclaw/main.go   entrypoint; dispatches CLI commands and `daemon run`
  internal/
    cli/                 thin client: command dispatch + RPC calls to zeroclawd
    daemon/              zeroclawd: supervisor, scheduler, RPC server
                         (unix socket / windows named pipe)
    env/                 container lifecycle: docker.go, tiers, doctor checks
    agent/               driver.go (harness interface, zero-agnostic),
                         zerodriver.go (zero exec + stream-json),
                         sessions.go (conversation-to-session map)
    channels/            clichat.go (gateway client), telegram.go (net/http
                         long polling)
    config/              host config (~/.zeroclaw/config.json) + secrets
  env/
    Dockerfile
    bootstrap/           ZEROCLAW.md, MEMORY.md, HEARTBEAT.md seeds
```

## Milestones (each one demoable; stop and show after each)

- **M0 walking skeleton**: Dockerfile builds with zero installed; `zeroclaw up`
  creates volume + container and seeds bootstrap files; `zeroclaw exec "hi"` runs
  one stream-JSON turn through zero in the container and prints the reply.
- **M1 conversations + daemon/client split**: `zeroclawd` runs detached and
  survives terminal close; `zeroclaw chat` is an interactive REPL client over the
  local socket with a persistent conversation; conversations resume across both
  client disconnect and daemon restart; `give`/`take`; `doctor` (checks docker,
  image, volume, provider key, daemon socket, inner sandbox status). Demo:
  start a chat, kill the terminal, reopen, continue the same conversation.
- **M2 autonomy**: scheduler with heartbeat session + user cron entries; memory
  bootstrap prompts proven (agent writes to MEMORY.md unprompted across two
  sessions); tier 2 nesting investigation resolved and documented.
- **M3 channel**: Telegram via long polling, single-owner allowlist by chat id.
- **M4 hardening (optional)**: tier 3 fallback, egress allowlist proxy,
  autostart at login (Task Scheduler / launchd), Mac run.

Scope note: M0 is roughly a Dockerfile, a handful of small Go files, and a
stream-JSON event decoder. Minutes-to-hours territory, not days. The genuinely slow items are the
tier 2 nesting investigation and Telegram end-to-end testing.

## Ground rules for the build

- Never bind-mount host paths into the container.
- Zero is consumed as a released binary in the image; never modify the zero repo
  from this project.
- Minimal deps, early returns, no em dashes in any authored text. Go idioms over
  the TS style rules in the global CLAUDE.md.
- No git actions of any kind without explicit consent, including `git init`.
- Every milestone ends with a real end-to-end demo command, not just tests.

## Open questions (answer before or during the milestone that hits them)

1. Provider/model for the prototype's default (whatever zero setup writes, but
   which key does the daemon inject?).
2. Telegram vs Discord for M3.
3. Egress allowlist proxy: worth it for the prototype, or note-and-defer?
4. Should zeroclaw expose zero's TUI directly (`docker exec -it zeroclaw zero`)
   as a power-user escape hatch? (Cheap, probably yes.)
5. One agent or multiple named agents (`zeroclaw -a work`)? Prototype assumes one.
6. Resident agent process inside the container (a long-lived loop that zeroclawd
   delivers messages to) instead of per-turn `zero exec` invocations? Revisit if
   per-turn latency or in-environment background work demands it; zero's daemon
   package and ACP support are the candidate mechanisms. Per-turn exec stays the
   prototype default because zero sessions already give continuity.
