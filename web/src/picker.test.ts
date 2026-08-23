import { expect, test } from "bun:test";
import { activeDescendantId, planRows } from "./picker";
import type { CatalogItem } from "./api";

function item(group: string, label: string): CatalogItem {
  return { group, label, value: label.toLowerCase(), meta: "", provider: "openai" };
}

test("option ids follow the filtered index, not the position in the list", () => {
  // Two group headers are interleaved with three options, so the second
  // option sits at DOM position 3 while its selectable index is 1. Deriving
  // aria-activedescendant from the rendered position would point at the wrong
  // row, or at a header.
  const entries = planRows([item("A", "one"), item("B", "two"), item("B", "three")], 1, "p1");
  expect(entries.map((e) => e.kind)).toEqual(["group", "option", "group", "option", "option"]);
  const options = entries.filter((e) => e.kind === "option");
  expect(options.map((o) => o.id)).toEqual(["p1-opt-0", "p1-opt-1", "p1-opt-2"]);
  expect(options.map((o) => o.selected)).toEqual([false, true, false]);
});

test("a repeated group emits one header", () => {
  const entries = planRows([item("A", "one"), item("A", "two")], 0, "p1");
  expect(entries.filter((e) => e.kind === "group")).toHaveLength(1);
});

test("activeDescendantId names the highlighted row", () => {
  const entries = planRows([item("A", "one"), item("A", "two")], 1, "p2");
  expect(activeDescendantId(entries)).toBe("p2-opt-1");
});

test("activeDescendantId is empty when the filter matched nothing", () => {
  expect(activeDescendantId(planRows([], 0, "p3"))).toBe("");
});

test("id prefixes keep two pickers from colliding", () => {
  const a = activeDescendantId(planRows([item("A", "one")], 0, "picker-1"));
  const b = activeDescendantId(planRows([item("A", "one")], 0, "picker-2"));
  expect(a).not.toBe(b);
});
