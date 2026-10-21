import { render, findFeedback } from "../../../feedback";
import {
  act,
  cleanup,
  fireEvent,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { CanvasPage } from "@/app/canvas/open";
import { PromptEditor } from "@/components/generation/prompt-editor";
import type { ComposerSession } from "@/components/generation/composer-session";
import { applyOperations, type Effect } from "@/app/canvas/operations";
import type { Canvas } from "@/app/canvas/document";
import type { Envelope } from "@/app/canvas/collaboration";
const api = vi.hoisted(() => ({
  project: vi.fn(),
  canvas: vi.fn(),
  identity: vi.fn(),
  save: vi.fn(),
  send: vi.fn(),
  live: vi.fn(),
  branches: vi.fn(),
  write: vi.fn(),
  remove: vi.fn(),
  flushResolved: vi.fn(),
  flushRejected: vi.fn(),
  sessions: new Map<string, ComposerSession>(),
}));
// Use the real PM editor/transaction path; only provider/model UI is omitted.
vi.mock("@/components/generation/rich-generation", () => ({
  RichGeneration: function Controlled(
    props: import("react").ComponentProps<
      typeof import("@/components/generation/rich-generation").RichGeneration
    >,
  ) {
    const owner = props.canvas;
    if (!owner)
      return <div data-testid="text-generation-composer">文本生成</div>;
    return (
      <div data-testid="controlled-shared-composer">
        {props.headerActions}
        <PromptEditor
          contextKey={owner.contextKey}
          replicaId={owner.contextKey}
          draft={owner.draft}
          collaboration={owner.collaboration}
          onReady={(session) => {
            api.sessions.set(owner.contextKey, session);
            owner.onSession?.(session);
          }}
          onDispose={() => api.sessions.delete(owner.contextKey)}
          onChange={(snapshot, tr) => {
            if (!tr?.getMeta("composer-authoritative"))
              owner.onChange(snapshot.draft, tr);
          }}
        />
        <button
          onClick={() =>
            void owner.flush().then(api.flushResolved, api.flushRejected)
          }
        >
          测试确认共享输入
        </button>
      </div>
    );
  },
}));
vi.mock("@/api/identity", () => ({ getSession: api.identity }));
vi.mock("@/api/projects", () => ({ getProject: api.project }));
vi.mock("@/api/canvases", () => ({
  getCanvas: api.canvas,
  applyCanvasCommands: api.save,
}));
vi.mock("@/components/media/project-media", () => ({
  ProjectMediaPreview: () => null,
  ProjectMediaPicker: () => null,
}));
vi.mock("@/components/generation/draft-save", async (original) => ({
  ...(await original<typeof import("@/components/generation/draft-save")>()),
  branchJournal: { list: api.branches, write: api.write, remove: api.remove },
}));
type Callbacks = ConstructorParameters<
  typeof import("@/app/canvas/live").CanvasLive
>[1];
vi.mock("@/app/canvas/live", () => ({
  CanvasLive: class {
    constructor(
      _id: string,
      private callbacks: Callbacks,
    ) {
      api.live(callbacks);
    }
    start() {
      this.callbacks.collaboration?.actor("actor");
      this.callbacks.status("live", true);
    }
    stop() {}
    sendCollaboration(envelope: Envelope, resolve = false) {
      api.send(envelope, resolve);
    }
  },
}));
const projectID = "00000000-0000-4000-8000-000000000001";
const canvasID = "00000000-0000-4000-8000-000000000002";
const project = {
  id: projectID,
  canvas_id: canvasID,
  name: "真实画布项目",
  is_delete: false,
  last_modified_at: "2026-10-09T00:00:00Z",
  last_modified_basis: "created",
  style_key: null,
  status: "active",
  aspect_ratio: "1:1",
  revision: 1,
  update_time: "2026-10-09T00:00:00Z",
};
const empty = (): Canvas => ({
  id: canvasID,
  project_id: projectID,
  revision: 1,
  nodes: [],
  edges: [],
});
let base: Canvas;
const cb = () => api.live.mock.lastCall![0] as Callbacks;
function effects(before: Canvas, after: Canvas): Effect[] {
  const result: Effect[] = [];
  for (const entity of ["node", "edge"] as const) {
    const left = entity === "node" ? before.nodes : before.edges;
    const right = entity === "node" ? after.nodes : after.edges;
    for (const id of new Set([...left, ...right].map((n) => n.id!))) {
      const value = right.find((n) => n.id === id);
      result.push({
        entity,
        id,
        path: [],
        index: Math.max(
          0,
          right.findIndex((n) => n.id === id),
        ),
        after_exists: !!value,
        ...(value ? { after: value } : {}),
      });
    }
  }
  return result;
}
function confirm(
  envelope: Envelope,
  operations = envelope.body.operations,
  actor = "actor",
) {
  const next = applyOperations(base, operations);
  for (const n of next.nodes)
    if (n.node_type.endsWith("_generation"))
      n.input_revision =
        (base.nodes.find((b) => b.id === n.id)?.input_revision ?? 0) + 1;
  next.revision = base.revision + 1;
  const change = {
    revision: next.revision,
    actor_id: actor,
    op_id: envelope.key,
    operations,
    effects: effects(base, next),
  };
  base = next;
  cb().collaboration!.changes([change]);
  cb().collaboration!.receipt({
    status: "accepted",
    op_id: envelope.key,
    revision: next.revision,
  });
}
async function open(doc = empty()) {
  base = structuredClone(doc);
  api.project.mockResolvedValue(project);
  api.canvas.mockResolvedValue(doc);
  render(<CanvasPage projectID={projectID} canvasID={canvasID} />);
  await screen.findByRole("heading", { name: project.name });
  await waitFor(() => expect(api.live).toHaveBeenCalled());
}
async function draft() {
  await open();
  fireEvent.click(screen.getByRole("button", { name: "新增图片节点" }));
  await waitFor(() => expect(api.sessions.size).toBe(1));
  return [...api.sessions.values()][0];
}
function input(session: ComposerSession, text: string) {
  act(() =>
    session.editor.view.dispatch(session.editor.state.tr.insertText(text)),
  );
}
function save() {
  // Completed transactions already auto-submit. Explicit flush is available only
  // when the page still has a stable dirty draft, not while its ACK is in flight.
  const button = screen.queryByRole("button", { name: "保存画布" });
  if (button && !button.hasAttribute("disabled")) fireEvent.click(button);
}
beforeEach(() => {
  vi.clearAllMocks();
  api.sessions.clear();
  api.identity.mockResolvedValue({ actor: { id: "actor" } });
  api.branches.mockResolvedValue([]);
});
afterEach(() => {
  cleanup();
  vi.useRealTimers();
});
it("缺少上下文不请求固定项目，拒绝项目与画布串线", async () => {
  const view = render(<CanvasPage />);
  expect(
    screen.getByRole("heading", { name: "请选择有效的项目与画布" }),
  ).toBeTruthy();
  expect(api.project).not.toHaveBeenCalled();
  view.unmount();
  api.project.mockResolvedValue(project);
  api.canvas.mockResolvedValue({ ...empty(), project_id: canvasID });
  render(<CanvasPage projectID={projectID} canvasID={canvasID} />);
  await findFeedback();
  expect(api.live).not.toHaveBeenCalled();
});
it("实际编辑步骤经协议2发送，数据库回执前flush不能生成", async () => {
  const session = await draft();
  const creation = api.send.mock.calls[0][0] as Envelope;
  act(() => confirm(creation));
  api.send.mockClear();
  input(session, "中文😀");
  fireEvent.click(screen.getByRole("button", { name: "测试确认共享输入" }));
  await waitFor(() => expect(api.send).toHaveBeenCalledOnce());
  const envelope = api.send.mock.calls[0][0] as Envelope;
  expect(envelope.body).toMatchObject({
    operation_version: 1,
    base_revision: 2,
  });
  expect(
    envelope.body.operations.some(
      (o) =>
        o.type === "text" &&
        o.text?.some((c) =>
          c.insert?.some((a) => a.type === "text" && a.text === "中文😀"),
        ),
    ),
  ).toBe(true);
  expect(api.flushResolved).not.toHaveBeenCalled();
  expect(api.save).not.toHaveBeenCalled();
  act(() => confirm(envelope));
  await waitFor(() => expect(api.flushResolved).toHaveBeenCalledOnce());
  expect(api.flushResolved.mock.calls[0][0]).toMatchObject({
    input_revision: 2,
  });
});
it("P冻结且ACK不提前确认B，后续Q使用确认后的新基线", async () => {
  const session = await draft();
  input(session, "A");
  save();
  await waitFor(() => expect(api.send).toHaveBeenCalledOnce());
  const p = api.send.mock.calls[0][0] as Envelope,
    frozen = JSON.stringify(p);
  input(session, "B");
  act(() =>
    cb().collaboration!.receipt({
      status: "accepted",
      op_id: p.key,
      revision: 2,
    }),
  );
  expect(api.send).toHaveBeenCalledOnce();
  expect(JSON.stringify(p)).toBe(frozen);
  act(() => confirm(p));
  await waitFor(() => expect(api.send).toHaveBeenCalledTimes(2));
  const q = api.send.mock.calls[1][0] as Envelope;
  expect(q.key).not.toBe(p.key);
  expect(q.body.base_revision).toBe(2);
  expect(session.editor.getText()).toBe("AB");
  act(() => confirm(q));
  await waitFor(() =>
    expect(screen.getByRole("status").textContent).toContain("已保存"),
  );
});
it("断线保留原P和后续输入，重连查证仍引用原key和原body", async () => {
  const session = await draft();
  input(session, "原请求");
  const p = api.send.mock.calls[0][0] as Envelope;
  const frozen = JSON.stringify(p);
  act(() => cb().status("reconnecting"));
  input(session, "后续输入");
  expect(session.editor.getText()).toBe("原请求后续输入");
  expect(JSON.stringify(cb().collaboration!.pending())).toContain(p.key);
  expect(JSON.stringify(api.send.mock.calls[0][0])).toBe(frozen);
  expect(api.send).toHaveBeenCalledOnce();
  expect(api.save).not.toHaveBeenCalled();
});
it.each(["field_conflict", "history_unavailable", "canvas_graph_read_only"])(
  "持久化拒绝%s保留内容，不自动重发原操作",
  async (code) => {
    const session = await draft();
    input(session, "保留中文");
    save();
    await waitFor(() => expect(api.send).toHaveBeenCalledOnce());
    act(() =>
      cb().collaboration!.receipt({
        status: "rejected",
        op_id: api.send.mock.calls[0][0].key,
        code,
      }),
    );
    expect(session.editor.getText()).toBe("保留中文");
    expect(
      await screen.findByRole("button", { name: "复制保留内容" }),
    ).toBeTruthy();
    await new Promise((resolve) => setTimeout(resolve, 20));
    expect(api.send).toHaveBeenCalledOnce();
    expect(api.save).not.toHaveBeenCalled();
  },
);
it("未保存和原请求待确认均保留离开与刷新保护", async () => {
  await draft();
  fireEvent.click(screen.getByRole("link", { name: "织光首页" }));
  expect(
    screen.getByRole("alertdialog", { name: "确认离开画布" }),
  ).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "留在画布" }));
  save();
  await waitFor(() => expect(api.send).toHaveBeenCalledOnce());
  act(() => cb().status("reconnecting", false));
  const event = new Event("beforeunload", { cancelable: true });
  window.dispatchEvent(event);
  expect(event.defaultPrevented).toBe(true);
});
it("远端连续变更更新节点，后续操作基于新revision", async () => {
  await open();
  const node = {
    id: crypto.randomUUID(),
    title: "远端文本",
    node_type: "text",
    node_action: "resource",
    x: 80,
    y: 80,
    z_index: 0,
    config: { text: "AB" },
  } as Canvas["nodes"][number];
  act(() =>
    confirm(
      {
        key: crypto.randomUUID(),
        body: {
          operation_version: 1,
          base_revision: 1,
          operations: [{ type: "create_node", node }],
        },
      },
      undefined,
      "other",
    ),
  );
  const move = await screen.findByRole("button", { name: "移动 远端文本" });
  fireEvent.keyDown(move, { key: "ArrowRight", shiftKey: true });
  save();
  await waitFor(() => expect(api.send).toHaveBeenCalledOnce());
  expect(api.send.mock.calls[0][0].body).toMatchObject({
    base_revision: 2,
    operations: [
      {
        type: "set",
        node_id: node.id,
        path: ["position"],
        value: { x: 90, y: 80 },
      },
    ],
  });
});
it("贡献撤销发送undo_of，不在本地回滚整图", async () => {
  await draft();
  save();
  await waitFor(() => expect(api.send).toHaveBeenCalledOnce());
  const p = api.send.mock.calls[0][0] as Envelope;
  act(() => confirm(p));
  fireEvent.click(screen.getByRole("button", { name: "撤销编辑" }));
  await waitFor(() => expect(api.send).toHaveBeenCalledTimes(2));
  expect(api.send.mock.calls[1][0].body).toMatchObject({
    undo_of: p.key,
    operations: [],
    base_revision: 2,
  });
  expect(screen.getByRole("button", { name: "移动 图片草稿" })).toBeTruthy();
  act(() =>
    confirm(api.send.mock.calls[1][0], [
      { type: "delete_node", node_id: base.nodes[0].id },
    ]),
  );
  await waitFor(() =>
    expect(screen.queryByRole("button", { name: "移动 图片草稿" })).toBeNull(),
  );
});
it("后端只读状态不提交新操作，身份撤销清除会话内容", async () => {
  await open();
  act(() => cb().status("live", false));
  fireEvent.click(screen.getByRole("button", { name: "新增图片节点" }));
  expect(api.sessions.size).toBe(0);
  expect(api.send).not.toHaveBeenCalled();
  act(() => cb().revoked());
  await screen.findByText("请重新打开画布");
  expect(screen.queryByText("双击画布，创建节点")).toBeNull();
});
it("画布业务不读写本地恢复库，重开只读取服务端确认数据", async () => {
  await draft();
  save();
  await waitFor(() => expect(api.send).toHaveBeenCalledOnce());
  act(() => confirm(api.send.mock.calls[0][0]));
  cleanup();
  api.canvas.mockResolvedValue(base);
  render(<CanvasPage projectID={projectID} canvasID={canvasID} />);
  await screen.findByRole("button", { name: "移动 图片草稿" });
  expect(api.branches).not.toHaveBeenCalled();
  expect(api.write).not.toHaveBeenCalled();
  expect(api.remove).not.toHaveBeenCalled();
  expect(api.save).not.toHaveBeenCalled();
  expect(api.canvas).toHaveBeenCalledTimes(2);
});

