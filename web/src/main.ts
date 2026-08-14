import { readToken, fetchStatus, fetchProviders, fetchModels, streamTurn, type AgentEvent } from "./api";
import { openPicker } from "./picker";

let currentProvider: string | undefined;
let currentModel: string | undefined;

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

function systemMessage(text: string) {
  const el = appendBlock("system");
  el.textContent = text;
}

// handleSlashCommand mirrors internal/tui/model.go's executeSlashCommandWithPrev:
// /help and /clear act locally, the argument forms of /model and /provider set
// local state threaded into the next turn, and bare /model or /provider open
// the same catalog picker the TUI does (backed by GET /models and
// GET /providers). Unrecognized slash text is swallowed rather than sent to
// the model, matching the TUI's own silent fallthrough for unknown commands.
// Returns true if the input was handled locally and should not be sent as a
// prompt.
async function handleSlashCommand(token: string, text: string): Promise<boolean> {
  if (text === "/help" || text === "/?" || text === "/h") {
    systemMessage(
      "zeroclaw web commands:\n" +
        "  /model [name]      Choose or switch active LLM model\n" +
        "  /provider [name]   Choose or switch model provider\n" +
        "  /help              Show available commands\n" +
        "  /clear             Clear chat transcript",
    );
    return true;
  }
  if (text === "/clear") {
    transcript.replaceChildren();
    return true;
  }
  if (text === "/model") {
    const items = await fetchModels(token, currentProvider ?? "gitlawb-opengateway");
    const picked = await openPicker("Choose a model", items);
    if (picked) {
      currentModel = picked.value;
      systemMessage(`Switched model to ${currentModel}`);
    }
    return true;
  }
  if (text === "/provider") {
    const items = await fetchProviders(token);
    const picked = await openPicker("Choose a provider", items);
    if (picked) {
      currentProvider = picked.value;
      systemMessage(`Switched provider to ${currentProvider}`);
    }
    return true;
  }
  if (text.startsWith("/model ")) {
    currentModel = text.slice("/model ".length).trim() || currentModel;
    systemMessage(`Switched model to ${currentModel}`);
    return true;
  }
  if (text.startsWith("/provider ")) {
    currentProvider = text.slice("/provider ".length).trim() || currentProvider;
    systemMessage(`Switched provider to ${currentProvider}`);
    return true;
  }
  if (text.startsWith("/")) {
    return true;
  }
  return false;
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
  autoGrow();

  if (await handleSlashCommand(token, prompt)) return;
  sendBtn.disabled = true;

  try {
    const onEvent = renderTurn();
    const trailer = await streamTurn(
      token,
      conversation,
      prompt,
      { provider: currentProvider, model: currentModel },
      onEvent,
    );
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
  // resize: none in CSS; grow with content instead of the native drag handle
  // (which rendered as a near-invisible nub in the wrong corner of the row).
  promptInput.addEventListener("input", autoGrow);
  autoGrow();
}

function autoGrow() {
  promptInput.style.height = "auto";
  promptInput.style.height = `${promptInput.scrollHeight}px`;
}

void main();
