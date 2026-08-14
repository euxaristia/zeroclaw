import { readToken, fetchStatus, fetchProviders, fetchModels, streamTurn, type AgentEvent, type CatalogItem } from "./api";
import { openPicker } from "./picker";
import { renderMarkdown } from "./markdown";

let authToken = "";
let currentProvider: string | undefined;
let currentModel: string | undefined;
let inFlight: AbortController | null = null;

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

// Persisted in sessionStorage, same scope as the auth token: survives a
// refresh but clears when the tab closes. A plain page reload previously
// lost the whole transcript and the active /model, /provider, and
// conversation selections, unlike the TUI where those live for the process
// lifetime of one `zeroclaw chat` invocation.
const TRANSCRIPT_KEY = "zeroclaw_transcript";
const STATE_KEY = "zeroclaw_ui_state";

function saveTranscript() {
  const blocks = Array.from(transcript.children).map((el) => ({
    className: el.className,
    html: el.innerHTML,
  }));
  sessionStorage.setItem(TRANSCRIPT_KEY, JSON.stringify(blocks));
}

// loadTranscript replays a saved transcript's markup back into the DOM. The
// stored HTML was produced entirely by this file via .textContent, never
// from user- or model-supplied raw HTML, so re-injecting it here doesn't
// introduce anything that wasn't already safely escaped on the way in.
function loadTranscript(): boolean {
  const raw = sessionStorage.getItem(TRANSCRIPT_KEY);
  if (!raw) return false;
  let blocks: { className: string; html: string }[];
  try {
    blocks = JSON.parse(raw);
  } catch {
    return false;
  }
  if (!Array.isArray(blocks) || blocks.length === 0) return false;
  for (const b of blocks) {
    const el = document.createElement("div");
    el.className = b.className;
    el.innerHTML = b.html;
    transcript.appendChild(el);
  }
  transcript.scrollTop = transcript.scrollHeight;
  return true;
}

function saveUIState() {
  sessionStorage.setItem(
    STATE_KEY,
    JSON.stringify({ model: currentModel, provider: currentProvider, conversation: convInput.value }),
  );
}

function loadUIState() {
  const raw = sessionStorage.getItem(STATE_KEY);
  if (!raw) return;
  try {
    const state = JSON.parse(raw) as { model?: string; provider?: string; conversation?: string };
    currentModel = state.model;
    currentProvider = state.provider;
    if (state.conversation) convInput.value = state.conversation;
  } catch {
    // ignore malformed state
  }
}

const transcript = document.getElementById("transcript") as HTMLDivElement;
const statusAgent = document.getElementById("status-agent") as HTMLSpanElement;
const convInput = document.getElementById("conv-input") as HTMLInputElement;
const composer = document.getElementById("composer") as HTMLFormElement;
const promptInput = document.getElementById("prompt-input") as HTMLTextAreaElement;
const sendBtn = document.getElementById("send-btn") as HTMLButtonElement;
const historyIndicator = document.getElementById("history-indicator") as HTMLSpanElement;

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
  saveTranscript();
}

function setSendButtonStopping(stopping: boolean) {
  sendBtn.textContent = stopping ? "Stop" : "Send";
  sendBtn.classList.toggle("stopping", stopping);
}

