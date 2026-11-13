import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { SWRConfig } from "swr";
import { CanvasHistoryDialog } from "@/components/canvas/history/canvas-history-dialog";
import { CanvasMediaContext } from "@/components/canvas/lib/canvas-media-context";
import { listProjectOperations } from "@/api/operations";
vi.mock("@/api/operations", () => ({ listProjectOperations: vi.fn() }));
vi.mock("@/components/feedback/feedback-toast", () => ({
  FeedbackToast: ({
    message,
    action,
  }: {
    message: string;
    action: { label: string; onClick: () => void };
  }) => (
    <div role="alert">
      {message}
      <button onClick={action.onClick}>{action.label}</button>
    </div>
  ),
}));
vi.mock("@/components/media/project-media", () => ({
  ProjectMediaPreview: () => <span>图片预览</span>,
}));
const locate = vi.fn();
const projectId = "11111111-1111-4111-8111-111111111111",
  canvasId = "22222222-2222-4222-8222-222222222222",
  id = "33333333-3333-4333-8333-333333333333";
function History() {
  return (
    <SWRConfig value={{ provider: () => new Map(), dedupingInterval: 0 }}>
      <CanvasMediaContext value={{ projectId, canvasId, bindingRevision: "1" }}>
        <CanvasHistoryDialog onLocate={locate}>
          <button>历史入口</button>
        </CanvasHistoryDialog>
      </CanvasMediaContext>
    </SWRConfig>
  );
}
beforeEach(() => vi.resetAllMocks());
afterEach(cleanup);
it("无需旧认证Provider即可打开当前画布真实历史，类型筛选透传并可读详情", async () => {
  vi.mocked(listProjectOperations).mockResolvedValue({
    items: [
      {
        id,
        project_id: projectId,
        status: "completed",
        model_name: "测试模型",
        canvas_prompt_preview: "一只白色陶瓷杯",
        canvas_results: [{ asset_id: id, kind: "image" }],
        source: { canvas_id: canvasId, node_id: id },
      },
    ],
  });
  render(<History />);
  expect(listProjectOperations).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole("button", { name: "历史入口" }));
  fireEvent.click(await screen.findByRole("button", { name: "测试模型详情" }));
  expect(await screen.findByText("一只白色陶瓷杯")).toBeTruthy();
  expect(vi.mocked(listProjectOperations).mock.calls[0][0]).toMatchObject({
    pid: projectId,
    canvas_id: canvasId,
  });
  fireEvent.click(screen.getByRole("button", { name: /定位测试模型结果/ }));
  expect(locate).toHaveBeenCalledWith(id);
  fireEvent.click(screen.getByRole("button", { name: "历史入口" }));
  fireEvent.mouseDown(screen.getByRole("tab", { name: "视频" }));
  await waitFor(() =>
    expect(vi.mocked(listProjectOperations).mock.lastCall?.[0].capability).toBe(
      "video.generate",
    ),
  );
});
it("读取失败不冒充空态，重试成功后显示空态", async () => {
  vi.mocked(listProjectOperations)
    .mockRejectedValueOnce(new Error("offline"))
    .mockResolvedValue({ items: [] });
  render(<History />);
  fireEvent.click(screen.getByRole("button", { name: "历史入口" }));
  expect(await screen.findByRole("alert")).toBeTruthy();
  expect(screen.queryByText("暂无生成历史")).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "重新读取" }));
  expect(await screen.findByText("暂无生成历史")).toBeTruthy();
});
