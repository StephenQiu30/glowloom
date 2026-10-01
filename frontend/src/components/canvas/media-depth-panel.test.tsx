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
import { MediaDepthPanel } from "./media-depth-panel";
const api = vi.hoisted(() => ({ list: vi.fn(), get: vi.fn(), run: vi.fn() }));
vi.mock("./media-depth-queries", () => ({
  MEDIA_DEPTHS_KEY: ["canvas", "media-depths"],
  listDepths: api.list,
  getDepth: api.get,
  runDepthIntent: api.run,
  downloadDepth: vi.fn(),
}));
vi.mock("./media-depth-preview-dialog", () => ({
  MediaDepthPreviewDialog: () => null,
}));
const project = "b580ad59-17a5-4ed2-985b-cb0cdd04b4c4",
  canvas = "5db0c65d-d391-45a0-ab1a-d7f6eb4c868d",
  node = "0c6e50ee-0912-4da2-96b3-c84e27b61389",
  id = "fba9de81-80c4-49ce-8f59-1dac39717a04";
const source = { canvas_id: canvas, node_id: node, revision: 4 };
const job = {
  id,
  project_id: project,
  source,
  source_asset_id: "b6f81c20-2489-4276-909d-04a9d13b3e43",
  source_asset_revision: 1,
  source_sha256: "a".repeat(64),
  profile_id: "vda-small-relative-v1" as const,
  status: "failed" as const,
  stage: "failed" as const,
  attempt: 1,
  revision: 8,
  asset_id: null,
  sha256: null,
  failure_code: "synthetic_failure",
  retryable: true,
  needs_reconciliation: false,
  reconciliation_requested: false,
  execution_unconfirmed: false,
  cancellation_requested: false,
  created_at: "2026-10-01T19:00:00Z",
  updated_at: "2026-10-01T19:01:00Z",
};
const page = {
  items: [job],
  next_cursor: null,
  current_actor_id: id,
  current_org_id: project,
};
const clients: QueryClient[] = [];
function show(prepare = vi.fn().mockResolvedValue(source), jobId?: string) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0 } },
  });
  clients.push(client);
  const view = render(
    <QueryClientProvider client={client}>
      <MediaDepthPanel
        projectId={project}
        canvasId={canvas}
        nodeId={node}
        jobId={jobId}
        onPrepareSource={prepare}
      />
    </QueryClientProvider>,
  );
  return { ...view, prepare };
}
beforeEach(() => {
  sessionStorage.clear();
  api.list.mockResolvedValue(page);
  api.get.mockResolvedValue(job);
});
afterEach(() => {
  cleanup();
  clients.splice(0).forEach((c) => c.clear());
  vi.resetAllMocks();
  sessionStorage.clear();
});
it("未知创建刷新后只人工核验原键正文，不重新冻结更高修订或自动发送", async () => {
  api.run.mockRejectedValueOnce(new ApiError(0, "network_error"));
  const first = show();
  fireEvent.click(
    await screen.findByRole("button", { name: "创建视频深度任务" }),
  );
  const recover = await screen.findByRole("button", { name: "核验原请求" });
  expect(recover.hasAttribute("disabled")).toBe(false);
  const intent = api.run.mock.calls[0][0];
  expect(intent.body).toEqual(source);
  first.unmount();
  show(vi.fn().mockResolvedValue({ ...source, revision: 99 }));
  await screen.findByRole("button", { name: "核验原请求" });
  expect(api.run).toHaveBeenCalledTimes(1);
  expect(
    screen
      .getByRole("button", { name: "创建视频深度任务" })
      .hasAttribute("disabled"),
  ).toBe(true);
  api.run.mockResolvedValueOnce(job);
  fireEvent.click(screen.getByRole("button", { name: "核验原请求" }));
  await waitFor(() => expect(api.run).toHaveBeenCalledTimes(2));
  expect(api.run.mock.calls[1][0]).toEqual(intent);
  await waitFor(() => expect(sessionStorage.length).toBe(0));
});
it("存储写入失败不发 DML，恢复窗口可重新检查存储", async () => {
  const spy = vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => {
    throw new Error("quota");
  });
  show();
  fireEvent.click(
    await screen.findByRole("button", { name: "创建视频深度任务" }),
  );
  await screen.findByText(/浏览器存储不可用/);
  expect(api.run).not.toHaveBeenCalled();
  spy.mockRestore();
});
it("确定 409 后必须人工读最新，再确认新意图与新键", async () => {
  let revision = 4;
  const prepare = vi.fn(async () => ({ ...source, revision }));
  api.run.mockRejectedValueOnce(new ApiError(409, "media_depth_conflict"));
  show(prepare);
  fireEvent.click(
    await screen.findByRole("button", { name: "创建视频深度任务" }),
  );
  fireEvent.click(await screen.findByRole("button", { name: "读取最新状态" }));
  await waitFor(() =>
    expect(
      screen
        .getByRole("button", { name: "创建视频深度任务" })
        .hasAttribute("disabled"),
    ).toBe(false),
  );
  revision = 9;
  api.run.mockResolvedValueOnce(job);
  fireEvent.click(screen.getByRole("button", { name: "创建视频深度任务" }));
  await waitFor(() => expect(api.run).toHaveBeenCalledTimes(2));
  expect(api.run.mock.calls[1][0].key).not.toBe(api.run.mock.calls[0][0].key);
  expect(api.run.mock.calls[1][0].body.revision).toBe(9);
});
it("作用域改变时不会在另一账号下核验保留的未知请求", async () => {
  api.run.mockRejectedValueOnce(new ApiError(0, "network_error"));
  show();
  fireEvent.click(
    await screen.findByRole("button", { name: "创建视频深度任务" }),
  );
  await screen.findByRole("button", { name: "核验原请求" });
  api.list.mockResolvedValue({ ...page, current_actor_id: node });
  fireEvent.click(screen.getByRole("button", { name: "核验原请求" }));
  await screen.findByText(/当前工作区身份已变化/);
  expect(api.run).toHaveBeenCalledTimes(1);
  expect(sessionStorage.length).toBe(1);
});
it("未知执行停止只显示事实且禁止重试与核验；取消只记录意图", async () => {
  api.get.mockResolvedValue({
    ...job,
    needs_reconciliation: true,
    retryable: false,
    execution_unconfirmed: true,
  });
  show(undefined, id);
  await screen.findByText(/等待原尝试停止/);
  expect(
    screen.getByRole("button", { name: "重试原任务" }).hasAttribute("disabled"),
  ).toBe(true);
  expect(
    screen.getByRole("button", { name: "核验原结果" }).hasAttribute("disabled"),
  ).toBe(true);
  expect(
    screen.getByRole("button", { name: "请求取消" }).hasAttribute("disabled"),
  ).toBe(false);
});
it("分页错误可重读，按服务端游标请求，不丢失当前详情", async () => {
  api.list
    .mockResolvedValueOnce({ ...page, next_cursor: "next" })
    .mockRejectedValueOnce(new ApiError(503, "dependency_unavailable"));
  show(undefined, id);
  fireEvent.click(
    await screen.findByRole("button", { name: "下一页深度任务" }),
  );
  await screen.findByText("深度任务列表读取失败");
  expect(api.list.mock.calls.some((args) => args[3] === "next")).toBe(true);
  expect(screen.getAllByText(id).length).toBeGreaterThan(0);
});
it("已审核结果回到实际 canvas 路由并保留任务和视频节点身份", async () => {
  api.get.mockResolvedValue({
    ...job,
    status: "succeeded",
    stage: "complete",
    asset_id: canvas,
    sha256: "b".repeat(64),
  });
  show(undefined, id);
  expect(
    (
      await screen.findByRole("link", { name: "到来源画布采纳结果" })
    ).getAttribute("href"),
  ).toBe(
    `/canvas?project=${project}&canvas=${canvas}&node_id=${node}&depth_job=${id}`,
  );
});
it("取消前真实 CAS 已变化时不发送 DML，必须重新读取状态", async () => {
  api.get.mockResolvedValueOnce(job).mockResolvedValue({ ...job, revision: 9 });
  show(undefined, id);
  fireEvent.click(await screen.findByRole("button", { name: "请求取消" }));
  await screen.findByRole("button", { name: "读取最新状态" });
  expect(api.run).not.toHaveBeenCalled();
});
