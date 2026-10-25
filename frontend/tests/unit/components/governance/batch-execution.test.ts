import { expect, it, vi } from "vitest";
import {
  executeBatch,
  type BatchEntry,
} from "@/components/governance/batch-execution";
import { RequestError } from "@/lib/request";
const item = (
  id: string,
  status: BatchEntry<{ id: string }>["status"] = "ready",
): BatchEntry<{ id: string }> => ({
  id,
  row: { id },
  label: id,
  requestId: `request-${id}`,
  status,
  message: "",
});
it("continues after a known failure, never reruns successful or unknown records", async () => {
  const entries = [
    item("done", "success"),
    item("lost", "unknown"),
    item("stale"),
    item("good"),
  ];
  const run = vi.fn(async (entry: BatchEntry<{ id: string }>) => {
    if (entry.id === "stale") throw new RequestError("revision_conflict", 409);
    return { message: "已保存" };
  });
  const changed: BatchEntry<{ id: string }>[] = [];
  await executeBatch(
    entries,
    run,
    (e) => changed.push(e),
    new AbortController().signal,
  );
  expect(run.mock.calls.map(([e]) => e.id)).toEqual(["stale", "good"]);
  expect(
    changed.filter((e) => e.status !== "running").map((e) => e.status),
  ).toEqual(["failed", "success"]);
});
it("network uncertainty keeps the same request identity and is not counted as failure", async () => {
  const changed: BatchEntry<{ id: string }>[] = [];
  await executeBatch(
    [item("lost")],
    async () => {
      throw new Error("network");
    },
    (e) => changed.push(e),
    new AbortController().signal,
  );
  expect(changed.at(-1)).toMatchObject({
    status: "unknown",
    requestId: "request-lost",
  });
});
it("stops queued writes when authorization is lost", async () => {
  const run = vi.fn(async () => {
    throw new RequestError("forbidden", 403);
  });
  const changed: BatchEntry<{ id: string }>[] = [];
  await executeBatch(
    [item("a"), item("b")],
    run,
    (e) => changed.push(e),
    new AbortController().signal,
  );
  expect(run).toHaveBeenCalledTimes(1);
  expect(changed.at(-1)).toMatchObject({ id: "b", status: "skipped" });
});
