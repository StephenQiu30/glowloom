import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import {
  CanvasTransferDialog,
  CanvasTransferReceiver,
} from "@/components/canvas/editor/canvas-transfer-dialog";
import {
  beginCanvasTransfer,
  clearCanvasTransfer,
  pendingCanvasTransfer,
} from "@/components/canvas/lib/canvas-transfer-state";
import { createCanvasNode } from "@/components/canvas/lib/canvas-node-factory";
import type { CanvasYjsLiveSession } from "@/components/canvas/yjs-live-session";
const api = vi.hoisted(() => ({
  get: vi.fn(),
  create: vi.fn(),
  list: vi.fn(),
  navigate: vi.fn(),
  success: vi.fn(),
}));
vi.mock("@/api/canvases", () => ({
  getCanvas: api.get,
  createCanvas: api.create,
  listCanvases: api.list,
}));
vi.mock("next/navigation", () => ({
  useRouter: () => ({ push: api.navigate }),
}));
vi.mock("@/components/auth/session", () => ({
  useSession: () => ({ session_id: "transfer-session" }),
}));
vi.mock("sonner", () => ({ toast: { success: api.success } }));
const owner = {
  actorID: "actor",
  sessionID: "transfer-session",
  projectID: "project",
  scopeID: "scope",
};
const target: API.DocumentDTO = {
  id: "target",
  name: "目标画布",
  project_id: "project",
  permissions: { read: true, write: true },
};
afterEach(() => {
  cleanup();
  const intent = pendingCanvasTransfer(owner);
  if (intent) clearCanvasTransfer(intent);
  vi.clearAllMocks();
});
function fixture() {
  const node = createCanvasNode("subject", { x: 0, y: 0 });
  const selection = { nodes: [node], connections: [] };
  const source: API.DocumentDTO = {
    id: "source",
    project_id: "project",
    permissions: { read: true, write: true },
    nodes: [{ id: node.id }],
  };
  const graph = {
    nodes: [],
    connections: [],
    viewport: { x: 0, y: 0, zoom: 1 },
  };
  const snapshot = {
    document: graph,
    writable: true,
    dirty: false,
    migration: false,
  };
  const change = vi.fn();
  const waitForSaved = vi.fn().mockResolvedValue(target);
  const client = {
    subscribe: () => () => {},
    getSnapshot: () => snapshot,
    change,
    canAccept: () => true,
    waitForSaved,
  } as unknown as CanvasYjsLiveSession;
  return { node, selection, source, client, change, waitForSaved };
}
it("opens only a writable same-project target after reading source and target", async () => {
  const f = fixture();
  api.list.mockResolvedValue({
    items: [
      target,
      {
        ...target,
        id: "readonly",
        name: "只读目标",
        permissions: { read: true, write: false },
      },
    ],
  });
  api.get.mockImplementation(({ id }) =>
    Promise.resolve(id === "source" ? f.source : target),
  );
  render(
    <CanvasTransferDialog
      {...owner}
      client={f.client}
      canvasID="source"
      selection={f.selection}
      onClose={vi.fn()}
    />,
  );
  expect(
    await screen.findByRole("button", { name: "只读目标 · 无编辑权限" }),
  ).toHaveProperty("disabled", true);
  fireEvent.click(screen.getByRole("button", { name: "目标画布" }));
  await waitFor(() =>
    expect(api.navigate).toHaveBeenCalledWith(
      "/canvas?project_id=project&canvas_id=target",
    ),
  );
  expect(api.get).toHaveBeenCalledWith({ id: "source" });
  expect(pendingCanvasTransfer(owner)?.targetID).toBe("target");
  expect(f.change).not.toHaveBeenCalled();
});
it("retains the original identities after an unknown save and verifies without inserting twice", async () => {
  const f = fixture();
  const intent = beginCanvasTransfer(owner, "source", target, f.selection);
  f.waitForSaved
    .mockRejectedValueOnce(new Error("保存尚未确认"))
    .mockResolvedValue(target);
  api.get.mockImplementation(({ id }) =>
    Promise.resolve(
      id === "source"
        ? f.source
        : {
            ...target,
            nodes: intent.copy.nodes.map((n) => ({
              id: n.id,
              node_type: n.type,
              title: n.title,
              config: n.serviceData?.config,
              input_revision: 1,
            })),
            edges: [],
          },
    ),
  );
  render(
    <CanvasTransferReceiver {...owner} client={f.client} canvasID="target" />,
  );
  expect(f.change).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole("button", { name: "保存所选内容" }));
  await screen.findByText("保存尚未确认");
  expect(f.change).toHaveBeenCalledTimes(1);
  fireEvent.click(screen.getByRole("button", { name: "核验原保存" }));
  await waitFor(() =>
    expect(api.success).toHaveBeenCalledWith("所选内容已保存到当前画布"),
  );
  expect(f.change).toHaveBeenCalledTimes(1);
  expect(pendingCanvasTransfer(owner)).toBeNull();
});
it("does not mutate the target after source read permission is revoked", async () => {
  const f = fixture();
  beginCanvasTransfer(owner, "source", target, f.selection);
  api.get.mockResolvedValue({
    ...f.source,
    permissions: { read: false, write: false },
  });
  render(
    <CanvasTransferReceiver {...owner} client={f.client} canvasID="target" />,
  );
  fireEvent.click(screen.getByRole("button", { name: "保存所选内容" }));
  await screen.findByRole("alert");
  expect(f.change).not.toHaveBeenCalled();
  expect(api.success).not.toHaveBeenCalled();
});

it("reuses the original create key after an unknown target-creation result", async () => {
  const f = fixture();
  api.list.mockResolvedValue({ items: [] });
  api.get.mockImplementation(({ id }) =>
    Promise.resolve(id === "source" ? f.source : target),
  );
  api.create
    .mockRejectedValueOnce(new Error("创建尚未确认"))
    .mockResolvedValue(target);
  render(
    <CanvasTransferDialog
      {...owner}
      client={f.client}
      canvasID="source"
      selection={f.selection}
      onClose={vi.fn()}
    />,
  );
  fireEvent.change(screen.getByRole("textbox", { name: "新目标画布名称" }), {
    target: { value: "跨画布一致性验收-20261029" },
  });
  fireEvent.click(screen.getByRole("button", { name: "新建并打开" }));
  await screen.findByText("创建尚未确认");
  expect(
    screen.getByRole("textbox", { name: "新目标画布名称" }),
  ).toHaveProperty("disabled", true);
  fireEvent.click(screen.getByRole("button", { name: "核验创建结果" }));
  await waitFor(() =>
    expect(api.navigate).toHaveBeenCalledWith(
      "/canvas?project_id=project&canvas_id=target",
    ),
  );
  expect(api.create).toHaveBeenCalledTimes(2);
  expect(api.create.mock.calls[1]).toEqual(api.create.mock.calls[0]);
  expect(api.create.mock.calls[0][0]).toEqual({ pid: "project" });
});
