import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { ApiError } from "@/lib/request";
import { saveCopyIntent } from "./copy-intent";
import { ProjectCopyDialog } from "./project-copy-dialog";

const ports = vi.hoisted(() => ({
  source: vi.fn(),
  list: vi.fn(),
  get: vi.fn(),
  run: vi.fn(),
  close: vi.fn(),
  select: vi.fn(),
}));
vi.mock("./queries", () => ({
  PROJECTS_KEY: ["projects"],
  getProject: ports.source,
}));
vi.mock("./copy-queries", async (original) => ({
  ...(await original<typeof import("./copy-queries")>()),
  listCopies: ports.list,
  getCopy: ports.get,
  runCopyIntent: ports.run,
}));
const sourceId = "e61dbdb9-d428-45dc-a034-53f3f1bb1068";
const scope = {
  origin: window.location.origin,
  actorId: "b51e05a5-3e86-4685-a624-bf28841d5618",
  orgId: "9c06e9e9-c029-4404-b6c0-68509d031341",
  sourceId,
};
const job = {
  id: "93021a72-3281-42b2-91cf-6a4413ee8956",
  source_project_id: sourceId,
  target_project_id: "c5d5711d-ab9a-4347-be88-933b51e399ae",
  target_name: "完整副本",
  source_revision: 7,
  status: "queued" as const,
  stage: "media" as const,
  revision: 1,
  attempt: 1,
  documents: 2,
  assets: 4,
  renditions: 8,
  completed_documents: 0,
  completed_assets: 0,
  completed_renditions: 0,
  retryable: false,
  needs_reconciliation: false,
  reconciliation_requested: false,
  execution_unconfirmed: false,
  cancellation_requested: false,
};
const page = {
  copies: [job],
  next_cursor: null,
  current_actor_id: scope.actorId,
  current_org_id: scope.orgId,
};
let cache: QueryClient;
beforeEach(() => {
  vi.resetAllMocks();
  sessionStorage.clear();
  ports.source.mockResolvedValue({ id: sourceId, name: "源工程", revision: 7 });
  ports.list.mockResolvedValue(page);
  ports.get.mockResolvedValue(job);
  ports.run.mockResolvedValue(job);
  cache = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
});
afterEach(() => {
  cleanup();
  cache.clear();
  vi.restoreAllMocks();
});
function open(jobId?: string) {
  return render(
    <QueryClientProvider client={cache}>
      <ProjectCopyDialog
        sourceId={sourceId}
        jobId={jobId}
        onClose={ports.close}
        onJobSelected={ports.select}
      />
    </QueryClientProvider>,
  );
}
it("源项目与作用域读取并行，未成功目标隐藏，只展示真实完成数量", async () => {
  open(job.id);
  await screen.findByRole("heading", { name: "复制任务详情" });
  expect(ports.source).toHaveBeenCalled();
  expect(ports.list).toHaveBeenCalled();
  expect(
    await screen.findByText("画布 0 / 2 · 素材 0 / 4 · 衍生物 0 / 8"),
  ).toBeTruthy();
  expect(screen.queryByRole("link", { name: "打开副本画布" })).toBeNull();
  expect(screen.queryByRole("progressbar")).toBeNull();
});
it("剧本阶段列出全部历史事实，只有正式成功后才开放副本剧本", async () => {
  const counts = {
    sources: 4,
    versions: 3,
    version_sources: 7,
    project_states: 1,
    version_heads: 3,
    split_sets: 5,
    split_confirmations: 2,
    episodes: 6,
    structures: 8,
    scenes: 9,
    dialogue_lines: 11,
    action_lines: 12,
    objects: 15,
  };
  ports.get.mockResolvedValueOnce({
    ...job,
    status: "running",
    stage: "script",
    script: { counts },
  });
  open(job.id);
  await screen.findByText("正在复制 · 复制剧本与全部历史");
  expect(screen.getByText("对白")).toBeTruthy();
  expect(screen.getByText("动作")).toBeTruthy();
  expect(screen.getByText("0 / 11")).toBeTruthy();
  expect(screen.getByText("0 / 12")).toBeTruthy();
  expect(screen.getByText("0 / 15")).toBeTruthy();
  expect(screen.queryByRole("link", { name: "打开副本剧本" })).toBeNull();
  ports.get.mockResolvedValue({
    ...job,
    revision: 5,
    status: "succeeded",
    stage: "complete",
    completed_documents: job.documents,
    completed_assets: job.assets,
    completed_renditions: job.renditions,
    script: { counts, completed_counts: counts },
  });
  fireEvent.click(screen.getByRole("button", { name: "刷新任务状态" }));
  expect(
    (await screen.findByRole("link", { name: "打开副本剧本" })).getAttribute(
      "href",
    ),
  ).toBe(`/projects/${job.target_project_id}/script`);
  expect(screen.getByText("11 / 11")).toBeTruthy();
  expect(screen.getByText("12 / 12")).toBeTruthy();
  expect(screen.getByText("15 / 15")).toBeTruthy();
});
it("本次安全读取完成前不复用其他页面或主体的来源详情缓存", async () => {
  cache.setQueryData(["projects", "detail", sourceId], {
    id: sourceId,
    name: "其他主体的旧来源缓存",
    revision: 7,
  });
  let resolveScope!: (value: typeof page) => void;
  ports.list.mockReturnValueOnce(
    new Promise<typeof page>((resolve) => {
      resolveScope = resolve;
    }),
  );
  ports.source.mockReturnValue(new Promise(() => {}));
  open();
  await act(async () => {
    resolveScope(page);
    await new Promise((resolve) => setTimeout(resolve, 10));
  });
  expect(screen.queryByLabelText("副本名称")).toBeNull();
  expect(screen.queryByText(/其他主体的旧来源缓存/)).toBeNull();
  expect(screen.getByText("正在读取来源与复制任务…")).toBeTruthy();
});
it("202 不覆盖较新缓存；只有 GET 成功才开放副本，且不自动导航", async () => {
  const succeeded = {
    ...job,
    revision: 8,
    status: "succeeded" as const,
    stage: "complete" as const,
    completed_documents: 2,
    completed_assets: 4,
    completed_renditions: 8,
  };
  ports.get.mockResolvedValue(succeeded);
  open(job.id);
  await screen.findByRole("link", { name: "打开副本画布" });
  fireEvent.change(screen.getByLabelText("副本名称"), {
    target: { value: "完整副本" },
  });
  fireEvent.click(screen.getByRole("button", { name: "创建完整副本" }));
  await waitFor(() => expect(ports.run).toHaveBeenCalledOnce());
  expect(
    screen.getByRole("link", { name: "打开副本画布" }).getAttribute("href"),
  ).toBe(`/projects/${job.target_project_id}/canvas`);
  expect(ports.close).not.toHaveBeenCalled();
});
it("未知创建锁字段、新提交与关闭，刷新恢复同一原 key/body，须人工核验", async () => {
  ports.run.mockRejectedValueOnce(new ApiError(0, "dependency_unavailable"));
  const mounted = open();
  await screen.findByLabelText("副本名称");
  await waitFor(() =>
    expect(
      (
        screen.getByRole("button", {
          name: "创建完整副本",
        }) as HTMLButtonElement
      ).disabled,
    ).toBe(false),
  );
  fireEvent.change(screen.getByLabelText("副本名称"), {
    target: { value: "保留的正文" },
  });
  fireEvent.click(screen.getByRole("button", { name: "创建完整副本" }));
  await screen.findByRole("button", { name: "使用原请求核验结果" });
  const original = ports.run.mock.calls[0][0];
  expect((screen.getByLabelText("副本名称") as HTMLInputElement).disabled).toBe(
    true,
  );
  fireEvent.click(screen.getByRole("button", { name: "关闭复制任务" }));
  expect(ports.close).not.toHaveBeenCalled();
  const leaving = new Event("beforeunload", { cancelable: true });
  window.dispatchEvent(leaving);
  expect(leaving.defaultPrevented).toBe(true);
  mounted.unmount();
  open();
  await screen.findByRole("button", { name: "使用原请求核验结果" });
  expect(ports.run).toHaveBeenCalledTimes(1);
  expect((screen.getByLabelText("副本名称") as HTMLInputElement).value).toBe(
    "保留的正文",
  );
  fireEvent.click(screen.getByRole("button", { name: "使用原请求核验结果" }));
  await waitFor(() => expect(ports.run).toHaveBeenCalledTimes(2));
  expect(ports.run.mock.calls[1][0]).toEqual(original);
});
it("fresh GET 当前主体变化不重放旧作用域请求", async () => {
  saveCopyIntent(sessionStorage, {
    ...scope,
    version: 1,
    key: "511054be-4f0d-4a9a-b9ad-2a6af5a3979f",
    action: "create",
    body: { expected_revision: 7, target_name: "完整副本" },
  });
  open();
  await screen.findByRole("button", { name: "使用原请求核验结果" });
  ports.list.mockResolvedValueOnce({ ...page, current_actor_id: job.id });
  fireEvent.click(screen.getByRole("button", { name: "使用原请求核验结果" }));
  await screen.findByText(/当前工作区身份已变化/);
  expect(ports.run).not.toHaveBeenCalled();
  expect(sessionStorage.length).toBe(1);
});
it("storage 写入失败绝不发送 DML", async () => {
  open();
  await screen.findByLabelText("副本名称");
  await waitFor(() =>
    expect(
      (
        screen.getByRole("button", {
          name: "创建完整副本",
        }) as HTMLButtonElement
      ).disabled,
    ).toBe(false),
  );
  vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => {
    throw new Error("quota");
  });
  fireEvent.click(screen.getByRole("button", { name: "创建完整副本" }));
  await screen.findByText(/浏览器存储不可用/);
  expect(ports.run).not.toHaveBeenCalled();
});
it("确定 409 需人工读取最新后另立意图，不自动刷新 CAS 发送", async () => {
  ports.run.mockRejectedValueOnce(new ApiError(409, "revision_conflict"));
  open();
  await screen.findByLabelText("副本名称");
  await waitFor(() =>
    expect(
      (
        screen.getByRole("button", {
          name: "创建完整副本",
        }) as HTMLButtonElement
      ).disabled,
    ).toBe(false),
  );
  fireEvent.click(screen.getByRole("button", { name: "创建完整副本" }));
  await screen.findByRole("button", { name: "读取最新后重新确认" });
  expect(
    (screen.getByRole("button", { name: "创建完整副本" }) as HTMLButtonElement)
      .disabled,
  ).toBe(true);
  ports.source.mockResolvedValue({ id: sourceId, name: "源工程", revision: 8 });
  fireEvent.click(screen.getByRole("button", { name: "读取最新后重新确认" }));
  await waitFor(() =>
    expect(
      (
        screen.getByRole("button", {
          name: "创建完整副本",
        }) as HTMLButtonElement
      ).disabled,
    ).toBe(false),
  );
  expect(ports.run).toHaveBeenCalledTimes(1);
  fireEvent.click(screen.getByRole("button", { name: "创建完整副本" }));
  await waitFor(() => expect(ports.run).toHaveBeenCalledTimes(2));
  expect(ports.run.mock.calls[1][0].body.expected_revision).toBe(8);
  expect(ports.run.mock.calls[1][0].key).not.toBe(
    ports.run.mock.calls[0][0].key,
  );
});
it("执行未退出时仅可记录取消意图，禁用核验与重试，不宣称已停止", async () => {
  ports.get.mockResolvedValue({
    ...job,
    status: "failed",
    retryable: true,
    needs_reconciliation: true,
    execution_unconfirmed: true,
  });
  open(job.id);
  await screen.findByText(/原尝试尚未确认退出/);
  expect(
    (screen.getByRole("button", { name: "核验旧结果" }) as HTMLButtonElement)
      .disabled,
  ).toBe(true);
  expect(screen.queryByRole("button", { name: "重试原任务" })).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "请求取消复制" }));
  await waitFor(() => expect(ports.run).toHaveBeenCalledOnce());
  expect(ports.run.mock.calls[0][0]).toMatchObject({
    action: "cancel",
    jobId: job.id,
    body: { expected_revision: 1 },
  });
  expect(screen.queryByText("复制已停止")).toBeNull();
});
it("真实 cursor 分页和错误独立可重读，不编造任务", async () => {
  ports.list.mockResolvedValue({ ...page, next_cursor: "next" });
  open();
  fireEvent.click(
    await screen.findByRole("button", { name: "下一页复制任务" }),
  );
  await waitFor(() =>
    expect(ports.list.mock.calls.some(([, cursor]) => cursor === "next")).toBe(
      true,
    ),
  );
});