const canvas = empty();
const openDraft = draft;

it("切换项目取消旧请求，迟到的旧项目不能覆盖新项目", async () => {
  let finish!: (value: unknown) => void;
  const nextID = "00000000-0000-4000-8000-000000000003";
  api.project.mockImplementation(({ pid }: { pid: string }) =>
    pid === projectID
      ? new Promise((resolve) => {
          finish = resolve;
        })
      : Promise.resolve({ ...project, id: nextID, name: "新项目" }),
  );
  api.canvas
    .mockResolvedValueOnce(canvas)
    .mockResolvedValueOnce({ ...canvas, project_id: nextID });
  const view = render(<CanvasPage projectID={projectID} canvasID={canvasID} />);
  const oldSignal = api.project.mock.calls[0][1].signal as AbortSignal;
  view.rerender(<CanvasPage projectID={nextID} canvasID={canvasID} />);
  await screen.findByRole("heading", { name: "新项目" });
  finish(project);
  await waitFor(() => expect(oldSignal.aborted).toBe(true));
  expect(screen.queryByRole("heading", { name: "真实画布项目" })).toBeNull();
  expect(screen.getByText("双击画布，创建节点")).toBeTruthy();
});

it("历史视口仅兼容读取，缩放不触发数据库写入或本地业务缓存", async () => {
  api.project.mockResolvedValue(project);
  api.canvas.mockResolvedValue({
    ...canvas,
    viewport: { x: 120, y: -80, zoom: 0.5 },
  });
  render(<CanvasPage projectID={projectID} canvasID={canvasID} />);
  await screen.findByText("双击画布，创建节点");
  expect(screen.getByText("50%")).toBeTruthy();
  fireEvent.keyDown(screen.getByLabelText("画布工作区"), {
    key: "+",
    ctrlKey: true,
  });
  await act(async () => {
    await new Promise((r) => setTimeout(r, 1000));
  });
  expect(
    (screen.getByRole("button", { name: "保存画布" }) as HTMLButtonElement)
      .disabled,
  ).toBe(true);
  expect(api.save).not.toHaveBeenCalled();
  expect(api.write).not.toHaveBeenCalled();
});

