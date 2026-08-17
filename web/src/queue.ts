export type SubmitIntent = "send" | "queue" | "stop" | "ignore";

// submitIntent is what the composer should do on Enter/click. A non-empty
// prompt during a run is a follow-up, not a cancel: the previous Send=Stop
// collapse made every nudge abort the turn.
export function submitIntent(inFlight: boolean, prompt: string): SubmitIntent {
  if (!prompt.trim()) return inFlight ? "stop" : "ignore";
  return inFlight ? "queue" : "send";
}

// Per-conversation FIFO of prompts typed while a turn is already running.
export function createPromptQueue() {
  const pending = new Map<string, string[]>();
  return {
    enqueue(conversation: string, prompt: string) {
      const q = pending.get(conversation) ?? [];
      q.push(prompt);
      pending.set(conversation, q);
    },
    dequeue(conversation: string): string | undefined {
      const q = pending.get(conversation);
      if (!q?.length) return undefined;
      const next = q.shift();
      if (!q.length) pending.delete(conversation);
      return next;
    },
    clear(conversation: string) {
      pending.delete(conversation);
    },
    size(conversation: string): number {
      return pending.get(conversation)?.length ?? 0;
    },
  };
}
