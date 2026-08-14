import {
  readToken,
  fetchStatus,
  fetchProviders,
  fetchModels,
  fetchConversations,
  resetConversation,
  fireHeartbeat,
  streamTurn,
  type AgentEvent,
  type CatalogItem,
} from "./api";
import { openPicker } from "./picker";
import { renderMarkdown } from "./markdown";
import { applyTheme, defaultTheme } from "./theme";
import { THEMES } from "./themes";
import { contextFill, gaugeText } from "./usage";

let authToken = "";
let currentProvider: string | undefined;
let currentModel: string | undefined;
let inFlight: AbortController | null = null;
let currentTheme = defaultTheme().name;
let currentContextLabel = "";
let currentContextWindow = 0;

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

// Transcripts are stored per conversation. Each conversation is a separate
// zero session on the daemon, so showing one conversation's history while
// another is selected would misrepresent what the agent actually remembers.
// activeConversation is the single source of truth for which conversation
// is selected. Reading it back off the input element instead would go stale
// the moment a switch happens through a path that doesn't touch the input
// (the /conversation picker), and transcripts would save under the wrong key.
let activeConversation = "main";

function transcriptKey(conversation = activeConversation): string {
  return `${TRANSCRIPT_KEY}:${conversation}`;
}

function saveTranscript() {
  const blocks = Array.from(transcript.children).map((el) => ({
    className: el.className,
    html: el.innerHTML,
  }));
  sessionStorage.setItem(transcriptKey(), JSON.stringify(blocks));
}

// loadTranscript replays a saved transcript's markup back into the DOM. The
// stored HTML was produced entirely by this file via .textContent, never
// from user- or model-supplied raw HTML, so re-injecting it here doesn't
// introduce anything that wasn't already safely escaped on the way in.
function loadTranscript(): boolean {
  const raw = sessionStorage.getItem(transcriptKey());
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
    JSON.stringify({
      model: currentModel,
      provider: currentProvider,
      conversation: activeConversation,
      theme: currentTheme,
    }),
  );
}

function loadUIState() {
  const raw = sessionStorage.getItem(STATE_KEY);
  if (!raw) return;
  try {
    const state = JSON.parse(raw) as {
      model?: string;
      provider?: string;
      conversation?: string;
      theme?: string;
    };
    currentModel = state.model;
    currentProvider = state.provider;
    if (state.conversation) {
      activeConversation = state.conversation;
      convInput.value = state.conversation;
    }
    if (state.theme) currentTheme = state.theme;
  } catch {
    // ignore malformed state
  }
}

const transcript = document.getElementById("transcript") as HTMLDivElement;
const convInput = document.getElementById("conv-input") as HTMLInputElement;
const composer = document.getElementById("composer") as HTMLFormElement;
const promptInput = document.getElementById("prompt-input") as HTMLTextAreaElement;
const sendBtn = document.getElementById("send-btn") as HTMLButtonElement;
const historyIndicator = document.getElementById("history-indicator") as HTMLSpanElement;
const modelIndicator = document.getElementById("model-indicator") as HTMLSpanElement;
const composerModel = document.getElementById("composer-model") as HTMLSpanElement;
const statusline = document.getElementById("statusline") as HTMLElement;
const statusText = document.getElementById("status-text") as HTMLSpanElement;
const contextGauge = document.getElementById("context-gauge") as HTMLSpanElement;
const statusMeta = document.getElementById("status-meta") as HTMLSpanElement;

// Zero shows the model in two places and the web UI follows: titleModelSegment
// puts "provider/model" at the right of the title bar, and composerDividerLine
// repeats the model alone, muted, in the rule above the input. The provider is
// deliberately not repeated down there.
function updateModelIndicator() {
  const provider = currentProvider?.trim() ?? "";
  const model = currentModel?.trim() ?? "";

  modelIndicator.replaceChildren();
  if (!provider && !model) {
    modelIndicator.textContent = "no provider";
    modelIndicator.classList.add("unset");
  } else {
    modelIndicator.classList.remove("unset");
    modelIndicator.append(provider && model ? `${provider}/${model}` : model || provider);
    // titleBar appends the context window in faint after the model, and
    // only when it is known.
    if (currentContextLabel) {
      const ctx = document.createElement("span");
      ctx.className = "ctx";
      ctx.textContent = ` · ${currentContextLabel}`;
      modelIndicator.appendChild(ctx);
    }
  }

  composerModel.textContent = model || "no model";
}

// statusLine: green "ready" when idle, muted "working Ns" counting up while
// a turn runs, with the session id on the right once one exists.
let workingTimer: number | undefined;

function setStatusReady() {
  clearInterval(workingTimer);
  workingTimer = undefined;
  statusline.classList.remove("working");
  statusText.textContent = "ready";
}

