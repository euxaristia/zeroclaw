import type { CatalogItem } from "./api";

// Slash commands the web UI implements. Kept here so the palette filter can
// be tested without booting the page: a missing /auth is how the command
// became invisible after the handler landed.
export const COMMANDS: CatalogItem[] = [
  { group: "Commands", label: "/model", value: "/model", meta: "Choose or switch active LLM model", provider: "" },
  { group: "Commands", label: "/provider", value: "/provider", meta: "Choose or switch model provider", provider: "" },
  { group: "Commands", label: "/auth", value: "/auth", meta: "Store an API key for a provider", provider: "" },
  { group: "Commands", label: "/conversation", value: "/conversation", meta: "Switch conversation", provider: "" },
  { group: "Commands", label: "/theme", value: "/theme", meta: "Choose a UI colour theme", provider: "" },
  { group: "Commands", label: "/status", value: "/status", meta: "Show agent, container, and model", provider: "" },
  { group: "Commands", label: "/effort", value: "/effort", meta: "Set reasoning effort", provider: "" },
  { group: "Commands", label: "/turns", value: "/turns", meta: "Set the tool-turn budget", provider: "" },
  { group: "Commands", label: "/new", value: "/new", meta: "Reset this conversation's session", provider: "" },
  { group: "Commands", label: "/retry", value: "/retry", meta: "Resend the last prompt", provider: "" },
  { group: "Commands", label: "/copy", value: "/copy", meta: "Copy the last reply", provider: "" },
  { group: "Commands", label: "/export", value: "/export", meta: "Download the transcript", provider: "" },
  { group: "Commands", label: "/beat", value: "/beat", meta: "Fire a heartbeat turn now", provider: "" },
  { group: "Commands", label: "/help", value: "/help", meta: "Show available commands", provider: "" },
  { group: "Commands", label: "/clear", value: "/clear", meta: "Clear chat transcript", provider: "" },
];

// matchSlashCommands is the live palette filter: query is the composer text,
// including the leading slash. Unknown input hides the list (the empty
// state is "no matching command").
export function matchSlashCommands(input: string, commands = COMMANDS): CatalogItem[] {
  if (!input.startsWith("/") || input.includes("\n")) return [];
  const q = input.slice(1).toLowerCase();
  return commands.filter((c) => `${c.label} ${c.meta}`.toLowerCase().includes(q));
}

export type RouteState = {
  provider?: string;
  model?: string;
};

// applyRunStart updates local route state from a backend run_start event.
// Zero's run_start.provider is the API kind (openai for any OpenAI-compatible
// profile), not the profile name we send as ZERO_PROVIDER. Adopting it as
// the next turn's override is how a gitlawb-opengateway turn flipped to
// platform.openai.com on the message after.
export function applyRunStart(current: RouteState, ev: RouteState): RouteState {
  const model = ev.model?.trim() || current.model;
  return { provider: current.provider, model };
}

// formatTitleModel is the title-bar "who is answering" string. A slash
// between the two is how openai + nvidia/nemotron... read as one OpenRouter
// model id; the middle dot keeps the profile and the model id apart.
export function formatTitleModel(provider?: string, model?: string): string {
  const p = provider?.trim() ?? "";
  const m = model?.trim() ?? "";
  if (!p && !m) return "no provider";
  if (p && m) return `${p} · ${m}`;
  return m || p;
}