it("含只读直接成员的分组不能解组，不污染其他合法编辑", async () => {
  const group = {
    id: "00000000-0000-4000-8000-000000000030",
    title: "受限分组",
    node_type: "group",
    node_action: "resource",
    config: { collapsed: false },
    x: 80,
    y: 120,
    width: 500,
    height: 400,
    z_index: 0,
  };
  const child = {
    id: "00000000-0000-4000-8000-000000000031",
    title: "业务投影",
    node_type: "text",
    node_action: "resource",
    config: { text: "受保护内容" },
    last_operation_id: "00000000-0000-4000-8000-000000000032",
    parent_id: group.id,
    x: 100,
    y: 140,
    z_index: 0,
  };
  api.project.mockResolvedValue(project);
  api.canvas.mockResolvedValue({ ...canvas, nodes: [group, child] });
  render(<CanvasPage projectID={projectID} canvasID={canvasID} />);
  fireEvent.click(await screen.findByRole("button", { name: "选择 受限分组" }));
  fireEvent.click(screen.getByRole("button", { name: "解除所选分组" }));
  await findFeedback("分组包含只读成员，不能解除关系。");
  expect(
    screen.getByRole("button", { name: "保存画布" }).hasAttribute("disabled"),
  ).toBe(true);
});

it("分组和标注线沿服务端合同保存，标注不进入生成流程", async () => {
  const a = {
    id: "00000000-0000-4000-8000-000000000003",
    title: "素材A",
    node_type: "image",
    node_action: "resource",
    ref_type: "media_asset",
    ref_id: "00000000-0000-4000-8000-000000000013",
    config: {},
    x: 80,
    y: 80,
    z_index: 0,
  };
  const b = {
    ...a,
    id: "00000000-0000-4000-8000-000000000004",
    ref_id: "00000000-0000-4000-8000-000000000014",
    title: "素材B",
    x: 420,
  };
  api.project.mockResolvedValue(project);
  api.canvas.mockResolvedValue({ ...canvas, nodes: [a, b] });
  render(<CanvasPage projectID={projectID} canvasID={canvasID} />);
  fireEvent.click(await screen.findByRole("button", { name: "选择 素材A" }));
  fireEvent.click(screen.getByRole("button", { name: "选择 素材B" }), {
    shiftKey: true,
  });
  fireEvent.click(screen.getByRole("button", { name: "连接所选节点" }));
  expect(document.querySelectorAll("[data-connection-id]")).toHaveLength(1);
  fireEvent.click(screen.getByRole("button", { name: "组合所选节点" }));
  expect(screen.getByRole("button", { name: "选择 分组" })).toBeTruthy();
  expect(api.save).not.toHaveBeenCalled();
});

