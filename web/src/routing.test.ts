import { expect, test } from "bun:test";
import {
  applyRunStart,
  COMMANDS,
  formatTitleModel,
  matchSlashCommands,
  planConversationItems,
  planConversations,
  planNextConversation,
} from "./routing";

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

test("planConversations highlights active conversation in place without moving to first position", () => {
  const convs = planConversations({ main: "sess-1", refactor: "sess-2" }, ["draft"], "refactor");
  expect(convs[0].name).toBe("main");
  expect(convs.map((c) => c.name)).toEqual(["main", "draft", "refactor"]);
  const refactor = convs.find((c) => c.name === "refactor");
  expect(refactor?.isCurrent).toBe(true);
  expect(refactor?.meta).toBe("current · session sess-2");
  expect(convs.find((c) => c.name === "main")?.isCurrent).toBe(false);
  expect(convs.find((c) => c.name === "main")?.meta).toBe("session sess-1");
  expect(convs.find((c) => c.name === "draft")?.meta).toBe("local");
});

test("planConversationItems prepends + New conversation action", () => {
  const items = planConversationItems({ main: "sess-1" }, [], "main");
  expect(items[0].value).toBe("__new__");
  expect(items[0].group).toBe("Actions");
  expect(items.some((i) => i.value === "main" && i.group === "Conversations")).toBe(true);
});

test("planNextConversation selects next adjacent conversation in place", () => {
  const names = ["main", "fresh-recall", "heartbeat", "usageprobe", "usageprobe2", "usageprobe4"];
  expect(planNextConversation(names, "usageprobe2")).toBe("usageprobe4");
  expect(planNextConversation(names, "heartbeat")).toBe("usageprobe");
  expect(planNextConversation(names, "main")).toBe("fresh-recall");
});

test("planNextConversation selects preceding conversation when last item deleted", () => {
  const names = ["main", "fresh-recall", "heartbeat", "usageprobe", "usageprobe2", "usageprobe4"];
  expect(planNextConversation(names, "usageprobe4")).toBe("usageprobe2");
});

test("planNextConversation falls back to main when unknown or only item", () => {
  expect(planNextConversation(["main"], "main")).toBe("main");
  expect(planNextConversation(["main", "draft"], "unknown")).toBe("main");
  expect(planNextConversation([], "anything")).toBe("main");
});