it("写入非法 DTO 保留未知意图，不能以新 key 自动重试", async () => {
  ports.run.mockRejectedValueOnce(new ApiError(502, "invalid_response"));
  open();
  await screen.findByRole("button", { name: "创建完整副本" });
  fireEvent.click(screen.getByRole("button", { name: "创建完整副本" }));
  await screen.findByRole("button", { name: "使用原请求核验结果" });
  expect(ports.run).toHaveBeenCalledTimes(1);
  expect(sessionStorage.length).toBe(1);
  expect((screen.getByLabelText("副本名称") as HTMLInputElement).disabled).toBe(
    true,
  );
});
it("取消与完成竞态通过最新 GET 得到 succeeded，来源和正式副本不撤销", async () => {
  open(job.id);
  fireEvent.click(await screen.findByRole("button", { name: "请求取消复制" }));
  await waitFor(() => expect(ports.run).toHaveBeenCalledOnce());
  ports.get.mockResolvedValue({
    ...job,
    status: "succeeded",
    stage: "complete",
    revision: 9,
    completed_documents: 2,
    completed_assets: 4,
    completed_renditions: 8,
  });
  fireEvent.click(screen.getByRole("button", { name: "刷新任务状态" }));
  await screen.findByRole("link", { name: "打开副本画布" });
  expect(screen.queryByRole("button", { name: "请求取消复制" })).toBeNull();
  expect(ports.run).toHaveBeenCalledTimes(1);
});
it("失败取消意图需要核验后清理，不能作为普通重试", async () => {
  ports.get.mockResolvedValue({
    ...job,
    status: "failed",
    retryable: true,
    needs_reconciliation: true,
    cancellation_requested: true,
  });
  open(job.id);
  await screen.findByText(/待核验原结果并清理副本/);
  expect(screen.queryByRole("button", { name: "重试原任务" })).toBeNull();
  expect(screen.queryByRole("button", { name: "请求取消复制" })).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "核验旧结果" }));
  await waitFor(() => expect(ports.run).toHaveBeenCalledOnce());
  expect(ports.run.mock.calls[0][0].action).toBe("reconcile");
});
