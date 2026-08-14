// Context-fill gauge, ported from zero's internal/tui/view.go
// (contextFillPercent / contextWindowSegment / humanCount) so the web UI
// reports context pressure the same way and with the same thresholds.

// humanCount renders a token count the way the status line wants it: 999,
// 12.4K, 200K, 1M, 1.2M.
export function humanCount(n: number): string {
  if (n < 0) n = 0;
  if (n < 1000) return String(Math.trunc(n));
  const scaled = (value: number, suffix: string) => {
    const text = `${value.toFixed(1)}${suffix}`;
    return text.replace(`.0${suffix}`, suffix);
  };
  if (n < 1_000_000) return scaled(n / 1000, "K");
  return scaled(n / 1_000_000, "M");
}

export type FillLevel = "ok" | "warn" | "high";

export interface ContextFill {
  pct: number;
  used: number;
  window: number;
  level: FillLevel;
}

// contextFill grades usage against the window. Returns null when either
// figure is unknown, which is how the gauge stays hidden before the first
// turn rather than rendering a misleading zero.
export function contextFill(used: number, window: number): ContextFill | null {
  if (used <= 0 || window <= 0) return null;
  const ratio = Math.min(used / window, 1);
  // Thresholds match contextFillPercent: amber at 75%, red at 90%.
  const level: FillLevel = ratio >= 0.9 ? "high" : ratio >= 0.75 ? "warn" : "ok";
  return { pct: Math.floor(ratio * 100 + 0.5), used, window, level };
}

// gaugeText mirrors contextWindowSegment's "◔ used/window · NN%".
export function gaugeText(fill: ContextFill): string {
  return `◔ ${humanCount(fill.used)}/${humanCount(fill.window)} · ${fill.pct}%`;
}
