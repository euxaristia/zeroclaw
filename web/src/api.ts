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
  // Present on "usage" events.
  promptTokens: number;
  completionTokens: number;
  totalTokens: number;
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
  // What a turn with no override would use. Absent when the daemon cannot
  // reach the backend to ask.
  provider?: string;
  model?: string;
  // Context size of the active model, absent when unknown.
  contextWindow?: number;
  contextWindowLabel?: string;
}

// Mirrors internal/catalog.Item.
export interface CatalogItem {
  group: string;
  label: string;
  value: string;
  meta: string;
  provider: string;
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

// Maps conversation name to the zero session id backing it.
export async function fetchConversations(token: string): Promise<Record<string, string>> {
  const resp = await fetch("/conversations", { headers: authHeaders(token) });
  if (!resp.ok) throw new Error(`conversations request failed: ${resp.status}`);
  return resp.json();
}

// Drops the conversation's session mapping so the next turn starts fresh.
export async function resetConversation(token: string, conversation: string): Promise<void> {
  const resp = await fetch(`/conversations/${encodeURIComponent(conversation)}`, {
    method: "DELETE",
    headers: authHeaders(token),
  });
  if (!resp.ok) throw new Error(`reset failed: ${resp.status}`);
}

export async function fireHeartbeat(token: string): Promise<void> {
  const resp = await fetch("/beat", { method: "POST", headers: authHeaders(token) });
  if (!resp.ok) throw new Error(`heartbeat failed: ${resp.status}`);
}

export async function fetchProviders(token: string): Promise<CatalogItem[]> {
  const resp = await fetch("/providers", { headers: authHeaders(token) });
  if (!resp.ok) throw new Error(`providers request failed: ${resp.status}`);
  return resp.json();
}

export async function fetchModels(token: string, provider: string): Promise<CatalogItem[]> {
  const url = provider ? `/models?provider=${encodeURIComponent(provider)}` : "/models";
  const resp = await fetch(url, { headers: authHeaders(token) });
  if (!resp.ok) throw new Error(`models request failed: ${resp.status}`);
  return resp.json();
}

export interface TurnOverrides {
  provider?: string;
  model?: string;
  // low | medium | high; omitted leaves the backend default.
  reasoningEffort?: string;
  // Caps the agent loop's tool turns; omitted leaves the backend default.
  maxTurns?: number;
}

// streamTurn posts one turn and calls onEvent for each driver event as it
// streams in over newline-delimited JSON, matching how the CLI's turnStream
// reads the same /turn response.
//
// Aborting `signal` is a real cancellation, not just a UI one: the daemon
// hands its request context to the driver, which hands it to `docker exec`,
// so dropping the connection kills the in-flight turn in the container.
export async function streamTurn(
  token: string,
  conversation: string,
  prompt: string,
  overrides: TurnOverrides,
  onEvent: (ev: AgentEvent) => void,
  signal?: AbortSignal,
): Promise<Trailer> {
  const resp = await fetch("/turn", {
    method: "POST",
    headers: { ...authHeaders(token), "Content-Type": "application/json" },
    body: JSON.stringify({ conversation, prompt, ...overrides }),
    signal,
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
