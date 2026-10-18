import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { CanvasAIPanel } from "./canvas-ai-panel";
import type { Project } from "@/components/projects/project";
import type { Node } from "./editor-state";

const api = vi.hoisted(() => ({
  list: vi.fn(),
  get: vi.fn(),
  cancel: vi.fn(),
}));
vi.mock("@/api/operations", () => ({
  listProjectOperations: api.list,
  getOperation: api.get,
  cancelOperation: api.cancel,
}));
const project: Project = {
  id: "11111111-1111-4111-8111-111111111111",
  canvas_id: null,
  name: "创作",
  aspect_ratio: "1:1",
  style_key: null,
  revision: 1,
  update_time: "2026-10-18T00:00:00Z",
};
const task = {
  id: "22222222-2222-4222-8222-222222222222",
  project_id: project.id,
  capability: "text.generate",
  status: "completed",
  inputs: [{ role: "prompt", text: "真实原始需求" }],
  outputs: [
    {
      id: "33333333-3333-4333-8333-333333333333",
      kind: "json",
      moderation_status: "passed",
      text: "供应商返回的真实剧本",
    },
  ],
};
const props = () => ({
  project,
  nodes: [],
  onGenerate: vi.fn(),
  onInsertText: vi.fn(),
  onClose: vi.fn(),
});
beforeEach(() => {
  vi.clearAllMocks();
  api.list.mockResolvedValue({ items: [] });
  api.get.mockResolvedValue(task);
  api.cancel.mockResolvedValue({});
});
afterEach(cleanup);

