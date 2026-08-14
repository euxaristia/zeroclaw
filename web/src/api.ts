// Mirrors internal/agent/driver.go's Event and internal/daemon/server.go's
// Trailer/TurnRequest. Keep in sync with those when the wire schema changes.

export interface AgentEvent {
  schemaVersion: number;
  type: string;
  runId: string;
  sessionId: string;
  delta: string;
  text: string;
  name: string;
  status: string;
  exitCode: number;
  code: string;
  message: string;
  provider: string;
  model: string;
  display: {
    kind: string;
    summary: string;
  };
}

export interface Trailer {
  type: "zeroclaw_result";
  sessionId: string;
  status: string;
  final: string;
  error?: string;
}

export interface StatusResponse {
  agent: string;
  container: string;
  pid: number;
  conversations: number;
}

const TOKEN_KEY = "zeroclaw_token";

// readToken pulls the bearer token out of ?token= on first load (the way
// zeroclaw web launches the page) and persists it in sessionStorage only, so
// it never touches disk and clears when the tab closes. The URL is then
// scrubbed so the token doesn't linger in history or get pasted around.
export function readToken(): string | null {
  const url = new URL(window.location.href);
  const fromQuery = url.searchParams.get("token");
  if (fromQuery) {
    sessionStorage.setItem(TOKEN_KEY, fromQuery);
    url.searchParams.delete("token");
    window.history.replaceState({}, "", url.toString());
    return fromQuery;
  }
  return sessionStorage.getItem(TOKEN_KEY);
}

function authHeaders(token: string): HeadersInit {
  return { Authorization: `Bearer ${token}` };
}

export async function fetchStatus(token: string): Promise<StatusResponse> {
  const resp = await fetch("/status", { headers: authHeaders(token) });
  if (!resp.ok) throw new Error(`status request failed: ${resp.status}`);
  return resp.json();
}

// streamTurn posts one turn and calls onEvent for each driver event as it
// streams in over newline-delimited JSON, matching how the CLI's turnStream
// reads the same /turn response.
export async function streamTurn(
  token: string,
  conversation: string,
  prompt: string,
  onEvent: (ev: AgentEvent) => void,
): Promise<Trailer> {
  const resp = await fetch("/turn", {
    method: "POST",
    headers: { ...authHeaders(token), "Content-Type": "application/json" },
    body: JSON.stringify({ conversation, prompt }),
  });
  if (!resp.ok || !resp.body) {
    throw new Error(`daemon rejected turn: ${resp.status}`);
  }

  const reader = resp.body.getReader();
  const decoder = new TextDecoder();
  let buf = "";
  let trailer: Trailer | null = null;

  const consumeLine = (line: string) => {
    const trimmed = line.trim();
    if (!trimmed) return;
    const parsed = JSON.parse(trimmed);
    if (parsed.type === "zeroclaw_result") {
      trailer = parsed as Trailer;
      return;
    }
    onEvent(parsed as AgentEvent);
  };

  for (;;) {
    const { done, value } = await reader.read();
    if (done) break;
    buf += decoder.decode(value, { stream: true });
    let idx: number;
    while ((idx = buf.indexOf("\n")) !== -1) {
      consumeLine(buf.slice(0, idx));
      buf = buf.slice(idx + 1);
    }
  }
  if (buf) consumeLine(buf);

  if (!trailer) throw new Error("connection to zeroclawd ended mid-turn");
  if ((trailer as Trailer).error) {
    throw new Error(`turn failed: ${(trailer as Trailer).error}`);
  }
  return trailer;
}
