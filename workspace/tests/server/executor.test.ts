import { expect, it } from "vitest";
import { fileURLToPath } from "node:url";
import { CodexHost } from "../../src/server/executors/codex";
it("consumes native skill metadata and recovers a completed original turn without a new dispatch", async () => {
  const host = new CodexHost(process.execPath, [
    fileURLToPath(new URL("./fixtures/codex-rpc.mjs", import.meta.url)),
  ]);
  try {
    const skills = await host.skills("/project/backend");
    expect(skills[0].path).toBe("/canonical/skills/workspace-project/SKILL.md");
    expect(
      await host.readTask({
        threadId: "original-thread",
        turnId: "original-turn",
      }),
    ).toEqual({
      status: "completed",
      handle: "original-thread:original-turn",
      result: "原任务结果",
    });
  } finally {
    host.close();
  }
});
