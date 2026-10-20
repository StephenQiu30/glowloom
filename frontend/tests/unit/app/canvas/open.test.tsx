import { render, findFeedback } from "../../../feedback";
import {
  act,
  cleanup,
  fireEvent,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, beforeAll, beforeEach, expect, it, vi } from "vitest";
import { CanvasPage } from "@/app/canvas/open";
import { RequestError } from "@/lib/request";
import type { Node } from "@/app/canvas/document";
const api = vi.hoisted(() => ({
  project: vi.fn(),
  live: vi.fn(),
  identity: vi.fn(),
  canvas: vi.fn(),
  save: vi.fn(),
  branches: vi.fn(),
  write: vi.fn(),
  remove: vi.fn(),
  rich: vi.fn(),
  preview: vi.fn(),
  flushResolved: vi.fn(),
  flushRejected: vi.fn(),
}));
// Parent coordination suite: drive the real canvas owner contract without booting
// provider/model/reference UI. Those components have their own integration suites.
vi.mock("@/components/generation/rich-generation", () => ({
  RichGeneration: function ControlledRichGeneration(
    props: import("react").ComponentProps<
      typeof import("@/components/generation/rich-generation").RichGeneration
    >,
  ) {
    api.rich(props);
    const owner = props.canvas;
    if (!owner)
      throw new Error("Canvas must provide its shared composer owner");
    const text = owner.draft.document.content
      .map((paragraph) =>
        (paragraph.content ?? [])
          .map((part) =>
            part.type === "text"
              ? part.text
              : part.type === "hardBreak"
                ? "\n"
                : "",
          )
          .join(""),
      )
      .join("\n");
    return (
      <div
        data-testid="controlled-shared-composer"
        data-presentation={props.presentation}
      >
        {props.headerActions}
        <input
          aria-label="测试共享富文本"
          value={text}
          onChange={(event) => {
            const draft = structuredClone(owner.draft);
            draft.document = {
              type: "doc",
              content: event.target.value.split("\n").map((text) => ({
                type: "paragraph",
                ...(text ? { content: [{ type: "text" as const, text }] } : {}),
              })),
            };
            owner.onChange(draft);
          }}
        />
        <button
          type="button"
          onClick={() => {
            void owner.flush().then(api.flushResolved, api.flushRejected);
          }}
        >
          测试确认共享输入
        </button>
      </div>
    );
  },
}));
vi.mock("@/api/identity", () => ({
  getSession: api.identity,
}));
vi.mock("@/components/generation/draft-save", async (original) => ({
  ...(await original<typeof import("@/components/generation/draft-save")>()),
  branchJournal: {
    list: api.branches,
    write: api.write,
    remove: api.remove,
  },
}));
vi.mock("@/app/canvas/live", () => ({
  CanvasLive: class {
    constructor(
      private id: string,
      private callbacks: {
        status: (status: string, writable?: boolean) => void;
      },
    ) {
      api.live(this.callbacks);
    }
    start() {
      this.callbacks.status("live", true);
    }
    stop() {}
    submit(key: string, body: API.CommandsInput) {
      return api.save({ id: this.id }, body, {
        headers: { "Idempotency-Key": key },
        signal: new AbortController().signal,
      });
    }
  },
}));
vi.mock("@/api/canvases", () => ({
  getCanvas: api.canvas,
  applyCanvasCommands: api.save,
}));
vi.mock("@/api/projects", () => ({
  getProject: api.project,
}));
vi.mock("@/api/media", () => ({ getMediaPreview: api.preview }));
vi.mock("@/components/media/project-media", () => ({
  ProjectMediaPreview: () => null,
  ProjectMediaPicker: () => null,
}));

vi.mock("@/app/canvas/node/text-editor", () => ({
  CanvasTextEditor: ({
    text,
    onChange,
  }: {
    text: string;
    onChange: (text: string) => void;
  }) => (
    <input
      aria-label="文本内容"
      value={text}
      onChange={(event) => onChange(event.target.value)}
    />
  ),
}));
const projectID = "00000000-0000-4000-8000-000000000001";
const canvasID = "00000000-0000-4000-8000-000000000002";
const project = {
  id: projectID,
  canvas_id: canvasID,
  name: "真实画布项目",
  status: "active",
  is_delete: false,
  last_modified_at: "2026-10-09T00:00:00Z",
  last_modified_basis: "created",
  aspect_ratio: "1:1",
  style_key: null,
  revision: 1,
  update_time: "2026-10-09T00:00:00Z",
};
const canvas = {
  id: canvasID,
  project_id: projectID,
  revision: 1,
  nodes: [],
  edges: [],
};
// Compile the lazy editor before event/receipt assertions; browser acceptance covers chunk loading.
beforeAll(async () => {
  await import("@/app/canvas/editor");
});
beforeEach(() => {
  vi.clearAllMocks();
  api.identity.mockReset().mockResolvedValue({ actor: { id: "actor" } });
  api.project.mockReset();
  api.canvas.mockReset();
  api.save.mockReset();
  api.branches.mockReset().mockResolvedValue([]);
  api.write.mockReset().mockResolvedValue(undefined);
  api.remove.mockReset().mockResolvedValue(true);
});
afterEach(() => {
  cleanup();
  vi.useRealTimers();
});
it("缺少上下文不请求或展示固定项目", () => {
  render(<CanvasPage />);
  expect(
    screen.getByRole("heading", { name: "请选择有效的项目与画布" }),
  ).toBeTruthy();
  expect(api.project).not.toHaveBeenCalled();
  expect(api.canvas).not.toHaveBeenCalled();
});

