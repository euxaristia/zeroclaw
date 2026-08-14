import { expect, test } from "bun:test";
import { THEMES, type Palette } from "./themes";

// themes.ts is generated from internal/tui/theme_palettes.go. Nothing stops
// someone editing one and not the other, and a drifted palette is the kind
// of thing you only notice by eye, so this reads the Go source back and
// checks every value still matches.

const goSource = await Bun.file(
  new URL("../../internal/tui/theme_palettes.go", import.meta.url),
).text();

function goPalettes(): Record<string, Record<string, string>> {
  const out: Record<string, Record<string, string>> = {};
  for (const m of goSource.matchAll(/var (\w+)Palette = palette\{([\s\S]*?)\n\}/g)) {
    const fields: Record<string, string> = {};
    for (const f of m[2]!.matchAll(/(\w+):\s*"([^"]*)"/g)) {
      fields[f[1]!] = f[2]!;
    }
    out[m[1]!] = fields;
  }
  return out;
}

function goRegistry(): { name: string; label: string; variable: string; isDark: boolean }[] {
  return [
    ...goSource.matchAll(
      /\{Name:\s*"([^"]+)",\s*Label:\s*"([^"]+)",\s*Palette:\s*(\w+)Palette,\s*IsDark:\s*(true|false)\}/g,
    ),
  ].map((m) => ({ name: m[1]!, label: m[2]!, variable: m[3]!, isDark: m[4] === "true" }));
}

const registry = goRegistry();
const palettes = goPalettes();

test("parses the Go theme registry", () => {
  expect(registry.length).toBeGreaterThan(0);
});

test("exports exactly the themes the TUI registers", () => {
  expect(THEMES.map((t) => t.name)).toEqual(registry.map((r) => r.name));
});

for (const entry of registry) {
  test(`theme ${entry.name} matches the Go palette`, () => {
    const ours = THEMES.find((t) => t.name === entry.name);
    expect(ours).toBeDefined();
    expect(ours!.label).toBe(entry.label);
    expect(ours!.isDark).toBe(entry.isDark);

    const want = palettes[entry.variable];
    expect(want).toBeDefined();
    for (const [field, value] of Object.entries(want!)) {
      expect(ours!.palette[field as keyof Palette]).toBe(value);
    }
  });
}
