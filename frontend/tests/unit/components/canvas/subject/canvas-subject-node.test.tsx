import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { CanvasSubjectNode } from "@/components/canvas/subject/canvas-subject-node";
import { CanvasMediaSelectionContext } from "@/components/canvas/lib/canvas-media-selection";
import { registerBuiltinNodes } from "@/components/canvas/nodes/builtin-nodes";
import { createCanvasNode } from "@/components/canvas/lib/canvas-node-factory";
import { subjectConfig } from "@/components/canvas/subject/subject-node-state";
import type {
  CanvasYjsLiveSession,
  YjsView,
} from "@/components/canvas/yjs-live-session";
const api = vi.hoisted(() => ({ preview: vi.fn() }));
vi.mock("@/api/media", () => ({ getMediaPreview: api.preview }));
vi.mock("next/dynamic", () => ({ default: () => () => null }));
vi.mock("@/components/feedback/feedback-toast", () => ({
  FeedbackToast: ({ message }: { message: string }) => (
    <p role="alert">{message}</p>
  ),
}));
registerBuiltinNodes();
afterEach(() => {
  cleanup();
  api.preview.mockReset();
});
function setup(
  select = vi.fn(
    async () => null as ReturnType<typeof createCanvasNode> | null,
  ),
) {
  const node = createCanvasNode("subject", { x: 0, y: 0 });
  let view = {
    writable: true,
    migration: false,
    document: {
      nodes: [node],
      connections: [],
      viewport: { x: 0, y: 0, k: 1 },
      backgroundMode: "dots",
      showImageInfo: false,
    },
  } as unknown as YjsView;
  let epoch = "first";
  const listeners = new Set<() => void>();
  const client = {
    subscribe: (f: () => void) => {
      listeners.add(f);
      return () => listeners.delete(f);
    },
    getSnapshot: () => view,
    getDocumentEpoch: () => epoch,
    canAccept: () => view.writable && !view.migration,
    waitForSaved: vi.fn(async () => {}),
    change: vi.fn((document: NonNullable<YjsView["document"]>) => {
      view = { ...view, document };
      listeners.forEach((f) => f());
    }),
  };
  const pid = "11111111-1111-4111-8111-111111111111";
  render(
    <CanvasMediaSelectionContext.Provider value={select}>
      <CanvasSubjectNode
        client={client as unknown as CanvasYjsLiveSession}
        nodeID={node.id}
        actorID="actor"
        scopeID="scope"
        projectID={pid}
      />
    </CanvasMediaSelectionContext.Provider>,
  );
  return {
    client,
    node,
    pid,
    select,
    foreign: () => {
      view = {
        ...view,
        document: {
          ...view.document!,
          nodes: view.document!.nodes.map((n) => ({
            ...n,
            title: "协作者的新名称",
          })),
        },
      };
      listeners.forEach((f) => f());
    },
    epoch: () => {
      epoch = "second";
    },
    addSource: (source: ReturnType<typeof createCanvasNode>) => {
      view = {
        ...view,
        document: {
          ...view.document!,
          nodes: [...view.document!.nodes, source],
        },
      };
      listeners.forEach((f) => f());
    },
    current: () => view.document!.nodes[0],
  };
}
it("卡片名称和描述各写入一次原节点，Escape不保存，输入法Enter不提交", async () => {
  const h = setup();
  fireEvent.click(screen.getByRole("button", { name: "编辑主体名称" }));
  const input = screen.getByRole("textbox", { name: "主体名称" });
  fireEvent.change(input, { target: { value: "测试主体" } });
  fireEvent.keyDown(input, { key: "Enter", keyCode: 229, isComposing: true });
  expect(h.client.change).not.toHaveBeenCalled();
  fireEvent.keyDown(input, { key: "Enter" });
  await waitFor(() => expect(h.current().title).toBe("测试主体"));
  await waitFor(() =>
    expect(screen.queryByRole("textbox", { name: "主体名称" })).toBeNull(),
  );
  expect(h.current().id).toBe(h.node.id);
  expect(h.client.change).toHaveBeenCalledTimes(1);
  fireEvent.click(screen.getByRole("button", { name: "编辑主体描述" }));
  let description = screen.getByRole("textbox", { name: "主体描述" });
  fireEvent.change(description, { target: { value: "取消的描述" } });
  fireEvent.keyDown(description, { key: "Escape" });
  expect(h.client.change).toHaveBeenCalledTimes(1);
  fireEvent.click(screen.getByRole("button", { name: "编辑主体描述" }));
  description = screen.getByRole("textbox", { name: "主体描述" });
  fireEvent.change(description, { target: { value: "角色说明" } });
  fireEvent.blur(description);
  await waitFor(() =>
    expect(subjectConfig(h.current()).local?.description).toBe("角色说明"),
  );
  expect(h.client.change).toHaveBeenCalledTimes(2);
});
it("并行改名时保留输入且不覆盖新内容", async () => {
  const h = setup();
  fireEvent.click(screen.getByRole("button", { name: "编辑主体描述" }));
  const field = screen.getByRole("textbox", { name: "主体描述" });
  fireEvent.change(field, { target: { value: "我的说明" } });
  act(() => h.foreign());
  fireEvent.blur(field);
  await waitFor(() => expect(screen.getByRole("alert")).toBeTruthy());
  expect(h.client.change).not.toHaveBeenCalled();
  expect((field as HTMLTextAreaElement).value).toBe("我的说明");
  expect(h.current().title).toBe("协作者的新名称");
});
it("画布选择取消不写入并恢复卡片入口", async () => {
  const h = setup();
  const trigger = screen.getByRole("button", { name: "从画布选择" });
  fireEvent.click(trigger);
  await waitFor(() => expect(h.select).toHaveBeenCalled());
  await waitFor(() => expect(document.activeElement).toBe(trigger));
  expect(h.client.change).not.toHaveBeenCalled();
  expect(screen.queryByRole("dialog")).toBeNull();
});

it("画布主媒体选中后填充原节点并只提交一次", async () => {
  const source = createCanvasNode("image", { x: 500, y: 0 });
  const assetID = "22222222-2222-4222-8222-222222222222";
  source.serviceData = {
    id: source.id,
    node_type: "image",
    node_action: "resource",
    media_ref: { asset_id: assetID },
  };
  const select = vi.fn(async () => source);
  const h = setup(select);
  act(() => h.addSource(source));
  api.preview.mockResolvedValue({
    asset: { id: assetID, project_id: h.pid, revision: 1, kind: "image" },
  });
  fireEvent.click(screen.getByRole("button", { name: "从画布选择" }));
  await waitFor(() => expect(h.client.change).toHaveBeenCalledTimes(1));
  expect(h.current().id).toBe(h.node.id);
  expect(subjectConfig(h.current()).local?.primary?.asset_id).toBe(assetID);
  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
});
it("epoch变化时不提交卡片输入", async () => {
  const h = setup();
  fireEvent.click(screen.getByRole("button", { name: "编辑主体名称" }));
  const input = screen.getByRole("textbox", { name: "主体名称" });
  fireEvent.change(input, { target: { value: "新名称" } });
  h.epoch();
  fireEvent.keyDown(input, { key: "Enter" });
  await waitFor(() => expect(screen.getByRole("alert")).toBeTruthy());
  expect(h.client.change).not.toHaveBeenCalled();
  expect((input as HTMLInputElement).value).toBe("新名称");
});
