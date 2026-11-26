import { describe, expect, it, vi } from "vitest";
import { listAgentSessions } from "@/api/agent";
import { findCanvasConversation } from "@/components/canvas/assistant/canvas-conversation";

vi.mock("@/api/agent", () => ({ listAgentSessions: vi.fn() }));

const owner = {
  projectID: "project",
  scopeID: "scope",
  actorID: "actor",
  canvasID: "canvas",
};
const session = {
  id: "session",
  actor_id: owner.actorID,
  scope_id: owner.scopeID,
  project_id: owner.projectID,
  canvas_id: owner.canvasID,
  org_id: "org",
  execution_revision: 1,
  title: "故事板对话",
};

describe("findCanvasConversation", () => {
  it("恢复此人、空间、项目和画布共同匹配的会话", async () => {
    const signal = new AbortController().signal;
    vi.mocked(listAgentSessions)
      .mockResolvedValueOnce({
        items: [
          { ...session, id: "other-person", actor_id: "other" },
          { ...session, id: "other-scope", scope_id: "other" },
          { ...session, id: "other-project", project_id: "other" },
          { ...session, id: "other-canvas", canvas_id: "other" },
          { ...session, id: "unbound", canvas_id: undefined },
        ],
        next_cursor: "next",
      })
      .mockResolvedValueOnce({ items: [session] });

    await expect(findCanvasConversation(owner, signal)).resolves.toBe(
      "session",
    );
    expect(listAgentSessions).toHaveBeenLastCalledWith(
      { pid: "project", limit: 50, cursor: "next" },
      { signal },
    );
  });
});
