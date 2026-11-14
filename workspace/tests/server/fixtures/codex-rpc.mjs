import { createInterface } from "node:readline";
const reader = createInterface({ input: process.stdin });
reader.on("line", (line) => {
  const m = JSON.parse(line);
  if (m.id === undefined) return;
  let result;
  if (m.method === "initialize") result = { userAgent: "fixture" };
  else if (m.method === "skills/list")
    result = {
      data: m.params.cwds.map((cwd) => ({
        cwd,
        skills: [
          {
            name: "workspace-project",
            path: "/canonical/skills/workspace-project/SKILL.md",
            enabled: true,
          },
        ],
        errors: [],
      })),
    };
  else if (m.method === "thread/resume")
    result = { thread: { id: m.params.threadId, turns: [] } };
  else if (m.method === "thread/read")
    result = {
      thread: {
        id: m.params.threadId,
        turns: [
          {
            id: "original-turn",
            status: "completed",
            items: [{ type: "agentMessage", text: "原任务结果" }],
          },
        ],
      },
    };
  else {
    process.stdout.write(
      JSON.stringify({
        id: m.id,
        error: { code: -32601, message: "Unexpected dispatch" },
      }) + "\n",
    );
    return;
  }
  process.stdout.write(JSON.stringify({ id: m.id, result }) + "\n");
});