it("双击空白画布打开创建菜单，按点击世界坐标创建文本", async () => {
  api.project.mockResolvedValue(project);
  api.canvas.mockResolvedValue(canvas);
  render(<CanvasPage projectID={projectID} canvasID={canvasID} />);
  await screen.findByText("双击画布，创建节点");
  const board = screen.getByLabelText("画布工作区");
  fireEvent.doubleClick(board, { clientX: 300, clientY: 240 });
  fireEvent.click(await screen.findByRole("menuitem", { name: "文本" }));
  const node = document.querySelector<HTMLElement>("[data-node-id]");
  expect(node?.style.left).toBe("300px");
  expect(node?.style.top).toBe("240px");
});

it("搜索画布可打开、筛选并选择节点", async () => {
  await openDraft();
  HTMLElement.prototype.scrollIntoView = vi.fn();
  fireEvent.click(screen.getByRole("button", { name: "搜索画布" }));
  const search = screen.getByPlaceholderText("搜索节点名称…");
  fireEvent.change(search, { target: { value: "图片" } });
  fireEvent.click(await screen.findByRole("option", { name: "图片草稿" }));
  expect(screen.queryByPlaceholderText("搜索节点名称…")).toBeNull();
  expect(screen.getByLabelText("节点编辑")).toBeTruthy();
});

