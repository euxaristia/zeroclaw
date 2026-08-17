import { expect, test } from "bun:test";
import { applyRunStart, COMMANDS, formatTitleModel, matchSlashCommands } from "./routing";

test("/auth is a first-class palette command", () => {
  expect(COMMANDS.some((c) => c.value === "/auth")).toBe(true);
  const hits = matchSlashCommands("/auth");
  expect(hits.map((c) => c.value)).toEqual(["/auth"]);
});

test("palette query auth does not collapse to no matching command", () => {
  const hits = matchSlashCommands("/au");
  expect(hits.some((c) => c.value === "/auth")).toBe(true);
});

test("applyRunStart keeps the selected profile when zero reports the API kind", () => {
  const next = applyRunStart(
    { provider: "gitlawb-opengateway", model: "nvidia/nemotron-3-ultra-550b-a55b:free" },
    { provider: "openai", model: "nvidia/nemotron-3-ultra-550b-a55b:free" },
  );
  expect(next.provider).toBe("gitlawb-opengateway");
  expect(next.model).toBe("nvidia/nemotron-3-ultra-550b-a55b:free");
});

test("applyRunStart still learns the routed model when none was picked", () => {
  const next = applyRunStart({ provider: "gitlawb-opengateway" }, { provider: "openai", model: "mimo-v2.5-pro" });
  expect(next.provider).toBe("gitlawb-opengateway");
  expect(next.model).toBe("mimo-v2.5-pro");
});

test("title bar does not glue profile and slashed model ids together", () => {
  expect(formatTitleModel("openai", "nvidia/nemotron-3-ultra-550b-a55b:free")).toBe(
    "openai · nvidia/nemotron-3-ultra-550b-a55b:free",
  );
  expect(formatTitleModel("", "nvidia/nemotron-3-ultra-550b-a55b:free")).toBe(
    "nvidia/nemotron-3-ultra-550b-a55b:free",
  );
  expect(formatTitleModel("", "")).toBe("no provider");
});