it("五个真实创作模式通过现有报价入口提交，关闭有响应", async () => {
  const p = props();
  render(<CanvasAIPanel {...p} />);
  await screen.findByText("还没有生成任务");
  expect(screen.getByRole("radio", { name: "全流程广告片导演" })).toBeTruthy();
  fireEvent.change(screen.getByRole("textbox", { name: "创作需求" }), {
    target: { value: "写一段开场" },
  });
  fireEvent.click(screen.getByRole("button", { name: "选择模型并获取报价" }));
  expect(p.onGenerate).toHaveBeenCalledWith(
    expect.objectContaining({
      project,
      capability: "text",
      prompt: expect.stringContaining("写一段开场"),
    }),
  );
  fireEvent.click(screen.getByRole("button", { name: "关闭 AI 创作" }));
  expect(p.onClose).toHaveBeenCalledOnce();
});
it("只展示任务真实文本，采用失败可重试且连续创作包含真实上文", async () => {
  api.list.mockResolvedValue({ items: [task] });
  const p = props();
  p.onInsertText
    .mockRejectedValueOnce(new Error("画布保存失败"))
    .mockResolvedValue(undefined);
  render(<CanvasAIPanel {...p} />);
  await screen.findByText("供应商返回的真实剧本");
  fireEvent.click(screen.getByRole("button", { name: "插入画布" }));
  await screen.findByText("画布保存失败");
  fireEvent.click(screen.getByRole("button", { name: "插入画布" }));
  await waitFor(() => expect(p.onInsertText).toHaveBeenCalledTimes(2));
  fireEvent.click(screen.getByRole("button", { name: "继续创作" }));
  fireEvent.change(screen.getByRole("textbox", { name: "创作需求" }), {
    target: { value: "再短一点" },
  });
  fireEvent.click(screen.getByRole("button", { name: "选择模型并获取报价" }));
  expect(p.onGenerate).toHaveBeenLastCalledWith(
    expect.objectContaining({
      prompt: expect.stringContaining("供应商返回的真实剧本"),
    }),
  );
});
it("读取失败可重试，活动任务取消后重新读取权威状态", async () => {
  api.list
    .mockRejectedValueOnce(new Error("offline"))
    .mockResolvedValue({ items: [{ ...task, status: "submitted" }] });
  api.get.mockResolvedValue({ ...task, status: "submitted", outputs: [] });
  render(<CanvasAIPanel {...props()} />);
  fireEvent.click(await screen.findByRole("button", { name: "重试读取" }));
  fireEvent.click(await screen.findByRole("button", { name: "取消任务" }));
  await waitFor(() =>
    expect(api.cancel).toHaveBeenCalledWith(
      { id: task.id },
      { project_id: project.id },
    ),
  );
  expect(screen.queryByText("供应商返回的真实剧本")).toBeNull();
});
it("未知状态不提供重复生成或文本采用", async () => {
  api.list.mockResolvedValue({ items: [{ ...task, status: "unknown" }] });
  api.get.mockResolvedValue({ ...task, status: "unknown", outputs: [] });
  render(<CanvasAIPanel {...props()} />);
  await screen.findByText("结果待确认");
  expect(screen.queryByRole("button", { name: "重新生成" })).toBeNull();
  expect(screen.queryByRole("button", { name: "插入画布" })).toBeNull();
});
it("可重试失败任务重新校验状态并回到报价，不重提未知任务", async () => {
  const failed = { ...task, status: "failed", retryable: true, outputs: [] };
  api.list.mockResolvedValue({ items: [failed] });
  api.get.mockResolvedValue(failed);
  const p = props();
  render(<CanvasAIPanel {...p} />);
  fireEvent.click(await screen.findByRole("button", { name: "重新生成" }));
  await waitFor(() =>
    expect(p.onGenerate).toHaveBeenCalledWith(
      expect.objectContaining({
        capability: "text",
        reuse: expect.objectContaining({ id: task.id }),
      }),
    ),
  );
  api.get.mockResolvedValue({ ...failed, status: "unknown" });
  fireEvent.click(screen.getByRole("button", { name: "重新生成" }));
  await screen.findByText("任务状态已变化，请刷新后重试。");
  expect(p.onGenerate).toHaveBeenCalledOnce();
});
it("选中媒体明确阻止不受支持的文字请求，移除参考后可以报价", async () => {
  const media = {
    id: "node",
    title: "森林参考",
    node_type: "video",
    ref_type: "media_asset",
    ref_id: task.id,
    config: {},
  } as Node;
  const p = props();
  render(<CanvasAIPanel {...p} nodes={[media]} selectedNode={media} />);
  await screen.findByText("还没有生成任务");
  fireEvent.change(screen.getByRole("textbox", { name: "创作需求" }), {
    target: { value: "镜头一是森林" },
  });
  fireEvent.click(screen.getByRole("button", { name: "选择模型并获取报价" }));
  await screen.findByText(
    "当前文字生成不支持读取媒体，请移除本次参考并提供镜头或画面文字描述。",
  );
  expect(p.onGenerate).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole("button", { name: "移除本次参考" }));
  fireEvent.click(screen.getByRole("button", { name: "选择模型并获取报价" }));
  expect(p.onGenerate).toHaveBeenCalledOnce();
});
it("项目切换中止旧详情读取并清除已输入需求", async () => {
  api.list.mockResolvedValue({ items: [task] });
  let finish: (value: unknown) => void = () => {};
  api.get.mockImplementationOnce(
    () =>
      new Promise((resolve) => {
        finish = resolve;
      }),
  );
  const p = props();
  const view = render(<CanvasAIPanel {...p} />);
  await waitFor(() => expect(api.get).toHaveBeenCalledOnce());
  fireEvent.change(screen.getByRole("textbox", { name: "创作需求" }), {
    target: { value: "旧项目需求" },
  });
  api.list.mockResolvedValue({ items: [] });
  view.rerender(
    <CanvasAIPanel
      {...p}
      project={{ ...project, id: "44444444-4444-4444-8444-444444444444" }}
    />,
  );
  finish(task);
  await screen.findByText("还没有生成任务");
  expect(screen.queryByText("供应商返回的真实剧本")).toBeNull();
  expect(
    (screen.getByRole("textbox", { name: "创作需求" }) as HTMLTextAreaElement)
      .value,
  ).toBe("");
});
