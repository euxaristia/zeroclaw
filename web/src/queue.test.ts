import { expect, test } from "bun:test";
import { createPromptQueue, submitIntent } from "./queue";

test("empty box while a turn runs is Stop, not a new turn", () => {
  expect(submitIntent(true, "")).toBe("stop");
  expect(submitIntent(true, "   ")).toBe("stop");
});

test("typed text while a turn runs queues instead of aborting", () => {
  expect(submitIntent(true, "also check the tests")).toBe("queue");
});

test("typed text while idle sends", () => {
  expect(submitIntent(false, "hello")).toBe("send");
  expect(submitIntent(false, "")).toBe("ignore");
});

test("queue is FIFO and per conversation", () => {
  const q = createPromptQueue();
  q.enqueue("main", "one");
  q.enqueue("main", "two");
  q.enqueue("work", "other");
  expect(q.size("main")).toBe(2);
  expect(q.dequeue("main")).toBe("one");
  expect(q.dequeue("main")).toBe("two");
  expect(q.dequeue("main")).toBeUndefined();
  expect(q.dequeue("work")).toBe("other");
});

test("clear drops only that conversation", () => {
  const q = createPromptQueue();
  q.enqueue("main", "keep going");
  q.enqueue("work", "stay");
  q.clear("main");
  expect(q.size("main")).toBe(0);
  expect(q.dequeue("work")).toBe("stay");
});