async function openDraft() {
  api.project.mockResolvedValue(project);
  api.canvas.mockResolvedValue(canvas);
  render(<CanvasPage projectID={projectID} canvasID={canvasID} />);
  await screen.findByText("双击画布，创建节点");
  fireEvent.click(screen.getByRole("button", { name: "新增图片节点" }));
}
// Go emits nullable model/mode pointers as null, rather than echoing the
// optional request fields. Preserve that wire contract in save receipts.
function receiptNode(value: API.Node, inputRevision = 1): Node {
  const node = structuredClone(value) as Node;
  if (node.config.draft?.composer) {
    node.config.draft.composer.mode ??= null;
    node.config.draft.composer.modelVersion ??= null;
  }
  if (node.node_type.endsWith("_generation"))
    node.input_revision = inputRevision;
  return node;
}
function addReceipt(body: API.CommandsInput) {
  return {
    ...canvas,
    revision: 2,
    nodes: body.commands
      ?.find((command) => command.type === "AddNodes")
      ?.nodes?.map((node) => receiptNode(node)),
    results: body.commands?.map(() => ({ ok: true })),
  };
}
it("保存中的后续编辑不会被旧回执标成已保存", async () => {
  await openDraft();
  fireEvent.change(screen.getByLabelText("测试共享富文本"), {
    target: { value: "冻结提示" },
  });
  let finish!: (value: unknown) => void;
  api.save.mockImplementationOnce(
    () =>
      new Promise((resolve) => {
        finish = resolve;
      }),
  );
  let finishLatest!: (value: unknown) => void;
  api.save.mockImplementationOnce(
    () =>
      new Promise((resolve) => {
        finishLatest = resolve;
      }),
  );
  fireEvent.click(screen.getByRole("button", { name: "保存画布" }));
  await waitFor(() => expect(api.save).toHaveBeenCalledOnce());
  const frozen = structuredClone(
    api.save.mock.calls[0][1],
  ) as API.CommandsInput;
  fireEvent.change(screen.getByLabelText("测试共享富文本"), {
    target: { value: "保存中继续编辑" },
  });
  await act(async () => finish(addReceipt(frozen)));
  expect(
    (screen.getByLabelText("测试共享富文本") as HTMLInputElement).value,
  ).toBe("保存中继续编辑");
  await waitFor(() => expect(api.save).toHaveBeenCalledTimes(2));
  expect(
    screen
      .getAllByRole("status")
      .some((node) => node.textContent?.includes("正在保存")),
  ).toBe(true);
  expect(screen.queryByText("所有更改已保存")).toBeNull();
  const nextBody = api.save.mock.calls[1][1];
  const committed = addReceipt(frozen);
  await act(async () =>
    finishLatest({
      ...committed,
      revision: 3,
      nodes: committed.nodes?.map((n) =>
        receiptNode({ ...n, config: nextBody.commands[0].config }, 2),
      ),
      results: nextBody.commands.map(() => ({ ok: true })),
    }),
  );
  await waitFor(() =>
    expect(screen.getByRole("status").textContent).toContain("已保存"),
  );
});
it("丢回执重试保留原key和冻结body，键盘移动可保存", async () => {
  await openDraft();
  fireEvent.keyDown(screen.getByRole("button", { name: "移动 图片草稿" }), {
    key: "ArrowRight",
    shiftKey: true,
  });
  fireEvent.click(screen.getByRole("button", { name: "节点参数" }));
  expect((screen.getByLabelText("X 坐标") as HTMLInputElement).value).toBe(
    "90",
  );
  fireEvent.keyDown(screen.getByLabelText("X 坐标"), { key: "Escape" });
  api.save.mockRejectedValueOnce(new RequestError("request_unconfirmed"));
  fireEvent.click(screen.getByRole("button", { name: "保存画布" }));
  await screen.findByRole("button", { name: "重试原保存" });
  const frozen = structuredClone(
    api.save.mock.calls[0][1],
  ) as API.CommandsInput;
  const key = api.save.mock.calls[0][2].headers["Idempotency-Key"];
  api.save.mockResolvedValueOnce(addReceipt(frozen));
  fireEvent.click(screen.getByRole("button", { name: "重试原保存" }));
  await waitFor(() =>
    expect(screen.getByRole("status").textContent).toContain("已保存"),
  );
  expect(api.save.mock.calls[1][1]).toEqual(frozen);
  expect(api.save.mock.calls[1][2].headers["Idempotency-Key"]).toBe(key);
});
it("未保存时返回项目和品牌首页均需明确处理", async () => {
  await openDraft();
  fireEvent.click(screen.getByRole("link", { name: "织光首页" }));
  expect(
    screen.getByRole("alertdialog", { name: "确认离开画布" }),
  ).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "留在画布" }));
  fireEvent.keyDown(screen.getByRole("button", { name: "项目选项" }), {
    key: "Enter",
  });
  fireEvent.click(await screen.findByRole("menuitem", { name: "返回项目" }));
  expect(
    screen.getByRole("alertdialog", { name: "确认离开画布" }),
  ).toBeTruthy();
});
it("即梦选择抓手切换不触发保存，输入框内V仍用于输入", async () => {
  await openDraft();
  expect(screen.getByRole("toolbar", { name: "画布工具" })).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "选择工具" }));
  expect(
    screen
      .getByRole("button", { name: "抓手工具" })
      .getAttribute("aria-pressed"),
  ).toBe("true");
  fireEvent.keyDown(screen.getByLabelText("测试共享富文本"), { key: "v" });
  expect(
    screen
      .getByRole("button", { name: "抓手工具" })
      .getAttribute("aria-pressed"),
  ).toBe("true");
  fireEvent.click(screen.getByRole("button", { name: "抓手工具" }));
  expect(
    screen
      .getByRole("button", { name: "选择工具" })
      .getAttribute("aria-pressed"),
  ).toBe("false");
  expect(api.save).not.toHaveBeenCalled();
  expect(api.rich.mock.lastCall?.[0]).toMatchObject({
    presentation: "canvas",
    canvas: { contextKey: expect.stringContaining(`canvas/${canvasID}/`) },
  });
  expect(api.flushResolved).not.toHaveBeenCalled();
});
it("冲突保留本地草稿，由用户明确采用服务端版本继续保存", async () => {
  await openDraft();
  const node = structuredClone(
    (await import("@/app/canvas/document")).createDraftNode(
      "image_generation",
      "1:1",
      0,
    ),
  );
  api.save.mockRejectedValueOnce(new RequestError("revision_conflict", 409));
  api.canvas.mockResolvedValueOnce({
    ...canvas,
    revision: 3,
    nodes: [{ ...node, input_revision: 1 }],
  });
  fireEvent.change(screen.getByLabelText("测试共享富文本"), {
    target: { value: "保留我的稿" },
  });
  fireEvent.click(screen.getByRole("button", { name: "保存画布" }));
  await screen.findByRole("button", { name: "保留本地编辑，以版本 3 继续" });
  expect(screen.getByLabelText("本地未保存内容").textContent).toContain(
    "保留我的稿",
  );
  fireEvent.click(
    screen.getByRole("button", { name: "保留本地编辑，以版本 3 继续" }),
  );
  api.save.mockImplementationOnce((_p, body) =>
    Promise.resolve({
      ...canvas,
      revision: 4,
      nodes: [
        { ...node, input_revision: 1 },
        ...body.commands[0].nodes.map((n: API.Node) => receiptNode(n)),
      ],
      results: body.commands.map(() => ({ ok: true })),
    }),
  );
  fireEvent.click(screen.getByRole("button", { name: "保存画布" }));
  await waitFor(() =>
    expect(screen.getByRole("status").textContent).toContain("已保存"),
  );
  expect(api.save.mock.calls[1][1].expected_revision).toBe(3);
});
it("并行读回真实空画布，拒绝项目与画布串线", async () => {
  api.project.mockResolvedValue(project);
  api.canvas.mockResolvedValue({ ...canvas, project_id: canvasID });
  render(<CanvasPage projectID={projectID} canvasID={canvasID} />);
  await findFeedback();
  expect(screen.queryByText("双击画布，创建节点")).toBeNull();
  expect(screen.queryByText("所有更改已保存")).toBeNull();
});
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