it("正式入口直接挂载受控画布及共享富文本，工具栏没有时间线", async () => {
  await openDraft();
  expect(
    document.querySelector('[aria-label="画布工作区"][data-canvas-viewport]'),
  ).not.toBeNull();
  expect(screen.getByTestId("controlled-shared-composer")).toBeTruthy();
  expect(screen.queryByRole("button", { name: /时间线/ })).toBeNull();
});

it("服务端只读原因禁止整图编辑且保留未知草稿原样展示", async () => {
  const { createDraftNode } = await import("@/app/canvas/document");
  const node = {
    ...createDraftNode("image_generation", "1:1", 0),
    config: { future: { version: 99 } },
  };
  api.project.mockResolvedValue(project);
  api.canvas.mockResolvedValue({
    ...canvas,
    nodes: [node],
    read_only_reason: "unclassified node config",
  });
  render(<CanvasPage projectID={projectID} canvasID={canvasID} />);
  await screen.findByText("画布数据待兼容 · 只读");
  expect(
    (screen.getByRole("button", { name: "保存画布" }) as HTMLButtonElement)
      .disabled,
  ).toBe(true);
  expect(
    screen.getByRole("button", { name: "添加文本" }).closest("[inert]"),
  ).toBeTruthy();
  fireEvent.keyDown(window, { key: "s", ctrlKey: true });
  expect(api.save).not.toHaveBeenCalled();
  expect(api.write).not.toHaveBeenCalled();
});

it("已归档画布节点不显示重命名及尺寸编辑控件", async () => {
  const { createResourceNode } = await import("@/app/canvas/document");
  const node = createResourceNode("text", { x: 80, y: 80 });
  api.project.mockResolvedValue({ ...project, status: "archived" });
  api.canvas.mockResolvedValue({ ...canvas, nodes: [node] });
  render(<CanvasPage projectID={projectID} canvasID={canvasID} />);
  await screen.findByText("已归档 · 只读");
  fireEvent.click(screen.getByRole("button", { name: "查看 文本" }));
  expect(screen.queryByRole("button", { name: "节点操作：文本" })).toBeNull();
  expect(screen.queryByRole("button", { name: /调整 文本/ })).toBeNull();
});

it("旧本地分支不再读取、恢复或参与画布保存", async () => {
  api.branches.mockResolvedValue([
    { ownerState: { nodes: [{ title: "历史本地数据" }] } },
  ]);
  await openDraft();
  expect(api.branches).not.toHaveBeenCalled();
  expect(api.write).not.toHaveBeenCalled();
  expect(api.remove).not.toHaveBeenCalled();
  expect(screen.queryByRole("button", { name: "恢复此分支" })).toBeNull();
});

it("文本单选展示生成提示词，双击正文隐藏下方面板，上传素材不展示面板", async () => {
  api.project.mockResolvedValue(project);
  api.canvas.mockResolvedValue({
    ...canvas,
    nodes: [
      {
        id: "00000000-0000-4000-8000-000000000013",
        title: "上传图片",
        node_type: "image",
        node_action: "resource",
        ref_type: "media_asset",
        ref_id: "00000000-0000-4000-8000-000000000014",
        config: {},
        x: 800,
        y: 80,
        z_index: 0,
      },
    ],
  });
  render(<CanvasPage projectID={projectID} canvasID={canvasID} />);
  fireEvent.click(await screen.findByRole("button", { name: "选择 上传图片" }));
  expect(document.querySelector(".canvas-node-panel")).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "添加文本" }));
  await screen.findByTestId("text-generation-composer");
  fireEvent.doubleClick(screen.getByRole("button", { name: "选择 文本" }));
  expect(screen.queryByTestId("text-generation-composer")).toBeNull();
  expect(document.querySelector(".canvas-node-panel")).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "选择 上传图片" }));
  fireEvent.click(screen.getByRole("button", { name: "选择 文本" }));
  await screen.findByTestId("text-generation-composer");
});

it("断线仅显示连接状态，不撤销已确认的编辑权限", async () => {
  await openDraft();
  act(() => api.live.mock.lastCall![0].status("reconnecting"));
  expect(screen.getByRole("status").textContent).toContain("连接中断");
  expect(
    screen
      .getByRole("toolbar", { name: "画布工具" })
      .getAttribute("aria-disabled"),
  ).toBe("false");
  act(() => api.live.mock.lastCall![0].status("live", true));
  expect(
    screen
      .getByRole("toolbar", { name: "画布工具" })
      .getAttribute("aria-disabled"),
  ).toBe("false");
});

