import { readToken, fetchStatus, fetchProviders, fetchModels, streamTurn, type AgentEvent, type CatalogItem } from "./api";
import { openPicker } from "./picker";

let authToken = "";
let currentProvider: string | undefined;
let currentModel: string | undefined;

// Mirrors internal/tui/model.go's inputHistory/historyIdx/historyDraft: Up
// walks back through previously sent prompts, Down walks forward, and the
// in-progress draft is preserved so arrowing back down past the newest
// entry restores what you were typing rather than an empty box. Slash
// commands are never recorded, matching handleSubmit's early return before
// the history append.
const inputHistory: string[] = [];
let historyIdx = 0;
let historyDraft = "";

const welcomeText = "zeroclaw web. Type /help for commands.";

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
async function handleSlashCommand(text: string): Promise<boolean> {
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
    appendBlock("welcome").textContent = welcomeText;
    return true;
  }
  if (text === "/model") {
    const items = await fetchModels(authToken, currentProvider ?? "gitlawb-opengateway");
    const picked = await openPicker("Choose a model", items);
    if (picked) {
      currentModel = picked.value;
      systemMessage(`Switched model to ${currentModel}`);
    }
    return true;
  }
  if (text === "/provider") {
    const items = await fetchProviders(authToken);
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

// --- Command palette -------------------------------------------------------
// Mirrors internal/tui/model.go: typing "/" as the first character of the
// input opens an overlay of matching slash commands that narrows live as you
// keep typing (query = text after "/"), the same substring match picker.ts
// uses for the /model and /provider item pickers. Arrow keys move the
// selection, Enter runs the highlighted command, Escape clears the input and
// closes it. Unlike the TUI's command picker (which also lists /theme and
// /quit) this only lists commands the web UI actually implements.
const COMMANDS: CatalogItem[] = [
  { group: "Commands", label: "/model", value: "/model", meta: "Choose or switch active LLM model", provider: "" },
  { group: "Commands", label: "/provider", value: "/provider", meta: "Choose or switch model provider", provider: "" },
  { group: "Commands", label: "/help", value: "/help", meta: "Show available commands", provider: "" },
  { group: "Commands", label: "/clear", value: "/clear", meta: "Clear chat transcript", provider: "" },
];

const palette = document.createElement("div");
palette.className = "command-palette";
palette.hidden = true;
composer.appendChild(palette);

let paletteItems: CatalogItem[] = [];
let paletteSelected = 0;

function renderPalette() {
  palette.replaceChildren();
  paletteItems.forEach((item, i) => {
    const row = document.createElement("div");
    row.className = "picker-row" + (i === paletteSelected ? " selected" : "");
    const label = document.createElement("span");
    label.textContent = item.label;
    const meta = document.createElement("span");
    meta.className = "meta";
    meta.textContent = item.meta;
    row.appendChild(label);
    row.appendChild(meta);
    // mousedown, not click: fires before the textarea blurs, matching the
    // TUI's immediate command execution on selection.
    row.addEventListener("mousedown", (e) => {
      e.preventDefault();
      void selectPaletteItem(i);
    });
    palette.appendChild(row);
  });
  palette.hidden = paletteItems.length === 0;
}

function updatePalette() {
  const val = promptInput.value;
  if (!val.startsWith("/") || val.includes("\n")) {
    closePalette();
    return;
  }
  const q = val.slice(1).toLowerCase();
  paletteItems = COMMANDS.filter((c) => `${c.label} ${c.meta}`.toLowerCase().includes(q));
  paletteSelected = 0;
  renderPalette();
}

function closePalette() {
  paletteItems = [];
  palette.hidden = true;
}

function historyBack() {
  if (inputHistory.length === 0) return;
  if (historyIdx === inputHistory.length) historyDraft = promptInput.value;
  if (historyIdx > 0) {
    historyIdx--;
    promptInput.value = inputHistory[historyIdx]!;
    autoGrow();
  }
}

function historyForward() {
  if (historyIdx >= inputHistory.length) return;
  historyIdx++;
  promptInput.value = historyIdx === inputHistory.length ? historyDraft : inputHistory[historyIdx]!;
  autoGrow();
}

async function selectPaletteItem(i: number) {
  const item = paletteItems[i];
  closePalette();
  if (!item) return;
  promptInput.value = "";
  autoGrow();
  await handleSlashCommand(item.value);
  promptInput.focus();
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

async function sendTurn() {
  const prompt = promptInput.value.trim();
  if (!prompt) return;
  const conversation = convInput.value.trim() || "main";

  const userEl = appendBlock("user");
  userEl.textContent = prompt;
  promptInput.value = "";
  autoGrow();
  closePalette();

  if (!prompt.startsWith("/")) {
    inputHistory.push(prompt);
    historyIdx = inputHistory.length;
    historyDraft = "";
  }

  if (await handleSlashCommand(prompt)) return;
  sendBtn.disabled = true;

  try {
    const onEvent = renderTurn();
    const trailer = await streamTurn(
      authToken,
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
  authToken = token;

  try {
    const status = await fetchStatus(authToken);
    statusAgent.textContent = `${status.agent} · ${status.container} · pid ${status.pid}`;
  } catch (err) {
    statusAgent.textContent = "connection failed";
    fail(err instanceof Error ? err.message : String(err));
    return;
  }
  appendBlock("welcome").textContent = welcomeText;

  composer.addEventListener("submit", (e) => {
    e.preventDefault();
    void sendTurn();
  });
  promptInput.addEventListener("keydown", (e) => {
    if (!palette.hidden && paletteItems.length > 0) {
      if (e.key === "ArrowDown") {
        e.preventDefault();
        paletteSelected = (paletteSelected + 1) % paletteItems.length;
        renderPalette();
        return;
      }
      if (e.key === "ArrowUp") {
        e.preventDefault();
        paletteSelected = (paletteSelected - 1 + paletteItems.length) % paletteItems.length;
        renderPalette();
        return;
      }
      if (e.key === "Enter" && !e.shiftKey) {
        e.preventDefault();
        void selectPaletteItem(paletteSelected);
        return;
      }
      if (e.key === "Escape") {
        e.preventDefault();
        promptInput.value = "";
        autoGrow();
        closePalette();
        return;
      }
    }
    if (e.key === "ArrowUp" && promptInput.selectionStart === 0 && promptInput.selectionEnd === 0) {
      e.preventDefault();
      historyBack();
      return;
    }
    if (
      e.key === "ArrowDown" &&
      promptInput.selectionStart === promptInput.value.length &&
      promptInput.selectionEnd === promptInput.value.length
    ) {
      e.preventDefault();
      historyForward();
      return;
    }
    if (e.key === "Enter" && !e.shiftKey) {
      e.preventDefault();
      composer.requestSubmit();
    }
  });
  // resize: none in CSS; grow with content instead of the native drag handle
  // (which rendered as a near-invisible nub in the wrong corner of the row).
  promptInput.addEventListener("input", () => {
    autoGrow();
    updatePalette();
  });
  autoGrow();
}

function autoGrow() {
  promptInput.style.height = "auto";
  promptInput.style.height = `${promptInput.scrollHeight}px`;
}

void main();
