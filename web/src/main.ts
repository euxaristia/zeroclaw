import { readToken, fetchStatus, streamTurn, type AgentEvent } from "./api";

const transcript = document.getElementById("transcript") as HTMLDivElement;
const statusAgent = document.getElementById("status-agent") as HTMLSpanElement;
const convInput = document.getElementById("conv-input") as HTMLInputElement;
const composer = document.getElementById("composer") as HTMLFormElement;
const promptInput = document.getElementById("prompt-input") as HTMLTextAreaElement;
const sendBtn = document.getElementById("send-btn") as HTMLButtonElement;

function appendBlock(className: string): HTMLDivElement {
  const el = document.createElement("div");
  el.className = `block ${className}`;
  transcript.appendChild(el);
  transcript.scrollTop = transcript.scrollHeight;
  return el;
}

function fail(message: string) {
  const el = appendBlock("error");
  el.textContent = message;
}

// renderTurn mirrors cli.go's renderer.event: reasoning stays muted and
// separate from the reply, tool calls get a name line plus a faint result
// summary, and running text deltas coalesce into one growing block instead
// of a new element per token.
function renderTurn(): (ev: AgentEvent) => void {
  let reasoningEl: HTMLDivElement | null = null;
  let textEl: HTMLDivElement | null = null;
  let lastType = "";

  return (ev: AgentEvent) => {
    switch (ev.type) {
      case "reasoning":
        if (!reasoningEl) reasoningEl = appendBlock("reasoning");
        reasoningEl.textContent += ev.delta;
        break;
      case "text":
        if (!textEl || lastType === "tool_call" || lastType === "tool_result") {
          textEl = appendBlock("reply");
        }
        textEl.textContent += ev.delta;
        break;
      case "tool_call": {
        const el = appendBlock("tool");
        const name = document.createElement("span");
        name.className = "name";
        name.textContent = `⏺ ${ev.name}`;
        el.appendChild(name);
        textEl = null;
        break;
      }
      case "tool_result":
        if (ev.display?.summary) {
          const el = appendBlock("tool");
          const summary = document.createElement("span");
          summary.className = "summary";
          summary.textContent = ev.display.summary;
          el.appendChild(summary);
        }
        break;
      case "error":
        fail(`${ev.code}: ${ev.message}`);
        textEl = null;
        break;
    }
    lastType = ev.type;
    transcript.scrollTop = transcript.scrollHeight;
  };
}

async function sendTurn(token: string) {
  const prompt = promptInput.value.trim();
  if (!prompt) return;
  const conversation = convInput.value.trim() || "main";

  const userEl = appendBlock("user");
  userEl.textContent = prompt;
  promptInput.value = "";
  sendBtn.disabled = true;

  try {
    const onEvent = renderTurn();
    const trailer = await streamTurn(token, conversation, prompt, onEvent);
    const el = appendBlock(`session ${trailer.error ? "err" : "ok"}`);
    el.textContent = `${trailer.status} · session ${trailer.sessionId}`;
  } catch (err) {
    fail(err instanceof Error ? err.message : String(err));
  } finally {
    sendBtn.disabled = false;
    promptInput.focus();
  }
}

async function main() {
  const token = readToken();
  if (!token) {
    statusAgent.textContent = "no token";
    fail("No auth token found. Open this page with `zeroclaw web`, not a bare URL.");
    composer.querySelectorAll("input, textarea, button").forEach((el) => {
      (el as HTMLInputElement | HTMLTextAreaElement | HTMLButtonElement).disabled = true;
    });
    return;
  }

  try {
    const status = await fetchStatus(token);
    statusAgent.textContent = `${status.agent} · ${status.container} · pid ${status.pid}`;
  } catch (err) {
    statusAgent.textContent = "connection failed";
    fail(err instanceof Error ? err.message : String(err));
    return;
  }

  composer.addEventListener("submit", (e) => {
    e.preventDefault();
    void sendTurn(token);
  });
  promptInput.addEventListener("keydown", (e) => {
    if (e.key === "Enter" && !e.shiftKey) {
      e.preventDefault();
      composer.requestSubmit();
    }
  });
}

void main();
