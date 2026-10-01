import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import type { MediaDepthPanelProps } from "./media-depth-panel";
import { CanvasNodeType } from "./model";
import { DepthWorkspace } from "./depth-workspace";
const api = vi.hoisted(() => ({
  parameters: "",
  replace: vi.fn(),
  projects: vi.fn(),
  project: vi.fn(),
  canvases: vi.fn(),
  canvas: vi.fn(),
  job: vi.fn(),
  jobs: vi.fn(),
  panel: undefined as MediaDepthPanelProps | undefined,
}));
vi.mock("next/navigation", () => ({
  useSearchParams: () => new URLSearchParams(api.parameters),
  useRouter: () => ({ replace: api.replace }),
}));
vi.mock("next/dynamic", () => ({
  default: () =>
    function Panel(props: MediaDepthPanelProps) {
      api.panel = props;
      return (
        <div data-testid="depth-panel">
          <button onClick={() => props.onLockChange?.(true)}>
            合成未知请求锁
          </button>
        </div>
      );
    },
}));
vi.mock("./queries", async (importOriginal) => ({
  ...(await importOriginal<typeof import("./queries")>()),
  listProjects: api.projects,
  listCanvases: api.canvases,
  getCanvas: api.canvas,
}));
vi.mock("@/components/project/queries", () => ({ getProject: api.project }));
vi.mock("./media-depth-queries", () => ({
  MEDIA_DEPTHS_KEY: ["canvas", "media-depths"],
  getDepth: api.job,
  listDepths: api.jobs,
}));
const project = "b580ad59-17a5-4ed2-985b-cb0cdd04b4c4",
  canvas = "5db0c65d-d391-45a0-ab1a-d7f6eb4c868d",
  node = "0c6e50ee-0912-4da2-96b3-c84e27b61389",
  id = "fba9de81-80c4-49ce-8f59-1dac39717a04";
const document = {
  id: canvas,
  projectId: project,
  name: "冻结来源",
  revision: 4,
  scope: {},
  viewport: { x: 0, y: 0, k: 1 },
  nodes: [
    {
      id: node,
      type: CanvasNodeType.Video,
      title: "合成正式视频",
      assetId: id,
      position: { x: 0, y: 0 },
      width: 200,
      height: 120,
      zIndex: 0,
    },
  ],
  connections: [],
};
const clients: QueryClient[] = [];
function show() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0 } },
  });
  clients.push(client);
  return render(
    <QueryClientProvider client={client}>
      <DepthWorkspace />
    </QueryClientProvider>,
  );
}
beforeEach(() => {
  api.parameters = `project_id=${project}&job_id=${id}`;
  api.panel = undefined;
  api.projects.mockResolvedValue({ items: [], next_cursor: null });
  api.project.mockResolvedValue({
    id: project,
    name: "URL恢复项目",
    status: "active",
    revision: 1,
  });
  api.canvases.mockResolvedValue({ items: [], next_cursor: null });
  api.canvas.mockResolvedValue(document);
  api.jobs.mockResolvedValue({
    items: [],
    next_cursor: null,
    current_actor_id: id,
    current_org_id: project,
  });
  api.job.mockResolvedValue({
    id,
    project_id: project,
    source: { canvas_id: canvas, node_id: node, revision: 4 },
  });
});
afterEach(() => {
  cleanup();
  clients.splice(0).forEach((c) => c.clear());
  vi.resetAllMocks();
});
it("只凭 project/job URL 读回冻结来源，不依赖项目或画布当前列表", async () => {
  show();
  await screen.findByTestId("depth-panel");
  expect(api.job).toHaveBeenCalledWith(project, id, expect.any(AbortSignal));
  expect(api.panel).toMatchObject({
    projectId: project,
    canvasId: canvas,
    nodeId: node,
    jobId: id,
  });
  await waitFor(() =>
    expect((screen.getByLabelText("来源画布") as HTMLSelectElement).value).toBe(
      canvas,
    ),
  );
});
it("创建前重新 GET 已保存来源，冻结真实修订；来源媒体变更则拒绝", async () => {
  show();
  await screen.findByTestId("depth-panel");
  await waitFor(() =>
    expect(api.panel?.onPrepareSource).toBeTypeOf("function"),
  );
  api.canvas.mockResolvedValue({ ...document, revision: 8 });
  expect(await api.panel!.onPrepareSource!()).toEqual({
    canvas_id: canvas,
    node_id: node,
    revision: 8,
  });
  api.canvas.mockResolvedValue({
    ...document,
    nodes: [{ ...document.nodes[0], assetId: project }],
  });
  await expect(api.panel!.onPrepareSource!()).rejects.toMatchObject({
    status: 409,
  });
});
it("未知写入锁住页面项目/画布/节点筛选；新选择清除旧任务身份并保留其他参数", async () => {
  api.parameters += `&filter=kept`;
  show();
  await screen.findByTestId("depth-panel");
  fireEvent.change(screen.getByLabelText("正式视频"), {
    target: { value: node },
  });
  expect(api.replace).toHaveBeenCalledWith(
    expect.stringContaining("filter=kept"),
    { scroll: false },
  );
  expect(api.replace.mock.calls[0][0]).not.toContain("job_id=");
  fireEvent.click(screen.getByRole("button", { name: "合成未知请求锁" }));
  expect(screen.getByLabelText("当前项目").hasAttribute("disabled")).toBe(true);
  expect(screen.getByLabelText("来源画布").hasAttribute("disabled")).toBe(true);
  expect(screen.getByLabelText("正式视频").hasAttribute("disabled")).toBe(true);
});
it("跨项目的画布响应不会开启创建；链接身份非法不发 job/source GET", async () => {
  api.parameters = `project_id=bad&job_id=${id}`;
  show();
  await screen.findByText("深度任务链接无效");
  expect(api.job).not.toHaveBeenCalled();
  expect(api.canvas).not.toHaveBeenCalled();
});
