import { useEffect } from "react";
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
const mocks = vi.hoisted(() => ({
  replace: vi.fn(),
  listProjects: vi.fn(),
  listCanvases: vi.fn(),
  getCanvas: vi.fn(),
  createCanvas: vi.fn(),
  renameCanvas: vi.fn(),
  deleteCanvas: vi.fn(),
  initialDirty: false,
}));
const projectId = "00000000-0000-4000-8000-000000000002",
  canvasId = "00000000-0000-4000-8000-000000000001";
vi.mock("next/navigation", () => ({
  useRouter: () => ({ replace: mocks.replace }),
  useSearchParams: () =>
    new URLSearchParams(`project=${projectId}&canvas=${canvasId}`),
}));
vi.mock("next/dynamic", () => ({
  default: () =>
    function Editor({
      onDirtyChange,
    }: {
      onDirtyChange: (value: boolean) => void;
    }) {
      useEffect(() => {
        onDirtyChange(mocks.initialDirty);
      }, [onDirtyChange]);
      return <div data-testid="controlled-editor" />;
    },
}));
vi.mock("@/components/theme-toggle", () => ({ ThemeToggle: () => null }));
vi.mock("./queries", () => ({
  listProjects: mocks.listProjects,
  listCanvases: mocks.listCanvases,
  getCanvas: mocks.getCanvas,
  createCanvas: mocks.createCanvas,
  renameCanvas: mocks.renameCanvas,
  deleteCanvas: mocks.deleteCanvas,
  saveCanvasCommands: vi.fn(),
}));
import { ApiError } from "@/lib/request";
import { CanvasWorkspace } from "./workspace";
const doc = (revision = 1) => ({
  id: canvasId,
  projectId,
  name: "原名称",
  revision,
  scope: {},
  nodes: [],
  connections: [],
  viewport: { x: 0, y: 0, k: 1 },
});
let client: QueryClient;
beforeEach(() => {
  vi.clearAllMocks();
  mocks.initialDirty = false;
  mocks.listProjects.mockResolvedValue({
    items: [{ id: projectId, name: "合成项目", status: "active", revision: 1 }],
    next_cursor: null,
  });
  mocks.listCanvases.mockResolvedValue({
    items: [{ id: canvasId, name: "原名称" }],
    next_cursor: null,
  });
  mocks.getCanvas.mockResolvedValue(doc());
  mocks.createCanvas.mockResolvedValue({
    ...doc(),
    id: "00000000-0000-4000-8000-000000000003",
  });
});
afterEach(() => {
  cleanup();
  client?.clear();
});
function open() {
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={client}>
      <CanvasWorkspace />
    </QueryClientProvider>,
  );
}
it("画布直接加载项目，没有登录或退出入口", async () => {
  open();
  await screen.findByTestId("controlled-editor");
  expect(screen.queryByRole("button", { name: "退出登录" })).toBeNull();
  expect(mocks.listProjects).toHaveBeenCalled();
  expect(mocks.replace).not.toHaveBeenCalled();
});
it("画布复制入口使用同源完整复制流程，未保存内容时禁用并提示保存", async () => {
  open();
  await screen.findByTestId("controlled-editor");
  expect(
    screen.getByRole("link", { name: "复制完整项目" }).getAttribute("href"),
  ).toBe(`/projects?copy_source=${projectId}`);
  fireEvent.change(screen.getByLabelText("画布名称"), {
    target: { value: "尚未保存" },
  });
  expect(screen.queryByRole("link", { name: "复制完整项目" })).toBeNull();
  expect(
    (screen.getByRole("button", { name: "复制完整项目" }) as HTMLButtonElement)
      .disabled,
  ).toBe(true);
  expect(
    screen.getByText("请先保存当前画布和名称，再复制完整项目。"),
  ).toBeTruthy();
});
it("项目服务失败不显示旧画布，仍提供重试路径", async () => {
  mocks.listProjects.mockRejectedValue(
    new ApiError(503, "dependency_unavailable"),
  );
  open();
  await screen.findByRole("alert");
  expect(screen.queryByTestId("controlled-editor")).toBeNull();
  expect(mocks.getCanvas).not.toHaveBeenCalled();
  expect(mocks.replace).not.toHaveBeenCalled();
});
it("空名称仍是未保存草稿，继续编辑不丢失空值", async () => {
  open();
  await screen.findByTestId("controlled-editor");
  fireEvent.change(screen.getByLabelText("画布名称"), {
    target: { value: "" },
  });
  fireEvent.click(screen.getByRole("link", { name: "Lanverse" }));
  await screen.findByRole("dialog", { name: "放弃未保存修改？" });
  fireEvent.click(screen.getByRole("button", { name: "继续编辑" }));
  expect((screen.getByLabelText("画布名称") as HTMLInputElement).value).toBe(
    "",
  );
});
it("创建请求在途拦截链接离开，收到结果前不可确认放弃", async () => {
  let finish!: (value: ReturnType<typeof doc>) => void;
  mocks.createCanvas.mockImplementationOnce(
    () =>
      new Promise((resolve) => {
        finish = resolve;
      }),
  );
  open();
  await screen.findByTestId("controlled-editor");
  fireEvent.change(screen.getByLabelText("新画布名称"), {
    target: { value: "新画布" },
  });
  fireEvent.click(screen.getByRole("button", { name: "创建画布" }));
  await waitFor(() => expect(mocks.createCanvas).toHaveBeenCalledOnce());
  fireEvent.click(screen.getByRole("link", { name: "Lanverse" }));
  await screen.findByRole("dialog", { name: "放弃未保存修改？" });
  expect(
    (screen.getByRole("button", { name: "放弃并继续" }) as HTMLButtonElement)
      .disabled,
  ).toBe(true);
  finish(doc());
  await waitFor(() => expect(mocks.replace).toHaveBeenCalledOnce());
});
it("创建前一次确认足够，成功导航不再次询问放弃草稿", async () => {
  mocks.initialDirty = true;
  open();
  await screen.findByTestId("controlled-editor");
  fireEvent.change(screen.getByLabelText("新画布名称"), {
    target: { value: "新画布" },
  });
  fireEvent.click(screen.getByRole("button", { name: "创建画布" }));
  await screen.findByRole("dialog", { name: "放弃未保存修改？" });
  fireEvent.click(screen.getByRole("button", { name: "放弃并继续" }));
  await waitFor(() => expect(mocks.replace).toHaveBeenCalledOnce());
  expect(mocks.createCanvas).toHaveBeenCalledOnce();
  expect(screen.queryByRole("dialog", { name: "放弃未保存修改？" })).toBeNull();
});
it("删除409需读取新revision后重新确认，不自动重复删除旧revision", async () => {
  mocks.deleteCanvas.mockRejectedValue(new ApiError(409, "revision_conflict"));
  mocks.getCanvas.mockResolvedValueOnce(doc()).mockResolvedValue(doc(2));
  open();
  await screen.findByTestId("controlled-editor");
  fireEvent.click(screen.getByRole("button", { name: "删除整个画布" }));
  fireEvent.click(await screen.findByRole("button", { name: "确认删除画布" }));
  await screen.findByRole("button", { name: "读取最新并重新确认删除" });
  expect(
    (screen.getByRole("button", { name: "确认删除画布" }) as HTMLButtonElement)
      .disabled,
  ).toBe(true);
  fireEvent.click(
    screen.getByRole("button", { name: "读取最新并重新确认删除" }),
  );
  await waitFor(() =>
    expect(screen.queryByRole("dialog", { name: "删除整个画布？" })).toBeNull(),
  );
  expect(mocks.getCanvas).toHaveBeenCalledTimes(2);
  expect(mocks.deleteCanvas).toHaveBeenCalledTimes(1);
  expect(client.getQueryData(["canvas", "document", canvasId])).toMatchObject({
    revision: 2,
  });
});
it("重命名409读取最新保留用户名称草稿", async () => {
  mocks.renameCanvas.mockRejectedValue(new ApiError(409, "revision_conflict"));
  mocks.getCanvas
    .mockResolvedValueOnce(doc())
    .mockResolvedValue({ ...doc(2), name: "其他页面的名称" });
  open();
  await screen.findByTestId("controlled-editor");
  fireEvent.change(screen.getByLabelText("画布名称"), {
    target: { value: "我的名称草稿" },
  });
  fireEvent.click(screen.getByRole("button", { name: "重命名画布" }));
  fireEvent.click(
    await screen.findByRole("button", { name: "读取最新画布（保留名称草稿）" }),
  );
  await waitFor(() =>
    expect(client.getQueryData(["canvas", "document", canvasId])).toMatchObject(
      { revision: 2 },
    ),
  );
  expect((screen.getByLabelText("画布名称") as HTMLInputElement).value).toBe(
    "我的名称草稿",
  );
  expect(mocks.renameCanvas).toHaveBeenCalledTimes(1);
});