it("媒体节点移动使用真实命令，复制保留媒体身份，移除本地副本不发删除请求", async () => {
  const media = {
    id: "00000000-0000-4000-8000-000000000013",
    title: "海边.png",
    node_type: "image",
    node_action: "resource",
    ref_type: "media_asset",
    ref_id: "00000000-0000-4000-8000-000000000014",
    config: {},
    x: 80,
    y: 80,
    z_index: 0,
    width: 288,
    height: 192,
  };
  api.project.mockResolvedValue(project);
  api.canvas.mockResolvedValue({ ...canvas, nodes: [media] });
  api.save.mockImplementationOnce((_p, body) =>
    Promise.resolve({
      ...canvas,
      nodes: [{ ...media, x: 90 }],
      revision: 2,
      results: body.commands.map(() => ({ ok: true })),
    }),
  );
  render(<CanvasPage projectID={projectID} canvasID={canvasID} />);
  fireEvent.keyDown(
    await screen.findByRole("button", { name: "移动 海边.png" }),
    { key: "ArrowRight", shiftKey: true },
  );
  fireEvent.click(screen.getByRole("button", { name: "保存画布" }));
  await waitFor(() =>
    expect(screen.getByRole("status").textContent).toContain("已保存"),
  );
  expect(api.save.mock.calls[0][1].commands).toEqual([
    { type: "MoveNodes", moves: [{ id: media.id, x: 90, y: 80 }] },
  ]);
  fireEvent.keyDown(
    screen.getByRole("button", { name: "节点操作：海边.png" }),
    { key: "Enter" },
  );
  expect(
    screen
      .getByRole("menuitem", { name: "移除节点" })
      .getAttribute("aria-disabled"),
  ).not.toBe("true");
  fireEvent.click(screen.getByRole("menuitem", { name: "复制节点" }));
  await screen.findByRole("button", { name: "移动 海边.png 副本" });
  fireEvent.keyDown(
    screen.getByRole("button", { name: "节点操作：海边.png 副本" }),
    { key: "Enter" },
  );
  fireEvent.click(screen.getByRole("menuitem", { name: "移除节点" }));
  await waitFor(() =>
    expect(
      screen.queryByRole("button", { name: "移动 海边.png 副本" }),
    ).toBeNull(),
  );
  expect(api.save).toHaveBeenCalledOnce();
  expect(screen.getByRole("status").textContent).toContain("已保存");
});

