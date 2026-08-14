import { expect, test } from "bun:test";
import { humanCount, contextFill, gaugeText } from "./usage";

// Values pinned against zero's internal/tui/view.go so the two cannot drift.

test("humanCount matches zero's formatting", () => {
  expect(humanCount(999)).toBe("999");
  expect(humanCount(12_400)).toBe("12.4K");
  expect(humanCount(200_000)).toBe("200K");
  expect(humanCount(1_000_000)).toBe("1M");
  expect(humanCount(1_200_000)).toBe("1.2M");
  expect(humanCount(-5)).toBe("0");
});

// The case zero's own TestContextWindowSegment pins: 161k against a 200k
// window reads as 161K/200K · 81%.
test("gauge reproduces zero's documented example", () => {
  const fill = contextFill(161_000, 200_000);
  expect(fill).not.toBeNull();
  expect(gaugeText(fill!)).toBe("◔ 161K/200K · 81%");
});

test("hidden until both figures are known", () => {
  expect(contextFill(0, 200_000)).toBeNull();
  expect(contextFill(1000, 0)).toBeNull();
});

test("grades at zero's thresholds", () => {
  expect(contextFill(50_000, 100_000)!.level).toBe("ok");
  expect(contextFill(74_000, 100_000)!.level).toBe("ok");
  expect(contextFill(75_000, 100_000)!.level).toBe("warn");
  expect(contextFill(89_000, 100_000)!.level).toBe("warn");
  expect(contextFill(90_000, 100_000)!.level).toBe("high");
});

test("clamps past a full window instead of exceeding 100%", () => {
  const fill = contextFill(250_000, 200_000)!;
  expect(fill.pct).toBe(100);
  expect(fill.level).toBe("high");
});