it("未知配置只读基线收到新快照保持opaque，不解析或触发重连", async () => {
  api.project.mockResolvedValue(project);
  const { createResourceNode } = await import("@/app/canvas/document");
  const node = {
    ...createResourceNode("text", { x: 0, y: 0 }),
    title: "旧图不透明节点",
    node_type: "future_kind",
    config: { timeline: { clips: { future: 1 } } },
  };
  const protectedCanvas = {
    ...canvas,
    nodes: [node],
    read_only_reason: "unsupported",
  };
  api.canvas.mockResolvedValue(protectedCanvas);
  render(<CanvasPage projectID={projectID} canvasID={canvasID} />);
  await screen.findByRole("heading", { name: "旧图不透明节点" });
  expect(() =>
    act(() =>
      api.live.mock.lastCall![0].snapshot(
        {
          ...protectedCanvas,
          revision: 2,
          nodes: [{ ...node, title: "新版不透明节点" }],
        },
        true,
      ),
    ),
  ).not.toThrow();
  await screen.findByRole("heading", { name: "新版不透明节点" });
  expect(api.save).not.toHaveBeenCalled();
});
it("冲突对照显示远端文本、composer及节点删除和连线变化", async () => {
  await openDraft();
  const { createResourceNode, createDraftNode, richCanvasDraft } =
    await import("@/app/canvas/document");
  const { emptyPromptDraft } =
    await import("@/components/generation/prompt-document");
  const text = {
    ...createResourceNode("text", { x: 0, y: 0 }),
    title: "远端文本节点",
    config: { text: "远端修改的正文" },
  };
  const draft = emptyPromptDraft();
  draft.document.content = [
    {
      type: "paragraph",
      content: [{ type: "text", text: "远端富文本提示词" }],
    },
  ];
  const generation = {
    ...createDraftNode("image_generation", "1:1", 0),
    title: "远端生成节点",
    input_revision: 1,
    config: { draft: richCanvasDraft(draft) },
  };
  const operations: import("@/app/canvas/operations").Operation[] = [
    { type: "create_node", node: text },
    { type: "create_node", node: generation },
  ];
  act(() =>
    confirm(
      {
        key: crypto.randomUUID(),
        body: {
          operation_version: 1,
          base_revision: base.revision,
          operations,
        },
      },
      operations,
      "other",
    ),
  );
  save();
  await waitFor(() => expect(api.send).toHaveBeenCalled());
  act(() =>
    cb().collaboration!.receipt({
      status: "rejected",
      op_id: api.send.mock.calls[0][0].key,
      code: "field_conflict",
    }),
  );
  const comparison = screen.getByLabelText("服务端内容");
  expect(comparison.textContent).toContain("远端修改的正文");
  expect(comparison.textContent).toContain("远端富文本提示词");
  expect(comparison.textContent).toContain("连线");
  expect(comparison.textContent).toContain("删除");
});