it("文字节点可编辑、撤销重做并通过原命令保存", async () => {
  api.project.mockResolvedValue(project);
  api.canvas.mockResolvedValue(canvas);
  api.save.mockImplementation(async (_path, body: API.CommandsInput) => ({
    ...canvas,
    revision: 2,
    nodes: body.commands?.find((c) => c.type === "AddNodes")?.nodes ?? [],
    results: body.commands?.map(() => ({ ok: true })),
  }));
  render(<CanvasPage projectID={projectID} canvasID={canvasID} />);
  await screen.findByText("双击画布，创建节点");
  fireEvent.click(screen.getByRole("button", { name: "添加文本" }));
  fireEvent.change(screen.getByRole("textbox", { name: "文本内容" }), {
    target: { value: "一段真实文字" },
  });
  fireEvent.click(screen.getByRole("button", { name: "撤销编辑" }));
  expect(
    (screen.getByRole("textbox", { name: "文本内容" }) as HTMLTextAreaElement)
      .value,
  ).toBe("");
  fireEvent.click(screen.getByRole("button", { name: "重做编辑" }));
  expect(
    (screen.getByRole("textbox", { name: "文本内容" }) as HTMLTextAreaElement)
      .value,
  ).toBe("一段真实文字");
  fireEvent.click(screen.getByRole("button", { name: "保存画布" }));
  await waitFor(() => expect(api.save).toHaveBeenCalledOnce());
  expect(api.save.mock.calls[0][1].commands[0].nodes[0]).toMatchObject({
    node_type: "text",
    node_action: "resource",
    config: { text: "一段真实文字" },
  });
  await screen.findByText("已保存");
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

it("素材侧边创建视频带入授权版本，连线与引用一次撤销重做并按原接口保存", async () => {
  const asset = {
    id: "00000000-0000-4000-8000-000000000013",
    project_id: projectID,
    kind: "image",
    revision: 4,
  };
  const source = {
    id: "00000000-0000-4000-8000-000000000003",
    title: "参考图片",
    node_type: "image",
    node_action: "resource",
    ref_type: "media_asset",
    ref_id: asset.id,
    config: {},
    x: 80,
    y: 80,
    z_index: 0,
  };
  api.project.mockResolvedValue(project);
  api.canvas.mockResolvedValue({ ...canvas, nodes: [source] });
  api.preview.mockResolvedValue({ asset });
  api.save.mockImplementation(async (_path, body: API.CommandsInput) => ({
    ...canvas,
    revision: 2,
    nodes: [
      source,
      ...(body.commands?.find((c) => c.type === "AddNodes")?.nodes ?? []).map(
        (n) => receiptNode(n),
      ),
    ],
    edges: body.commands?.find((c) => c.type === "Connect")?.edges ?? [],
    results: body.commands?.map(() => ({ ok: true })),
  }));
  render(<CanvasPage projectID={projectID} canvasID={canvasID} />);
  fireEvent.click(await screen.findByRole("button", { name: "选择 参考图片" }));
  fireEvent.click(screen.getByRole("button", { name: "在右侧添加节点" }));
  fireEvent.click(await screen.findByRole("option", { name: "视频" }));
  await screen.findByRole("button", { name: "选择 视频草稿" });
  expect(api.preview).toHaveBeenCalledWith(
    { pid: projectID, asset_id: asset.id },
    expect.objectContaining({ signal: expect.any(AbortSignal) }),
  );
  expect(
    api.rich.mock.calls.at(-1)?.[0].canvas.draft.references[0].source,
  ).toEqual({ kind: "media", sourceId: asset.id, sourceVersion: "4" });
  expect(
    document.querySelectorAll('[data-connection-kind="prompt_reference"]'),
  ).toHaveLength(1);
  fireEvent.click(screen.getByRole("button", { name: "撤销编辑" }));
  expect(screen.queryByRole("button", { name: "选择 视频草稿" })).toBeNull();
  expect(document.querySelectorAll("[data-connection-id]")).toHaveLength(0);
  fireEvent.click(screen.getByRole("button", { name: "重做编辑" }));
  expect(screen.getByRole("button", { name: "选择 视频草稿" })).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "保存画布" }));
  await screen.findByText("已保存");
  const commands = api.save.mock.calls[0][1].commands as API.Command[];
  expect(commands.map((c) => c.type)).toEqual(["AddNodes", "Connect"]);
  expect(commands[1].edges?.[0]).toMatchObject({
    edge_type: "annotation",
    source_node_id: source.id,
  });
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
  fireEvent.click(screen.getByRole("button", { name: "解除分组关系" }));
  await findFeedback("分组包含只读成员，不能解除关系。");
  expect(
    screen.getByRole("button", { name: "保存画布" }).hasAttribute("disabled"),
  ).toBe(true);
});

