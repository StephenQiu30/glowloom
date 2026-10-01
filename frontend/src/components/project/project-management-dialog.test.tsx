import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { ApiError } from "@/lib/request";
import { ProjectManagementDialog } from "./project-management-dialog";

const api = vi.hoisted(() => ({
  get: vi.fn(),
  update: vi.fn(),
  transition: vi.fn(),
  presets: vi.fn(),
}));
vi.mock("./queries", () => ({
  PROJECTS_KEY: ["projects"],
  getProject: api.get,
  updateProject: api.update,
  transitionProject: api.transition,
  listStylePresets: api.presets,
}));
const summary = {
  id: "9817c918-e49d-4dc8-b8b6-c92833b135e4",
  name: "逆光",
  aspect_ratio: "16:9" as const,
  style_type: "realistic" as const,
  status: "active" as const,
  is_delete: false,
  revision: 7,
};
const detail = {
  ...summary,
  description: "已保存的故事",
  style_preset_id: null,
  resolution: "1080p" as const,
  allow_overseas_models: true,
  default_models: {},
  archived_at: null,
  delete_time: null,
  purge_after: null,
  create_time: "2026-10-01T08:00:00Z",
  update_time: "2026-10-01T08:00:00Z",
};
beforeEach(() => {
  vi.stubGlobal(
    "ResizeObserver",
    class {
      observe() {}
      unobserve() {}
      disconnect() {}
    },
  );
  vi.resetAllMocks();
  api.get.mockResolvedValue(detail);
  api.presets.mockResolvedValue({ items: [], next_cursor: null });
});
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});
function setup(action: "settings" | "archive" | "restore" = "settings") {
  const close = vi.fn(),
    changed = vi.fn();
  render(
    <QueryClientProvider
      client={
        new QueryClient({
          defaultOptions: {
            queries: { retry: false },
            mutations: { retry: false },
          },
        })
      }
    >
      <ProjectManagementDialog
        project={summary}
        action={action}
        onClose={close}
        onChanged={changed}
      />
    </QueryClientProvider>,
  );
  return { close, changed };
}
it("设置读取正式详情，保存只提交实际变更及读取修订", async () => {
  api.update.mockResolvedValue({ ...detail, name: "新名字", revision: 8 });
  const { changed } = setup();
  const name = await screen.findByRole("textbox", { name: "项目名称" });
  expect(
    (screen.getByRole("textbox", { name: "项目描述" }) as HTMLTextAreaElement)
      .value,
  ).toBe("已保存的故事");
  fireEvent.change(name, { target: { value: "新名字" } });
  fireEvent.click(screen.getByRole("button", { name: "保存项目设置" }));
  await waitFor(() => expect(changed).toHaveBeenCalledOnce());
  expect(api.update).toHaveBeenCalledWith(
    summary.id,
    { expected_revision: 7, name: "新名字" },
    expect.any(String),
  );
});
it("未知写入锁住编辑和关闭，重试相同正文与幂等键", async () => {
  api.update
    .mockRejectedValueOnce(new ApiError(0, "dependency_unavailable"))
    .mockResolvedValue({ ...detail, name: "新名字", revision: 8 });
  const { close } = setup();
  const name = await screen.findByRole("textbox", { name: "项目名称" });
  fireEvent.change(name, { target: { value: "新名字" } });
  fireEvent.click(screen.getByRole("button", { name: "保存项目设置" }));
  await screen.findByRole("button", { name: "核验原修改请求" });
  expect((name as HTMLInputElement).disabled).toBe(true);
  expect(
    (screen.getByRole("button", { name: "关闭" }) as HTMLButtonElement)
      .disabled,
  ).toBe(true);
  expect(close).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole("button", { name: "核验原修改请求" }));
  await waitFor(() => expect(api.update).toHaveBeenCalledTimes(2));
  expect(api.update.mock.calls[1]).toEqual(api.update.mock.calls[0]);
});
it("归档遇到正在处理的工作时保留错误，不伪造归档或自动重试", async () => {
  api.transition.mockRejectedValue(new ApiError(409, "inflight_work"));
  const { close, changed } = setup("archive");
  fireEvent.click(screen.getByRole("button", { name: "确认归档" }));
  await screen.findByText(/项目仍有正在处理/);
  expect(api.transition).toHaveBeenCalledExactlyOnceWith(
    summary.id,
    "archive",
    7,
    expect.any(String),
  );
  expect(api.get).not.toHaveBeenCalled();
  expect(changed).not.toHaveBeenCalled();
  expect(close).not.toHaveBeenCalled();
});