function nodePointer(
  target: Element | Window,
  type: string,
  x: number,
  y: number,
  pointerId = 1,
) {
  const event = new MouseEvent(type, {
    bubbles: true,
    cancelable: true,
    button: 0,
    clientX: x,
    clientY: y,
  });
  Object.defineProperty(event, "pointerId", { value: pointerId });
  fireEvent(target, event);
  return event;
}
async function openDraggableNode() {
  const { createResourceNode } = await import("@/app/canvas/document");
  const node = {
    ...createResourceNode("text", { x: 200, y: 150 }),
    title: "拖动回归",
  };
  await open({ ...canvas, nodes: [node], viewport: { x: 0, y: 0, zoom: 0.5 } });
  const handle = await screen.findByRole("button", { name: "移动 拖动回归" });
  handle.hasPointerCapture = () => false;
  document.querySelector<HTMLElement>(
    "[data-canvas-viewport]",
  )!.hasPointerCapture = () => false;
  return { node, handle, card: handle.parentElement! };
}
function remotePosition(nodeID: string, x: number, y: number) {
  const operations: import("@/app/canvas/operations").Operation[] = [
    { type: "set", node_id: nodeID, path: ["position"], value: { x, y } },
  ];
  confirm(
    {
      key: crypto.randomUUID(),
      body: { operation_version: 1, base_revision: base.revision, operations },
    },
    operations,
    "other",
  );
}
it("拖动阻止原生拖拽，忽略其他指针并通过服务端贡献撤销落点", async () => {
  const { node, handle, card } = await openDraggableNode();
  expect(nodePointer(handle, "pointerdown", 100, 75).defaultPrevented).toBe(
    true,
  );
  nodePointer(handle, "pointermove", 600, 400, 2);
  expect(card.style.left).toBe("200px");
  nodePointer(handle, "pointermove", 130, 95);
  nodePointer(handle, "pointerup", 150, 115);
  expect([card.style.left, card.style.top]).toEqual(["300px", "230px"]);
  save();
  await waitFor(() => expect(api.send).toHaveBeenCalled());
  const move = api.send.mock.calls[0][0] as Envelope;
  act(() => confirm(move));
  fireEvent.click(screen.getByRole("button", { name: "撤销编辑" }));
  await waitFor(() =>
    expect(
      api.send.mock.calls.some((c) => c[0].body.undo_of === move.key),
    ).toBe(true),
  );
  const undo = api.send.mock.calls.find(
    (c) => c[0].body.undo_of === move.key,
  )![0] as Envelope;
  act(() =>
    confirm(undo, [
      {
        type: "set",
        node_id: node.id,
        path: ["position"],
        value: { x: 200, y: 150 },
      },
    ]),
  );
  expect([card.style.left, card.style.top]).toEqual(["200px", "150px"]);
  fireEvent.click(screen.getByRole("button", { name: "重做编辑" }));
  await waitFor(() =>
    expect(
      api.send.mock.calls.some((c) => c[0].body.undo_of === undo.key),
    ).toBe(true),
  );
  const redo = api.send.mock.calls.find(
    (c) => c[0].body.undo_of === undo.key,
  )![0] as Envelope;
  act(() =>
    confirm(redo, [
      {
        type: "set",
        node_id: node.id,
        path: ["position"],
        value: { x: 300, y: 230 },
      },
    ]),
  );
  expect([card.style.left, card.style.top]).toEqual(["300px", "230px"]);
  expect(api.save).not.toHaveBeenCalled();
});
it("拖动中的远端位置参与OT，松手后只提交最终落点", async () => {
  const { node, handle, card } = await openDraggableNode();
  nodePointer(handle, "pointerdown", 100, 75);
  nodePointer(handle, "pointermove", 130, 95);
  act(() => remotePosition(node.id, 800, 150));
  nodePointer(handle, "pointerup", 150, 115);
  expect([card.style.left, card.style.top]).toEqual(["300px", "230px"]);
  save();
  await waitFor(() => expect(api.send).toHaveBeenCalled());
  expect(api.send.mock.calls[0][0].body).toMatchObject({
    base_revision: 2,
    operations: [
      { type: "set", path: ["position"], value: { x: 300, y: 230 } },
    ],
  });
  expect(screen.queryByRole("region", { name: "版本冲突处理" })).toBeNull();
  expect(api.save).not.toHaveBeenCalled();
});
it("取消拖动保留已同步的远端位置，不产生本地移动", async () => {
  const { node, handle, card } = await openDraggableNode();
  nodePointer(handle, "pointerdown", 100, 75);
  nodePointer(handle, "pointermove", 130, 95);
  act(() => remotePosition(node.id, 800, 150));
  nodePointer(handle, "pointercancel", 130, 95);
  expect(card.style.left).toBe("800px");
  expect(api.send).not.toHaveBeenCalled();
});
it("连续拖动跨越旧操作回执，后一落点仍提交到新版本", async () => {
  const { handle, card } = await openDraggableNode();
  nodePointer(handle, "pointerdown", 100, 75);
  nodePointer(handle, "pointerup", 150, 115);
  save();
  await waitFor(() => expect(api.send).toHaveBeenCalledOnce());
  const first = api.send.mock.calls[0][0] as Envelope;
  nodePointer(handle, "pointerdown", 150, 115);
  nodePointer(handle, "pointermove", 170, 125);
  act(() => confirm(first));
  nodePointer(handle, "pointerup", 200, 145);
  expect([card.style.left, card.style.top]).toEqual(["400px", "290px"]);
  save();
  await waitFor(() =>
    expect(api.send.mock.calls.filter((c) => !c[1])).toHaveLength(2),
  );
  const second = api.send.mock.calls.filter((c) => !c[1])[1][0] as Envelope;
  expect(second.body).toMatchObject({
    base_revision: 2,
    operations: [
      { type: "set", path: ["position"], value: { x: 400, y: 290 } },
    ],
  });
  act(() => confirm(second));
  await waitFor(() =>
    expect(screen.getByRole("status").textContent).toContain("已保存"),
  );
  expect([card.style.left, card.style.top]).toEqual(["400px", "290px"]);
});
it("拖动中请求保存等待手势完成，只提交最终位置", async () => {
  const { handle } = await openDraggableNode();
  nodePointer(handle, "pointerdown", 100, 75);
  nodePointer(handle, "pointermove", 130, 95);
  fireEvent.keyDown(window, { key: "s", ctrlKey: true });
  expect(api.send).not.toHaveBeenCalled();
  nodePointer(handle, "pointerup", 150, 115);
  await waitFor(() => expect(api.send).toHaveBeenCalledOnce());
  expect(api.send.mock.calls[0][0].body.operations).toMatchObject([
    { type: "set", path: ["position"], value: { x: 300, y: 230 } },
  ]);
});
it("按下后未移动不把单击当成拖动", async () => {
  const { handle } = await openDraggableNode();
  nodePointer(handle, "pointerdown", 100, 75);
  fireEvent.keyDown(window, { key: "s", ctrlKey: true });
  nodePointer(handle, "pointerup", 101, 76);
  expect(handle.parentElement!.style.left).toBe("200px");
  expect(api.send).not.toHaveBeenCalled();
  expect(api.save).not.toHaveBeenCalled();
});

it("指针静止时协同位置到达也不能覆盖正在显示的拖动落点", async () => {
  const { node, handle, card } = await openDraggableNode();
  nodePointer(handle, "pointerdown", 100, 75);
  nodePointer(handle, "pointermove", 130, 95);
  await act(
    () =>
      new Promise<void>((resolve) => requestAnimationFrame(() => resolve())),
  );
  const shownX = () =>
    Number.parseFloat(
      card.style.getPropertyValue("--canvas-drag-x") || card.style.left,
    );
  expect(shownX()).toBe(260);
  act(() => remotePosition(node.id, 800, 150));
  expect(shownX()).toBe(260);
  nodePointer(handle, "pointerup", 130, 95);
  expect(card.style.left).toBe("260px");
  expect(card.style.getPropertyValue("--canvas-drag-x")).toBe("");
});