function systemMessage(text: string) {
  const el = appendBlock("system");
  el.textContent = text;
  saveTranscript();
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
    saveTranscript();
    return true;
  }
  if (text === "/model") {
    const items = await fetchModels(authToken, currentProvider ?? "gitlawb-opengateway");
    const picked = await openPicker("Choose a model", items);
    if (picked) {
      currentModel = picked.value;
      saveUIState();
      systemMessage(`Switched model to ${currentModel}`);
    }
    return true;
  }
  if (text === "/provider") {
    const items = await fetchProviders(authToken);
    const picked = await openPicker("Choose a provider", items);
    if (picked) {
      currentProvider = picked.value;
      saveUIState();
      systemMessage(`Switched provider to ${currentProvider}`);
    }
    return true;
  }
  if (text.startsWith("/model ")) {
    currentModel = text.slice("/model ".length).trim() || currentModel;
    saveUIState();
    systemMessage(`Switched model to ${currentModel}`);
    return true;
  }
  if (text.startsWith("/provider ")) {
    currentProvider = text.slice("/provider ".length).trim() || currentProvider;
    saveUIState();
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

// Shows "2/5" while browsing history, so a recalled prompt is visibly
// distinct from a fresh draft. Hidden once back at the live draft.
function updateHistoryIndicator() {
  if (historyIdx >= inputHistory.length) {
    historyIndicator.hidden = true;
    return;
  }
  historyIndicator.textContent = `${historyIdx + 1}/${inputHistory.length}`;
  historyIndicator.hidden = false;
}

function historyBack() {
  if (inputHistory.length === 0) return;
  if (historyIdx === inputHistory.length) historyDraft = promptInput.value;
  if (historyIdx > 0) {
    historyIdx--;
    promptInput.value = inputHistory[historyIdx]!;
    autoGrow();
    updateHistoryIndicator();
  }
}

function historyForward() {
  if (historyIdx >= inputHistory.length) return;
  historyIdx++;
  promptInput.value = historyIdx === inputHistory.length ? historyDraft : inputHistory[historyIdx]!;
  autoGrow();
  updateHistoryIndicator();
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
// of a new element per token. thinkingEl is the "thinking…" placeholder
// sendTurn shows the instant it fires the request (there is otherwise no
// feedback at all until the first token, matching internal/tui/model.go's
// "thinking..." row while pending with no streaming text/reasoning yet).
// run_start upgrades it to session/provider/model — the CLI's renderer
// shows this line too, and for /model auto it's the only place that
// reveals which model auto actually routed to — then the first real
// content event clears it.
function renderTurn(thinkingEl: HTMLElement | null): {
  onEvent: (ev: AgentEvent) => void;
  flush: () => void;
} {
  let reasoningEl: HTMLDivElement | null = null;
  let textEl: HTMLDivElement | null = null;
  let lastType = "";
  let thinking = thinkingEl;

  // Markdown is re-rendered from the accumulated source on every delta, so
  // the raw text has to be kept alongside the element. Repainting is
  // coalesced onto an animation frame: a fast stream would otherwise
  // re-parse the whole reply per token.
  let markdownSrc = "";
  let repaintQueued = false;
  const repaint = () => {
    repaintQueued = false;
    if (!textEl) return;
    textEl.replaceChildren(renderMarkdown(markdownSrc));
    transcript.scrollTop = transcript.scrollHeight;
  };
  const queueRepaint = () => {
    if (repaintQueued) return;
    repaintQueued = true;
    requestAnimationFrame(repaint);
  };

  const clearThinking = () => {
    thinking?.remove();
    thinking = null;
  };

  const onEvent = (ev: AgentEvent) => {
    switch (ev.type) {
      case "run_start":
        if (thinking) thinking.textContent = `session ${ev.sessionId} · ${ev.provider} ${ev.model}`;
        break;
      case "reasoning":
        clearThinking();
        if (!reasoningEl) reasoningEl = appendBlock("reasoning");
        reasoningEl.textContent += ev.delta;
        break;
      case "text":
        clearThinking();
        if (!textEl || lastType === "tool_call" || lastType === "tool_result") {
          textEl = appendBlock("reply");
          markdownSrc = "";
        }
        markdownSrc += ev.delta;
        queueRepaint();
        break;
      case "tool_call": {
        clearThinking();
        const el = appendBlock("tool");
        const name = document.createElement("span");
        name.className = "name";
        name.textContent = `⏺ ${ev.name}`;
        el.appendChild(name);
        textEl = null;
        break;
      }
      case "tool_result":
        clearThinking();
        if (ev.display?.summary) {
          const el = appendBlock("tool");
          const summary = document.createElement("span");
          summary.className = "summary";
          summary.textContent = ev.display.summary;
          el.appendChild(summary);
        }
        break;
      case "error":
        clearThinking();
        fail(`${ev.code}: ${ev.message}`);
        textEl = null;
        break;
    }
    lastType = ev.type;
    transcript.scrollTop = transcript.scrollHeight;
    saveTranscript();
  };

  // flush forces any frame-deferred markdown repaint to land now, so the
  // final delta is on screen (and in the saved transcript) before the turn
  // is considered finished.
  return { onEvent, flush: repaint };
}

async function sendTurn() {
  const prompt = promptInput.value.trim();
  if (!prompt) return;
  const conversation = convInput.value.trim() || "main";

  const userEl = appendBlock("user");
  userEl.textContent = prompt;
  saveTranscript();
  promptInput.value = "";
  autoGrow();
  closePalette();

  historyIndicator.hidden = true;
  if (!prompt.startsWith("/")) {
    inputHistory.push(prompt);
    historyIdx = inputHistory.length;
    historyDraft = "";
  }

  if (await handleSlashCommand(prompt)) return;

  const thinkingEl = appendBlock("thinking");
  thinkingEl.textContent = "thinking…";

  const turn = renderTurn(thinkingEl);
  // While a turn runs, Send becomes Stop. Aborting drops the connection,
  // which cancels the turn in the container rather than just hiding it.
  inFlight = new AbortController();
  setSendButtonStopping(true);
  try {
    const trailer = await streamTurn(
      authToken,
      conversation,
      prompt,
      { provider: currentProvider, model: currentModel },
      turn.onEvent,
      inFlight.signal,
    );
    turn.flush();
    const el = appendBlock(`session ${trailer.error ? "err" : "ok"}`);
    el.textContent = `${trailer.status} · session ${trailer.sessionId}`;
    saveTranscript();
  } catch (err) {
    turn.flush();
    if (err instanceof DOMException && err.name === "AbortError") {
      const el = appendBlock("session err");
      el.textContent = "stopped";
    } else {
      fail(err instanceof Error ? err.message : String(err));
    }
  } finally {
    inFlight = null;
    setSendButtonStopping(false);
    thinkingEl.remove();
    saveTranscript();
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
  loadUIState();

  try {
    const status = await fetchStatus(authToken);
    statusAgent.textContent = `${status.agent} · ${status.container} · pid ${status.pid}`;
  } catch (err) {
    statusAgent.textContent = "connection failed";
    fail(err instanceof Error ? err.message : String(err));
    return;
  }
  if (!loadTranscript()) {
    appendBlock("welcome").textContent = welcomeText;
    saveTranscript();
  }

  composer.addEventListener("submit", (e) => {
    e.preventDefault();
    if (inFlight) {
      inFlight.abort();
      return;
    }
    void sendTurn();
  });
  convInput.addEventListener("input", saveUIState);
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
    // Real typing only: history recall assigns .value directly, which does
    // not fire a native input event, so the badge survives a recall.
    historyIndicator.hidden = true;
  });
  autoGrow();
}

function autoGrow() {
  promptInput.style.height = "auto";
  promptInput.style.height = `${promptInput.scrollHeight}px`;
}

void main();