it.each([403, 413, 422])(
  "确定性 %i 拒绝后暂停自动重发，修改后可继续保存",
  async (status) => {
    await openDraft();
    vi.useFakeTimers();
    api.save.mockRejectedValue(new RequestError("invalid_request", status));
    fireEvent.click(screen.getByRole("button", { name: "保存画布" }));
    await act(async () => {});
    expect(api.save).toHaveBeenCalledOnce();
    await act(async () => {
      await vi.advanceTimersByTimeAsync(4000);
    });
    expect(api.save).toHaveBeenCalledOnce();
    fireEvent.change(screen.getByLabelText("测试共享富文本"), {
      target: { value: "已修正的输入" },
    });
    api.save.mockImplementation(async (_path, body) => addReceipt(body));
    await act(async () => {
      await vi.advanceTimersByTimeAsync(900);
    });
    expect(api.save).toHaveBeenCalledTimes(2);
    expect(api.save.mock.calls[1][2].headers["Idempotency-Key"]).not.toBe(
      api.save.mock.calls[0][2].headers["Idempotency-Key"],
    );
    expect(screen.getByRole("status").textContent).toContain("已保存");
  },
);
it("确定性拒绝保留手动保存入口，不自动重发", async () => {
  await openDraft();
  api.save.mockRejectedValueOnce(new RequestError("invalid_request", 422));
  fireEvent.click(screen.getByRole("button", { name: "保存画布" }));
  await findFeedback();
  api.save.mockImplementation(async (_path, body) => addReceipt(body));
  fireEvent.click(screen.getByRole("button", { name: "保存画布" }));
  await waitFor(() =>
    expect(screen.getByRole("status").textContent).toContain("已保存"),
  );
  expect(api.save).toHaveBeenCalledTimes(2);
});