it.each(["read_only", "reconnecting"] as const)(
  "拖动开始后%s，松手清理预览且恢复后可以继续提交",
  async (reason) => {
    const { node, handle, card } = await openDraggableNode();
    nodePointer(handle, "pointerdown", 100, 75);
    nodePointer(handle, "pointermove", 130, 95);
    await act(
      () =>
        new Promise<void>((resolve) => requestAnimationFrame(() => resolve())),
    );
    act(() =>
      cb().status(reason === "reconnecting" ? "reconnecting" : "live", false),
    );
    nodePointer(handle, "pointerup", 150, 115);
    expect([card.style.left, card.style.top]).toEqual(["200px", "150px"]);
    expect(card.style.getPropertyValue("--canvas-drag-x")).toBe("");
    expect(card.style.getPropertyValue("--canvas-drag-y")).toBe("");
    expect(api.send).not.toHaveBeenCalled();
    act(() => cb().status("live", true));
    expect([card.style.left, card.style.top]).toEqual(["200px", "150px"]);
    nodePointer(handle, "pointerdown", 100, 75);
    nodePointer(handle, "pointerup", 125, 95);
    save();
    await waitFor(() => expect(api.send).toHaveBeenCalledOnce());
    expect(api.send.mock.calls[0][0].body.operations).toMatchObject([
      {
        type: "set",
        node_id: node.id,
        path: ["position"],
        value: { x: 250, y: 190 },
      },
    ]);
  },
);

it("连接状态在一次渲染前恢复，可写界面仍允许后续拖动", async () => {
  const { node, handle, card } = await openDraggableNode();
  act(() => {
    cb().status("reconnecting", false);
    cb().status("live", true);
  });
  expect(
    screen
      .getByRole("toolbar", { name: "画布工具" })
      .getAttribute("aria-disabled"),
  ).toBe("false");
  nodePointer(handle, "pointerdown", 100, 75);
  nodePointer(handle, "pointerup", 125, 95);
  expect([card.style.left, card.style.top]).toEqual(["250px", "190px"]);
  save();
  await waitFor(() => expect(api.send).toHaveBeenCalledOnce());
  expect(api.send.mock.calls[0][0].body.operations).toMatchObject([
    {
      type: "set",
      node_id: node.id,
      path: ["position"],
      value: { x: 250, y: 190 },
    },
  ]);
});

it("短暂断线仍能完成并继续拖动，重连仅经WebSocket保存最新落点", async () => {
  const { handle, card } = await openDraggableNode();
  nodePointer(handle, "pointerdown", 100, 75);
  nodePointer(handle, "pointermove", 130, 95);
  act(() => cb().status("reconnecting"));
  expect(screen.getByRole("button", { name: "移动 拖动回归" })).toBeTruthy();
  nodePointer(handle, "pointerup", 150, 115);
  expect([card.style.left, card.style.top]).toEqual(["300px", "230px"]);
  nodePointer(handle, "pointerdown", 150, 115);
  nodePointer(handle, "pointerup", 175, 135);
  expect([card.style.left, card.style.top]).toEqual(["350px", "270px"]);
  expect(api.send).not.toHaveBeenCalled();
  act(() => cb().status("live", true));
  expect(api.send).toHaveBeenCalledOnce();
  const p = api.send.mock.calls[0][0] as Envelope;
  act(() => confirm(p));
  expect([card.style.left, card.style.top]).toEqual(["350px", "270px"]);
  expect(api.save).not.toHaveBeenCalled();
});

it("松手立即提交WebSocket操作，不等待静默定时器", async () => {
  const { handle } = await openDraggableNode();
  nodePointer(handle, "pointerdown", 100, 75);
  nodePointer(handle, "pointermove", 130, 95);
  expect(api.send).not.toHaveBeenCalled();
  nodePointer(handle, "pointerup", 150, 115);
  expect(api.send).toHaveBeenCalledOnce();
  expect(api.save).not.toHaveBeenCalled();
});

it("松手落在节点外仍结束手势，连续拖动不能被旧手势锁住", async () => {
  const { handle, card } = await openDraggableNode();
  nodePointer(handle, "pointerdown", 100, 75);
  nodePointer(window, "pointermove", 130, 95);
  nodePointer(window, "pointerup", 150, 115);
  expect([card.style.left, card.style.top]).toEqual(["300px", "230px"]);
  nodePointer(handle, "pointerdown", 150, 115);
  nodePointer(window, "pointerup", 175, 135);
  expect([card.style.left, card.style.top]).toEqual(["350px", "270px"]);
});

it("窗口失焦取消手势并释放监听，回来后可以重新拖动", async () => {
  const { handle, card } = await openDraggableNode();
  nodePointer(handle, "pointerdown", 100, 75);
  nodePointer(window, "pointermove", 130, 95);
  fireEvent(window, new Event("blur"));
  nodePointer(window, "pointerup", 150, 115);
  expect(card.style.left).toBe("200px");
  expect(api.send).not.toHaveBeenCalled();
  nodePointer(handle, "pointerdown", 100, 75);
  nodePointer(window, "pointerup", 150, 115);
  expect(card.style.left).toBe("300px");
});