function setStatusWorking() {
  const start = Date.now();
  statusline.classList.add("working");
  const tick = () => {
    statusText.textContent = `working ${Math.floor((Date.now() - start) / 1000)}s`;
  };
  tick();
  clearInterval(workingTimer);
  workingTimer = setInterval(tick, 1000) as unknown as number;
}

// Esc cancels a running turn, matching internal/tui/model.go: the first
// press only arms a 3s confirmation and leaves any draft alone, since
// nothing has been cancelled yet; a second press within the window actually
// aborts. Cancelling a long turn by accident is expensive, hence the
// two-step rather than a single key.
const ESC_CONFIRM_MS = 3000;
let cancelConfirmTimer: number | undefined;

function cancelConfirmActive(): boolean {
  return cancelConfirmTimer !== undefined;
}

function armCancelConfirm() {
  clearTimeout(cancelConfirmTimer);
  statusline.classList.add("confirming");
  statusText.textContent = "Press Esc again to cancel";
  cancelConfirmTimer = setTimeout(() => {
    cancelConfirmTimer = undefined;
    statusline.classList.remove("confirming");
    // Only restore the working display if the turn is genuinely still going.
    if (inFlight) setStatusWorking();
    else setStatusReady();
  }, ESC_CONFIRM_MS) as unknown as number;
}

function disarmCancelConfirm() {
  clearTimeout(cancelConfirmTimer);
  cancelConfirmTimer = undefined;
  statusline.classList.remove("confirming");
}

function setSession(id: string) {
  statusMeta.textContent = id ? `session ${id}` : "";
}

// contextWindowSegment: "◔ used/window · NN%", graded green/amber/red at
// zero's own 75% and 90% thresholds, hidden until both figures are known.
function updateContextGauge(used: number) {
  const fill = contextFill(used, currentContextWindow);
  if (!fill) {
    contextGauge.hidden = true;
    return;
  }
  contextGauge.textContent = gaugeText(fill);
  contextGauge.className = `fill-${fill.level}`;
  contextGauge.hidden = false;
}

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

