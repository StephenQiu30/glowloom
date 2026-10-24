import { afterEach, expect, it, vi } from "vitest";
import { pageBusinessMemory } from "@/lib/page-business-memory";
import {
  claimWorkflowCommand,
  clearWorkflowCommand,
  readWorkflowCommand,
} from "@/components/canvas/lib/canvas-workflow-journal";
afterEach(() => {
  pageBusinessMemory.clear();
  vi.unstubAllGlobals();
});
it("page-local workflow retries retain the original command without Web Locks or browser storage", async () => {
  vi.stubGlobal("navigator", {});
  const request = {
    commandId: "one",
    canvasId: "1",
    nodeIds: ["2"],
    expectedRevision: "1",
    expectedProjectBindingRevision: "1",
    expectedEpoch: "00000000-0000-4000-8000-000000000001",
    expectedCommitSeq: "0",
    expectedGraphHash: "a".repeat(64),
  };
  await claimWorkflowCommand("workflow:test", "1", request, true);
  await claimWorkflowCommand("workflow:test", "1", request, false);
  await expect(
    claimWorkflowCommand(
      "workflow:test",
      "1",
      { ...request, commandId: "two" },
      true,
    ),
  ).rejects.toThrow("本页运行命令已更新");
  await clearWorkflowCommand("workflow:test", "1", "old");
  expect(readWorkflowCommand("workflow:test", "1")).toEqual(request);
  await clearWorkflowCommand("workflow:test", "1", "one");
  expect(readWorkflowCommand("workflow:test", "1")).toBeNull();
});
