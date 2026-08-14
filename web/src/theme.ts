import { THEMES, type ThemeEntry, type Palette } from "./themes";

// Palette values are pushed onto :root as custom properties, so style.css
// names colours the same way internal/tui/theme.go does (ink, muted, faint,
// accent, line, ...) instead of hardcoding hex anywhere.
export function applyPalette(p: Palette) {
  const root = document.documentElement.style;
  root.setProperty("--panel", p.panel);
  root.setProperty("--prompt-bg", p.promptBg);
  root.setProperty("--line", p.line);
  root.setProperty("--line2", p.line2);
  root.setProperty("--ink", p.ink);
  root.setProperty("--muted", p.muted);
  root.setProperty("--faint", p.faint);
  root.setProperty("--faintest", p.faintest);
  root.setProperty("--accent", p.accent);
  root.setProperty("--green", p.green);
  root.setProperty("--red", p.red);
  root.setProperty("--amber", p.amber);
  root.setProperty("--blue", p.blue);
  root.setProperty("--on-accent", p.onAccent);
  root.setProperty("--card-run", p.cardRun);
  root.setProperty("--card-err", p.cardErr);
}

export function lookupTheme(name: string): ThemeEntry | undefined {
  const q = name.trim().toLowerCase();
  return THEMES.find((t) => t.name === q || t.label.toLowerCase() === q);
}

export function defaultTheme(): ThemeEntry {
  return THEMES[0]!;
}

export function applyTheme(name: string): ThemeEntry | undefined {
  const entry = lookupTheme(name);
  if (entry) {
    applyPalette(entry.palette);
    document.documentElement.style.setProperty("color-scheme", entry.isDark ? "dark" : "light");
  }
  return entry;
}
