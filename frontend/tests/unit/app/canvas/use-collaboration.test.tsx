import { act, renderHook, waitFor } from "@testing-library/react";
import { expect, it, vi } from "vitest";
import { useCanvasCollaboration } from "@/app/canvas/use-collaboration";
import { createResourceNode, type Canvas } from "@/app/canvas/document";
const transport = vi.hoisted(() => ({ callbacks: vi.fn(), send: vi.fn() }));
vi.mock("@/app/canvas/live", () => ({
  CanvasLive: class {
    constructor(
      _id: string,
      private callbacks: ConstructorParameters<
        typeof import("@/app/canvas/live").CanvasLive
      >[1],
    ) {
      transport.callbacks(callbacks);
    }
    start() {
      this.callbacks.collaboration?.actor("me");
      this.callbacks.status("live", true);
    }
    stop() {}
    sendCollaboration(...args: unknown[]) {
      transport.send(...args);
    }
  },
}));
it("retains the displayed branch immediately when P prediction becomes invalid, before terminal receipt", async () => {
  const id = crypto.randomUUID(),
    project = crypto.randomUUID(),
    node = createResourceNode("text", { x: 0, y: 0 });
  node.config.text = "AB";
  const initial: Canvas = {
    id,
    project_id: project,
    revision: 1,
    nodes: [node],
    edges: [],
  };
  let shown = initial;
  const hook = renderHook(() =>
    useCanvasCollaboration(initial, true, {
      update: (d) => {
        shown = d;
      },
      status: () => {},
      revoked: () => {},
      message: () => {},
    }),
  );
  const desired = structuredClone(initial);
  desired.nodes[0].config.text = "*AB*";
  act(() => {
    hook.result.current.edit(desired, { markdownSource: new Set([node.id]) });
    void hook.result.current.flush().catch(() => {});
  });
  await waitFor(() => expect(transport.send).toHaveBeenCalledOnce());
  const pending = transport.send.mock.calls[0][0];
  const remote = {
    type: "set",
    node_id: node.id,
    path: ["config", "text_source"],
    value: { text: "**AB**" },
    expected: { text: "AB" },
    expected_exists: true,
  };
  act(() =>
    transport.callbacks.mock.lastCall![0].collaboration.changes([
      {
        revision: 2,
        actor_id: "other",
        op_id: crypto.randomUUID(),
        operations: [remote],
        effects: [
          {
            entity: "node",
            id: node.id,
            path: ["config", "text"],
            index: 0,
            after_exists: true,
            after: "**AB**",
          },
        ],
      },
    ]),
  );
  expect(shown.nodes[0].config.text).toBe("*AB*");
  act(() =>
    transport.callbacks.mock.lastCall![0].collaboration.receipt({
      status: "rejected",
      op_id: pending.key,
      code: "field_conflict",
    }),
  );
  expect(shown.nodes[0].config.text).toBe("*AB*");
  expect(
    hook.result.current.controller.current?.base.nodes[0].config.text,
  ).toBe("**AB**");
  hook.unmount();
});