// switchConversation swaps which zero session turns go to. The outgoing
// transcript is saved and the incoming one restored, so each conversation
// keeps its own visible history.
function switchConversation(name: string) {
  const target = name.trim() || "main";
  if (target === activeConversation) {
    convInput.value = target;
    return;
  }
  saveTranscript();
  activeConversation = target;
  convInput.value = target;
  saveUIState();
  transcript.replaceChildren();
  if (!loadTranscript()) {
    appendBlock("welcome").textContent = `${welcomeText}\nconversation: ${target}`;
    saveTranscript();
  }
  systemMessage(`Switched conversation to ${target}`);
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
        "  /model [name]         Choose or switch active LLM model\n" +
        "  /provider [name]      Choose or switch model provider\n" +
        "  /conversation [name]  Switch conversation (own session + history)\n" +
        "  /help                 Show available commands\n" +
        "  /clear                Clear chat transcript",
    );
    return true;
  }
  if (text === "/clear") {
    transcript.replaceChildren();
    appendBlock("welcome").textContent = welcomeText;
    saveTranscript();
    return true;
  }
  if (text === "/conversation" || text.startsWith("/conversation ")) {
    const arg = text.slice("/conversation".length).trim();
    if (arg) {
      switchConversation(arg);
      return true;
    }
    const existing = await fetchConversations(authToken);
    const items = Object.entries(existing).map(([name, session]) => ({
      group: "Conversations",
      label: name,
      value: name,
      meta: name === activeConversation ? "current" : `session ${session}`,
      provider: "",
    }));
    const picked = await openPicker("Switch conversation", items);
    if (picked) switchConversation(picked.value);
    return true;
  }
  // /new mirrors the reset Telegram's /new performs: the backend session
  // mapping is dropped so the next turn starts with no context, and the
  // visible transcript is cleared to match.
  if (text === "/new") {
    try {
      await resetConversation(authToken, activeConversation);
    } catch (err) {
      fail(err instanceof Error ? err.message : String(err));
      return true;
    }
    transcript.replaceChildren();
    appendBlock("welcome").textContent = welcomeText;
    setSession("");
    saveTranscript();
    systemMessage(`Conversation reset. ${activeConversation} starts fresh.`);
    return true;
  }
  // /retry resends the last prompt rather than making you retype it. Slash
  // commands were never recorded in history, so this can only pick up a
  // real prompt.
  if (text === "/retry") {
    const last = inputHistory[inputHistory.length - 1];
    if (!last) {
      systemMessage("No previous prompt to retry.");
      return true;
    }
    promptInput.value = last;
    autoGrow();
    void sendTurn();
    return true;
  }
  if (text === "/copy") {
    const replies = transcript.querySelectorAll(".block.reply");
    const last = replies[replies.length - 1];
    if (!last?.textContent) {
      systemMessage("No reply to copy.");
      return true;
    }
    try {
      await navigator.clipboard.writeText(last.textContent);
      systemMessage("Copied the last reply to the clipboard.");
    } catch {
      // Clipboard access needs a secure context or permission; say so
      // rather than failing silently.
      fail("Could not access the clipboard.");
    }
    return true;
  }
  // /export hands the transcript over as a file. The page is sandboxed
  // enough that a blob download is the only way out of the tab.
  if (text === "/export") {
    const lines = Array.from(transcript.children).map((el) => el.textContent ?? "");
    const blob = new Blob([lines.join("\n\n")], { type: "text/plain" });
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = `zeroclaw-${activeConversation}-${new Date().toISOString().slice(0, 10)}.txt`;
    a.click();
    URL.revokeObjectURL(url);
    systemMessage("Exported the transcript.");
    return true;
  }
  if (text === "/beat") {
    try {
      await fireHeartbeat(authToken);
      systemMessage("Heartbeat fired. It runs in its own conversation; watch the daemon log.");
    } catch (err) {
      fail(err instanceof Error ? err.message : String(err));
    }
    return true;
  }
  if (text === "/status") {
    try {
      const s = await fetchStatus(authToken);
      systemMessage(
        [
          `agent         ${s.agent}`,
          `container     ${s.container}`,
          `daemon pid    ${s.pid}`,
          `conversations ${s.conversations}`,
          `provider      ${s.provider ?? "unknown"}`,
          `model         ${s.model ?? "unknown"}`,
        ].join("\n"),
      );
    } catch (err) {
      fail(err instanceof Error ? err.message : String(err));
    }
    return true;
  }
  if (text === "/theme" || text.startsWith("/theme ")) {
    const arg = text.slice("/theme".length).trim();
    if (arg) {
      const entry = applyTheme(arg);
      if (!entry) {
        fail(`Unknown theme: ${arg}`);
        return true;
      }
      currentTheme = entry.name;
      saveUIState();
      systemMessage(`Switched theme to ${entry.label}`);
      return true;
    }
    const previous = currentTheme;
    const items = THEMES.map((t) => ({
      group: t.isDark ? "Dark Themes" : "Light Themes",
      label: t.label,
      value: t.name,
      meta: t.name === currentTheme ? "current" : t.name,
      provider: "",
    }));
    // Preview as the selection moves, and restore on cancel, matching the
    // TUI's previewSelectedTheme / Esc behaviour.
    const picked = await openPicker("Choose a theme", items, (item) => {
      if (item) applyTheme(item.value);
    });
    if (picked) {
      currentTheme = picked.value;
      applyTheme(currentTheme);
      saveUIState();
      systemMessage(`Switched theme to ${picked.label}`);
    } else {
      applyTheme(previous);
    }
    return true;
  }
  if (text === "/model") {
    const items = await fetchModels(authToken, currentProvider ?? "gitlawb-opengateway");
    const picked = await openPicker("Choose a model", items);
    if (picked) {
      currentModel = picked.value;
      updateModelIndicator();
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
      updateModelIndicator();
      saveUIState();
      systemMessage(`Switched provider to ${currentProvider}`);
    }
    return true;
  }
  if (text.startsWith("/model ")) {
    currentModel = text.slice("/model ".length).trim() || currentModel;
    updateModelIndicator();
    saveUIState();
    systemMessage(`Switched model to ${currentModel}`);
    return true;
  }
  if (text.startsWith("/provider ")) {
    currentProvider = text.slice("/provider ".length).trim() || currentProvider;
    updateModelIndicator();
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
  { group: "Commands", label: "/conversation", value: "/conversation", meta: "Switch conversation", provider: "" },
  { group: "Commands", label: "/theme", value: "/theme", meta: "Choose a UI colour theme", provider: "" },
  { group: "Commands", label: "/status", value: "/status", meta: "Show agent, container, and model", provider: "" },
  { group: "Commands", label: "/new", value: "/new", meta: "Reset this conversation's session", provider: "" },
  { group: "Commands", label: "/retry", value: "/retry", meta: "Resend the last prompt", provider: "" },
  { group: "Commands", label: "/copy", value: "/copy", meta: "Copy the last reply", provider: "" },
  { group: "Commands", label: "/export", value: "/export", meta: "Download the transcript", provider: "" },
  { group: "Commands", label: "/beat", value: "/beat", meta: "Fire a heartbeat turn now", provider: "" },
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
  // An unrecognized command used to just hide the palette, which reads as
  // "the UI stopped responding" rather than "no such command".
  if (paletteItems.length === 0) {
    const empty = document.createElement("div");
    empty.className = "picker-empty";
    empty.textContent = "no matching command";
    palette.appendChild(empty);
    palette.hidden = false;
    return;
  }
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
        // The daemon reports what it actually used, which is the only way
        // an "auto" route resolves to a concrete model.
        if (ev.model) currentModel = ev.model;
        if (ev.provider) currentProvider = ev.provider;
        updateModelIndicator();
        if (ev.sessionId) setSession(ev.sessionId);
        if (thinking) thinking.textContent = `session ${ev.sessionId} · ${ev.provider} ${ev.model}`;
        break;
      case "usage":
        // zero reports the latest step's tokens; the gauge measures those
        // against the window rather than accumulating across the session.
        updateContextGauge(ev.promptTokens + ev.completionTokens);
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
  const conversation = activeConversation;

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
  setStatusWorking();
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
    // Drop any armed Esc confirmation with the turn it belonged to, so a
    // stray second Esc cannot cancel whatever runs next.
    disarmCancelConfirm();
    setSendButtonStopping(false);
    setStatusReady();
    thinkingEl.remove();
    saveTranscript();
    promptInput.focus();
  }
}