it("即梦缩放菜单和快捷键不抢输入，副本保留独立身份", async () => {
  await openDraft();
  const board = screen.getByLabelText("画布工作区");
  fireEvent.keyDown(board, { key: "+", ctrlKey: true });
  expect(screen.getByRole("button", { name: /缩放比例 120%/ })).toBeTruthy();
  fireEvent.keyDown(screen.getByLabelText("测试共享富文本"), {
    key: "1",
    ctrlKey: true,
  });
  expect(screen.getByRole("button", { name: /缩放比例 120%/ })).toBeTruthy();
  fireEvent.keyDown(board, { key: "1", ctrlKey: true });
  expect(screen.getByRole("button", { name: /缩放比例 100%/ })).toBeTruthy();
  fireEvent.keyDown(board, { key: "d", ctrlKey: true });
  expect(document.querySelectorAll("[data-node-id]")).toHaveLength(2);
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

function currentComposerOwner() {
  const props = api.rich.mock.lastCall?.[0] as
    | import("react").ComponentProps<
        typeof import("@/components/generation/rich-generation").RichGeneration
      >
    | undefined;
  if (!props?.canvas) throw new Error("Shared canvas owner was not rendered");
  return props.canvas;
}

it("共享onChange写入richCanvasDraft，flush只在真实保存回执后确认版本", async () => {
  await openDraft();
  fireEvent.change(screen.getByLabelText("测试共享富文本"), {
    target: { value: "共享输入的真实意图" },
  });
  let release!: (value: unknown) => void;
  api.save.mockImplementationOnce(
    () =>
      new Promise((resolve) => {
        release = resolve;
      }),
  );
  fireEvent.click(screen.getByRole("button", { name: "测试确认共享输入" }));
  await waitFor(() => expect(api.save).toHaveBeenCalledOnce());
  const body = structuredClone(api.save.mock.calls[0][1]) as API.CommandsInput;
  const added = body.commands?.find((command) => command.type === "AddNodes")
    ?.nodes?.[0];
  expect(added?.config?.draft).toMatchObject({
    prompt: "",
    negative_prompt: "",
    aspect_ratio: "",
    candidate_count: 0,
    composer: {
      document: {
        type: "doc",
        content: [
          {
            type: "paragraph",
            content: [{ type: "text", text: "共享输入的真实意图" }],
          },
        ],
      },
      params: { aspect_ratio: "1:1", output_count: 1 },
    },
  });
  expect(api.flushResolved).not.toHaveBeenCalled();
  await act(async () => release(addReceipt(body)));
  await waitFor(() =>
    expect(api.flushResolved).toHaveBeenCalledWith({
      kind: "canvas_input",
      canvas_id: canvasID,
      node_id: added!.id,
      canvas_revision: 2,
      input_revision: 1,
    }),
  );
  expect(api.flushRejected).not.toHaveBeenCalled();
});

it("首次共享boot的flush升级legacy草稿并使用回执中的输入版本，不重复保存", async () => {
  const { createDraftNode } = await import("@/app/canvas/document");
  const legacy = {
    ...createDraftNode("image_generation", "16:9", 0),
    input_revision: 4,
  };
  legacy.config.draft.prompt = "已保存的旧意图";
  legacy.config.draft.negative_prompt = "保留负面要求";
  const remote = { ...canvas, revision: 7, nodes: [legacy] };
  api.project.mockResolvedValue(project);
  api.canvas.mockResolvedValue(remote);
  api.save.mockImplementation(async (_path, body: API.CommandsInput) => ({
    ...remote,
    revision: 8,
    nodes: [
      receiptNode(
        {
          ...legacy,
          config: body.commands?.find(
            (command) => command.type === "UpdateNodeConfig",
          )?.config,
        },
        5,
      ),
    ],
    results: body.commands?.map(() => ({ ok: true })),
  }));
  render(<CanvasPage projectID={projectID} canvasID={canvasID} />);
  fireEvent.click(await screen.findByRole("button", { name: "移动 图片草稿" }));
  expect(
    (screen.getByLabelText("测试共享富文本") as HTMLInputElement).value,
  ).toBe("已保存的旧意图");
  // This is the same owner.flush call made by RichGeneration's canvas boot.
  fireEvent.click(screen.getByRole("button", { name: "测试确认共享输入" }));
  await waitFor(() => expect(api.flushResolved).toHaveBeenCalledOnce());
  const body = api.save.mock.calls[0][1] as API.CommandsInput;
  expect(body.expected_revision).toBe(7);
  expect(body.commands).toHaveLength(1);
  expect(body.commands![0]).toMatchObject({
    type: "UpdateNodeConfig",
    id: legacy.id,
    config: {
      draft: {
        prompt: "",
        negative_prompt: "",
        aspect_ratio: "",
        candidate_count: 0,
        composer: {
          params: {
            aspect_ratio: "16:9",
            output_count: 1,
            negative_prompt: "保留负面要求",
          },
        },
      },
    },
  });
  expect(api.flushResolved).toHaveBeenLastCalledWith({
    kind: "canvas_input",
    canvas_id: canvasID,
    node_id: legacy.id,
    canvas_revision: 8,
    input_revision: 5,
  });
  fireEvent.click(screen.getByRole("button", { name: "测试确认共享输入" }));
  await waitFor(() => expect(api.flushResolved).toHaveBeenCalledTimes(2));
  expect(api.save).toHaveBeenCalledOnce();
});

it("共享flush未知结果不返回可生成版本，重试使用原幂等键与冻结输入", async () => {
  await openDraft();
  fireEvent.change(screen.getByLabelText("测试共享富文本"), {
    target: { value: "等待确认的输入" },
  });
  api.save.mockRejectedValueOnce(new RequestError("request_unconfirmed"));
  fireEvent.click(screen.getByRole("button", { name: "测试确认共享输入" }));
  await waitFor(() => expect(api.flushRejected).toHaveBeenCalledOnce());
  expect(api.flushResolved).not.toHaveBeenCalled();
  const body = structuredClone(api.save.mock.calls[0][1]) as API.CommandsInput;
  const key = api.save.mock.calls[0][2].headers["Idempotency-Key"];
  api.save.mockResolvedValueOnce(addReceipt(body));
  fireEvent.click(screen.getByRole("button", { name: "测试确认共享输入" }));
  await waitFor(() => expect(api.flushResolved).toHaveBeenCalledOnce());
  expect(api.save.mock.calls[1][1]).toEqual(body);
  expect(api.save.mock.calls[1][2].headers["Idempotency-Key"]).toBe(key);
  expect(api.flushResolved).toHaveBeenLastCalledWith(
    expect.objectContaining({
      kind: "canvas_input",
      canvas_revision: 2,
      input_revision: 1,
    }),
  );
});

it("撤权后迟到的共享onChange与flush不写入画布，也不确认生成输入版本", async () => {
  await openDraft();
  const owner = currentComposerOwner();
  const draft = structuredClone(owner.draft);
  draft.document = {
    type: "doc",
    content: [
      {
        type: "paragraph",
        content: [{ type: "text", text: "撤权后的迟到编辑" }],
      },
    ],
  };
  await act(async () => {
    window.dispatchEvent(new Event("glowloom:private-cache-cleared"));
  });
  expect(screen.getByText("请重新打开画布")).toBeTruthy();
  expect(screen.getByRole("link", { name: "重新登录" })).toBeTruthy();
  await act(async () => owner.onChange(draft));
  await expect(owner.flush()).rejects.toThrow("画布保存尚未确认");
  expect(api.save).not.toHaveBeenCalled();
  expect(api.flushResolved).not.toHaveBeenCalled();
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
it("数据库保存不依赖本地分支库且卸载重开读取已确认数据", async () => {
  api.branches.mockRejectedValue(new DOMException("blocked", "SecurityError"));
  await openDraft();
  api.save.mockImplementation(async (_p, body) => addReceipt(body));
  fireEvent.click(screen.getByRole("button", { name: "保存画布" }));
  await screen.findByText("已保存");
  expect(api.write).not.toHaveBeenCalled();
  const committed = await api.save.mock.results[0].value;
  cleanup();
  api.canvas.mockResolvedValue(committed);
  render(<CanvasPage projectID={projectID} canvasID={canvasID} />);
  await screen.findByRole("button", { name: "移动 图片草稿" });
  expect(api.canvas).toHaveBeenCalledTimes(2);
  expect(api.save).toHaveBeenCalledOnce();
});

it("后端返回整图只读拒绝后停止后续编辑", async () => {
  await openDraft();
  api.save.mockRejectedValue(new RequestError("canvas_graph_read_only", 422));
  fireEvent.click(screen.getByRole("button", { name: "保存画布" }));
  await screen.findByText("画布数据待兼容 · 只读");
  expect(
    (screen.getByRole("button", { name: "保存画布" }) as HTMLButtonElement)
      .disabled,
  ).toBe(true);
  expect(api.save).toHaveBeenCalledOnce();
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

it.each([
  new RequestError("request_unconfirmed"),
  new RequestError("dependency_unavailable", 503),
])(
  "登录核验瞬时失败保留原结果未知请求，重核验后重试同键同体 %s",
  async (error) => {
    await openDraft();
    api.save.mockRejectedValueOnce(new RequestError("request_unconfirmed"));
    fireEvent.click(screen.getByRole("button", { name: "保存画布" }));
    await screen.findByRole("button", { name: "重试原保存" });
    const [path, body, options] = api.save.mock.calls[0];
    api.identity.mockRejectedValueOnce(error);
    fireEvent(document, new Event("visibilitychange"));
    await screen.findByText("登录状态待核验 · 只读");
    expect(screen.queryByText("请重新打开画布")).toBeNull();
    expect(screen.getByRole("button", { name: "查看 图片草稿" })).toBeTruthy();
    fireEvent.click(
      await screen.findByRole("button", { name: "重新核验登录" }),
    );
    await waitFor(() =>
      expect(
        (
          screen.getByRole("button", {
            name: "重试原保存",
          }) as HTMLButtonElement
        ).disabled,
      ).toBe(false),
    );
    api.save.mockResolvedValueOnce(addReceipt(body));
    fireEvent.click(screen.getByRole("button", { name: "重试原保存" }));
    await screen.findByText("已保存");
    expect(api.save.mock.calls[1][0]).toEqual(path);
    expect(api.save.mock.calls[1][1]).toEqual(body);
    expect(api.save.mock.calls[1][2].headers).toEqual(options.headers);
  },
);
it.each([
  new RequestError("unauthenticated", 401),
  { actor: { id: "another-actor" } },
])("明确失效或切换账号才清除画布私有内容 %s", async (result) => {
  await openDraft();
  if (result instanceof Error) api.identity.mockRejectedValueOnce(result);
  else api.identity.mockResolvedValueOnce(result);
  fireEvent(document, new Event("visibilitychange"));
  await screen.findByText("请重新打开画布");
  expect(screen.queryByRole("button", { name: /图片草稿/ })).toBeNull();
  expect(api.save).not.toHaveBeenCalled();
});
it("冲突读回未知生成配置可只读查看和明确载入，不解析未知草稿", async () => {
  const { createDraftNode } = await import("@/app/canvas/document");
  await openDraft();
  const node = {
    ...createDraftNode("image_generation", "1:1", 0),
    title: "未知版本草稿",
    config: { future: { version: 99 } },
  };
  const server = {
    ...canvas,
    revision: 2,
    nodes: [node],
    read_only_reason: "unknown config",
  };
  api.canvas.mockResolvedValueOnce(server);
  api.save.mockRejectedValueOnce(new RequestError("revision_conflict", 409));
  fireEvent.click(screen.getByRole("button", { name: "保存画布" }));
  await screen.findByText("画布数据待兼容 · 只读");
  expect(screen.getByRole("region", { name: "版本冲突处理" })).toBeTruthy();
  expect(
    (
      screen.getByRole("button", {
        name: "保留本地编辑，以版本 2 继续",
      }) as HTMLButtonElement
    ).disabled,
  ).toBe(true);
  fireEvent.click(
    screen.getByRole("button", { name: "放弃本地编辑，载入服务端版本" }),
  );
  await screen.findByRole("heading", { name: "未知版本草稿" });
  expect(node.config).toEqual({ future: { version: 99 } });
  expect(api.save).toHaveBeenCalledOnce();
});
it("只读拒绝后未保存文本仍触发刷新和站内离开保护", async () => {
  api.project.mockResolvedValue(project);
  api.canvas.mockResolvedValue(canvas);
  api.save.mockRejectedValueOnce(
    new RequestError("canvas_graph_read_only", 422),
  );
  render(<CanvasPage projectID={projectID} canvasID={canvasID} />);
  await screen.findByText("双击画布，创建节点");
  fireEvent.click(screen.getByRole("button", { name: "添加文本" }));
  fireEvent.change(screen.getByRole("textbox", { name: "文本内容" }), {
    target: { value: "尚未保存的文本" },
  });
  fireEvent.click(screen.getByRole("button", { name: "保存画布" }));
  await screen.findByText("画布数据待兼容 · 只读");
  const unload = new Event("beforeunload", { cancelable: true });
  window.dispatchEvent(unload);
  expect(unload.defaultPrevented).toBe(true);
  fireEvent.click(screen.getByRole("link", { name: "织光首页" }));
  await screen.findByRole("heading", { name: "确认离开画布" });
});

it("远端快照更新干净画布，后续提交以数据库新版本为基线", async () => {
  api.project.mockResolvedValue(project);
  api.canvas.mockResolvedValue(canvas);
  render(<CanvasPage projectID={projectID} canvasID={canvasID} />);
  await screen.findByText("双击画布，创建节点");
  const { createResourceNode } = await import("@/app/canvas/document");
  const node = {
    ...createResourceNode("text", { x: 0, y: 0 }),
    title: "远端文本",
    config: { text: "数据库新内容" },
  };
  act(() =>
    api.live.mock.lastCall![0].snapshot(
      { ...canvas, revision: 2, nodes: [node] },
      true,
    ),
  );
  await screen.findByRole("button", { name: "移动 远端文本" });
  fireEvent.click(screen.getByRole("button", { name: "添加文本" }));
  api.save.mockImplementation((_p, body) =>
    Promise.resolve({
      ...canvas,
      revision: 3,
      nodes: [node, ...body.commands[0].nodes],
      results: body.commands.map(() => ({ ok: true })),
    }),
  );
  fireEvent.click(screen.getByRole("button", { name: "保存画布" }));
  await waitFor(() => expect(api.save).toHaveBeenCalledOnce());
  expect(api.save.mock.calls[0][1].expected_revision).toBe(2);
});
it("本地有编辑时远端新版本保留本地内容并阻止自动覆盖", async () => {
  await openDraft();
  fireEvent.change(screen.getByLabelText("测试共享富文本"), {
    target: { value: "保留未提交意图" },
  });
  act(() =>
    api.live.mock.lastCall![0].snapshot({ ...canvas, revision: 2 }, true),
  );
  expect(screen.getByLabelText("本地未保存内容").textContent).toContain(
    "保留未提交意图",
  );
  expect(screen.getByRole("region", { name: "版本冲突处理" })).toBeTruthy();
  await new Promise((resolve) => setTimeout(resolve, 1000));
  expect(api.save).not.toHaveBeenCalled();
});
it("断线时显示连接状态并暂停新编辑，恢复后允许继续", async () => {
  await openDraft();
  act(() => api.live.mock.lastCall![0].status("reconnecting"));
  expect(screen.getByRole("status").textContent).toContain("连接中断");
  expect(
    screen
      .getByRole("toolbar", { name: "画布工具" })
      .getAttribute("aria-disabled"),
  ).toBe("true");
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
  act(() =>
    api.live.mock.lastCall![0].snapshot(
      { ...canvas, revision: 2, nodes: [text, generation] },
      true,
    ),
  );
  const comparison = screen.getByLabelText("服务端内容");
  expect(comparison.textContent).toContain("远端修改的正文");
  expect(comparison.textContent).toContain("远端富文本提示词");
  expect(comparison.textContent).toContain("连线");
  expect(comparison.textContent).toContain("删除");
});
