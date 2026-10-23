import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { GenerationHistory } from "@/components/media/generation-history";
import { listProjectOperations } from "@/api/operations";
const pid = "11111111-1111-4111-8111-111111111111";
vi.mock("@/api/operations", () => ({ listProjectOperations: vi.fn() }));
vi.mock("@/components/auth/session", () => ({
  useOptionalSession: () => ({ actor: { id: "actor" } }),
}));
vi.mock("@/components/spaces/space-context", () => ({
  useSpace: () => ({ selected: { id: "scope" } }),
}));
vi.mock("@/components/projects/project-picker", () => ({
  ProjectPicker: ({ onChange }: { onChange: (p: unknown) => void }) => (
    <button onClick={() => onChange({ id: pid, name: "测试项目" })}>
      选择项目
    </button>
  ),
}));
vi.mock("@/components/generation/generation-dialog", () => ({
  TaskDetail: ({ id }: { id: string }) => <p>任务详情 {id}</p>,
}));
beforeEach(() => {
  vi.mocked(listProjectOperations).mockReset();
});
afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});
it("选择项目后读取真实任务并按原游标翻页，打开任务详情", async () => {
  const first = "22222222-2222-4222-8222-222222222222";
  vi.mocked(listProjectOperations)
    .mockResolvedValueOnce({
      items: [
        {
          id: first,
          project_id: pid,
          status: "completed",
          model_name: "测试模型",
          create_time: "2026-10-23T00:00:00Z",
        },
      ],
      next_cursor: "opaque-cursor",
    })
    .mockResolvedValueOnce({ items: [] });
  render(<GenerationHistory />);
  expect(listProjectOperations).not.toHaveBeenCalled();
  fireEvent.click(screen.getByText("选择项目"));
  fireEvent.click(await screen.findByRole("button", { name: /测试模型/ }));
  expect(await screen.findByText(`任务详情 ${first}`)).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "关闭对话框" }));
  fireEvent.click(await screen.findByRole("button", { name: "更多记录" }));
  await waitFor(() => expect(listProjectOperations).toHaveBeenCalledTimes(2));
  expect(vi.mocked(listProjectOperations).mock.calls[1][0]).toMatchObject({
    pid,
    cursor: "opaque-cursor",
  });
});
it("读取失败提供重试，不把失败展示为空历史", async () => {
  vi.mocked(listProjectOperations).mockRejectedValueOnce(new Error("network"));
  render(<GenerationHistory />);
  fireEvent.click(screen.getByText("选择项目"));
  expect(await screen.findByRole("alert")).toBeTruthy();
  expect(screen.queryByText("还没有生成记录")).toBeNull();
  expect(screen.getByRole("button", { name: "重试读取" })).toBeTruthy();
});