async function main() {
  const token = readToken();
  if (!token) {
    statusText.textContent = "no token";
    fail("No auth token found. Open this page with `zeroclaw web`, not a bare URL.");
    composer.querySelectorAll("input, textarea, button").forEach((el) => {
      (el as HTMLInputElement | HTMLTextAreaElement | HTMLButtonElement).disabled = true;
    });
    return;
  }
  authToken = token;
  loadUIState();
  applyTheme(currentTheme);
  updateModelIndicator();

  try {
    const status = await fetchStatus(authToken);
    statusMeta.textContent = `${status.agent} · ${status.container} · pid ${status.pid}`;
    // Show what a turn would use before one has run. An explicit /model or
    // /provider from a previous session wins over the backend default.
    currentProvider ??= status.provider;
    currentModel ??= status.model;
    // Only meaningful while the model is the one /status described; a later
    // /model switch clears it until the next turn or status refresh.
    if (currentModel === status.model) {
      currentContextLabel = status.contextWindowLabel ?? "";
      currentContextWindow = status.contextWindow ?? 0;
    }
    updateModelIndicator();
    setStatusReady();
  } catch (err) {
    statusText.textContent = "connection failed";
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
  // "change", not "input": switching on every keystroke would swap
  // transcripts mid-word. This fires on blur or Enter, once the name is
  // actually settled. The input is reset to the active conversation first
  // so switchConversation saves the outgoing transcript under the right key.
  convInput.addEventListener("change", () => {
    const target = convInput.value.trim() || "main";
    convInput.value = activeConversation;
    switchConversation(target);
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
  // "/" anywhere on the page focuses the composer and opens the palette, so
  // the commands are reachable without clicking into the input first. Skips
  // cases where the key is ordinary text: an already-focused field, or a
  // modifier chord the browser owns.
  // Esc cancels the running turn from anywhere on the page. The picker and
  // the command palette own Esc while they are open, so this defers to them
  // rather than cancelling a turn out from under a dismissal.
  document.addEventListener("keydown", (e) => {
    if (e.key !== "Escape") return;
    if (document.querySelector(".picker-overlay")) return;
    if (!palette.hidden) return;
    if (!inFlight) return;
    e.preventDefault();
    if (cancelConfirmActive()) {
      disarmCancelConfirm();
      inFlight.abort();
      return;
    }
    armCancelConfirm();
  });

  document.addEventListener("keydown", (e) => {
    if (e.key !== "/" || e.ctrlKey || e.metaKey || e.altKey) return;
    const active = document.activeElement;
    if (active instanceof HTMLInputElement || active instanceof HTMLTextAreaElement) return;
    if (document.querySelector(".picker-overlay")) return;
    e.preventDefault();
    promptInput.focus();
    promptInput.value = "/";
    autoGrow();
    updatePalette();
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
  // scrollHeight excludes the border under border-box, so using it directly
  // leaves the box a couple of pixels short and the browser shows a
  // scrollbar on a single-line input. Add the borders back, and only allow
  // scrolling once the content actually exceeds the max height.
  const style = getComputedStyle(promptInput);
  const borders = parseFloat(style.borderTopWidth) + parseFloat(style.borderBottomWidth);
  const needed = promptInput.scrollHeight + borders;
  const max = parseFloat(style.maxHeight);
  promptInput.style.height = `${needed}px`;
  promptInput.style.overflowY = Number.isFinite(max) && needed > max ? "auto" : "hidden";
}

void main();
