import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { CanvasPage } from "./canvas-page";
import { AuthError } from "@/components/auth/request";
const api = vi.hoisted(() => ({
  project: vi.fn(),
  canvas: vi.fn(),
  save: vi.fn(),
}));
vi.mock("@/components/projects/generated/projects", () => ({
  getProject: api.project,
  getCanvas: api.canvas,
  applyCanvasCommands: api.save,
}));
const projectID = "00000000-0000-4000-8000-000000000001";
const canvasID = "00000000-0000-4000-8000-000000000002";
const project = {
  id: projectID,
  canvas_id: canvasID,
  name: "真实画布项目",
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
beforeEach(() => vi.clearAllMocks());
afterEach(cleanup);
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
  await screen.findByText("画布暂无节点");
  fireEvent.click(screen.getByRole("button", { name: "新增图片节点" }));
}
function addReceipt(body: ProjectAPI.CommandsInput) {
  return {
    ...canvas,
    revision: 2,
    nodes: body.commands?.[0].nodes?.map((n) => ({ ...n, input_revision: 1 })),
    results: body.commands?.map(() => ({ ok: true })),
  };
}
it("保存中的后续编辑不会被旧回执标成已保存", async () => {
  await openDraft();
  fireEvent.change(screen.getByLabelText("提示词"), {
    target: { value: "冻结提示" },
  });
  let finish!: (value: unknown) => void;
  api.save.mockImplementationOnce(
    () =>
      new Promise((resolve) => {
        finish = resolve;
      }),
  );
  fireEvent.click(screen.getByRole("button", { name: "保存画布" }));
  const frozen = structuredClone(
    api.save.mock.calls[0][1],
  ) as ProjectAPI.CommandsInput;
  fireEvent.change(screen.getByLabelText("提示词"), {
    target: { value: "保存中继续编辑" },
  });
  await act(async () => finish(addReceipt(frozen)));
  expect((screen.getByLabelText("提示词") as HTMLTextAreaElement).value).toBe(
    "保存中继续编辑",
  );
  expect(screen.getByRole("status").textContent).toContain("未保存");
  expect(screen.queryByText("所有更改已保存")).toBeNull();
});
it("丢回执重试保留原key和冻结body，键盘移动可保存", async () => {
  await openDraft();
  fireEvent.keyDown(screen.getByRole("button", { name: "移动 图片草稿" }), {
    key: "ArrowRight",
    shiftKey: true,
  });
  expect((screen.getByLabelText("X 坐标") as HTMLInputElement).value).toBe(
    "90",
  );
  api.save.mockRejectedValueOnce(new AuthError("request_unconfirmed"));
  fireEvent.click(screen.getByRole("button", { name: "保存画布" }));
  await screen.findByRole("button", { name: "重试原保存" });
  const frozen = structuredClone(
    api.save.mock.calls[0][1],
  ) as ProjectAPI.CommandsInput;
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
  fireEvent.click(screen.getByRole("link", { name: "浮光首页" }));
  expect(
    screen.getByRole("alertdialog", { name: "确认离开画布" }),
  ).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "留在画布" }));
  fireEvent.click(screen.getByRole("link", { name: "返回项目" }));
  expect(
    screen.getByRole("alertdialog", { name: "确认离开画布" }),
  ).toBeTruthy();
});
it("BeefTV浮动工具栏和快捷键不触发保存，输入框内H仍用于输入", async () => {
  await openDraft();
  expect(screen.getByRole("toolbar", { name: "画布工具" })).toBeTruthy();
  fireEvent.keyDown(window, { key: "h" });
  expect(
    screen
      .getByRole("button", { name: "抓手工具" })
      .getAttribute("aria-pressed"),
  ).toBe("true");
  fireEvent.keyDown(screen.getByLabelText("提示词"), { key: "v" });
  expect(
    screen
      .getByRole("button", { name: "抓手工具" })
      .getAttribute("aria-pressed"),
  ).toBe("true");
  fireEvent.click(screen.getByRole("button", { name: "区域选择" }));
  expect(
    screen
      .getByRole("button", { name: "区域选择" })
      .getAttribute("aria-pressed"),
  ).toBe("true");
  expect(api.save).not.toHaveBeenCalled();
  expect(
    screen
      .getByRole("button", { name: "生成当前节点" })
      .hasAttribute("disabled"),
  ).toBe(true);
});
it("冲突保留本地草稿，由用户明确采用服务端版本继续保存", async () => {
  await openDraft();
  const node = structuredClone(
    (await import("./editor-state")).createDraftNode(
      "image_generation",
      "1:1",
      0,
    ),
  );
  api.save.mockRejectedValueOnce(new AuthError("revision_conflict", 409));
  api.canvas.mockResolvedValueOnce({
    ...canvas,
    revision: 3,
    nodes: [{ ...node, input_revision: 1 }],
  });
  fireEvent.change(screen.getByLabelText("提示词"), {
    target: { value: "保留我的稿" },
  });
  fireEvent.click(screen.getByRole("button", { name: "保存画布" }));
  await screen.findByRole("button", { name: "保留本地编辑，以版本 3 继续" });
  expect((screen.getByLabelText("提示词") as HTMLTextAreaElement).value).toBe(
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
        ...body.commands[0].nodes.map((n: ProjectAPI.Node) => ({
          ...n,
          input_revision: 1,
        })),
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
  await screen.findByRole("alert");
  expect(screen.queryByText("画布暂无节点")).toBeNull();
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
  expect(screen.getByText("画布暂无节点")).toBeTruthy();
});
