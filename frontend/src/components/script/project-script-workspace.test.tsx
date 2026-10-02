import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { ProjectScriptWorkspace } from "./project-script-workspace";
import * as source from "./source-queries";
import { getProject } from "@/components/project/queries";
import { listFileImports } from "./file-import-queries";
import { saveFileImportIntent } from "./file-import-intent";
import { ApiError } from "@/lib/request";
const nav = vi.hoisted(() => ({ params: "", replace: vi.fn() }));
vi.mock("next/navigation", () => ({
  useSearchParams: () => new URLSearchParams(nav.params),
  usePathname: () => "/projects/33333333-3333-4333-8333-333333333333/script",
  useRouter: () => ({ replace: nav.replace }),
}));
vi.mock("./script-history-dialog", () => ({
  ScriptHistoryDialog: ({
    onChooseVersion,
  }: {
    onChooseVersion: (id: string) => void;
  }) => (
    <div role="dialog" aria-label="测试版本选择">
      <button
        onClick={() => onChooseVersion("66666666-6666-4666-8666-666666666666")}
      >
        选择历史完整版本
      </button>
    </div>
  ),
}));
vi.mock("@/components/project/queries", () => ({ getProject: vi.fn() }));
vi.mock("./file-import-queries", async (original) => ({
  ...(await original<typeof import("./file-import-queries")>()),
  listFileImports: vi.fn(),
  getFileImport: vi.fn(),
  runFileImportIntent: vi.fn(),
}));
vi.mock("./document-library", () => ({
  DocumentLibrary: () => (
    <section aria-label="项目文档原件">正式文档原件入口</section>
  ),
}));
vi.mock("./source-queries", async (original) => ({
  ...(await original<typeof import("./source-queries")>()),
  getWorkspace: vi.fn(),
  listSources: vi.fn(),
  getSource: vi.fn(),
  readAllSources: vi.fn(),
  runSourceIntent: vi.fn(),
}));
vi.mock("./source-edit-form", () => ({
  SourceEditForm: ({
    locked,
    onDirty,
    onSubmit,
    base,
  }: {
    locked: boolean;
    onDirty: (value: boolean) => void;
    onSubmit: (body: unknown) => void;
    base: unknown;
  }) => (
    <>
      <button disabled={locked} onClick={() => onDirty(true)}>
        测试草稿变化
      </button>
      <button
        disabled={locked}
        onClick={() =>
          onSubmit({
            ...(base as object),
            rights_confirmed: true,
            source_kind: "chapter",
            title: "章",
            status: "draft",
            document: { type: "doc" },
            provenance: {},
          })
        }
      >
        测试保存来源
      </button>
    </>
  ),
}));
const pid = "33333333-3333-4333-8333-333333333333";
const version = "44444444-4444-4444-8444-444444444444";
const lineage = "55555555-5555-4555-8555-555555555555";
const summary = {
  id: lineage,
  source_lineage_id: lineage,
  source_revision: 1,
  source_kind: "chapter" as const,
  title: "初章",
  status: "draft" as const,
  origin: "manual" as const,
  position: 0,
  char_count: 0,
  content_hash: "a".repeat(64),
  rich_sha256: "b".repeat(64),
};
const workspace = {
  current_actor_id: "11111111-1111-4111-8111-111111111111",
  current_org_id: "22222222-2222-4222-8222-222222222222",
  state: {
    project_id: pid,
    org_id: "22222222-2222-4222-8222-222222222222",
    revision: 1,
    draft_version_id: version,
    updated_at: "2026-10-02T00:00:00Z",
  },
};
beforeEach(() => {
  vi.resetAllMocks();
  nav.params = "";
  sessionStorage.clear();
  vi.mocked(getProject).mockResolvedValue({
    id: pid,
    name: "独占项目",
    status: "active",
    is_delete: false,
  } as Awaited<ReturnType<typeof getProject>>);
  vi.mocked(source.getWorkspace).mockResolvedValue(workspace);
  vi.mocked(listFileImports).mockResolvedValue({
    current_actor_id: workspace.current_actor_id,
    current_org_id: workspace.current_org_id,
    items: [],
  });
  vi.mocked(source.listSources).mockResolvedValue({
    version_id: version,
    items: [summary],
  });
  vi.mocked(source.getSource).mockResolvedValue({
    ...summary,
    document: { type: "doc" },
    plain_text: "",
    source_span: {
      source_id: lineage,
      source_lineage_id: lineage,
      position: 0,
      start: 0,
      end: 0,
    },
    provenance: {},
  });
});
it("历史选择经过脏稿确认，URL保留原Copy定位与其他筛选参数", async () => {
  nav.params = `copy_source=${pid}&copy_job=${version}&query=abc`;
  mount();
  const dirty = await screen.findByRole("button", { name: "测试草稿变化" });
  await waitFor(() => expect(dirty.matches(":disabled")).toBe(false));
  fireEvent.click(dirty);
  fireEvent.click(screen.getByRole("button", { name: "完整版本与来源历史" }));
  fireEvent.click(
    await screen.findByRole("button", { name: "选择历史完整版本" }),
  );
  expect(nav.replace).not.toHaveBeenCalled();
  fireEvent.click(
    await screen.findByRole("button", { name: "明确放弃草稿并继续" }),
  );
  expect(nav.replace).toHaveBeenCalledWith(
    `/projects/${pid}/script?copy_source=${pid}&copy_job=${version}&query=abc&script_version=66666666-6666-4666-8666-666666666666`,
    { scroll: false },
  );
});
afterEach(cleanup);
function mount(
  projectId = pid,
  client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  }),
) {
  return render(
    <QueryClientProvider client={client}>
      <ProjectScriptWorkspace projectId={projectId} />
    </QueryClientProvider>,
  );
}
function primedPrivateCache() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  const scope = {
    origin: window.location.origin,
    actorId: workspace.current_actor_id,
    orgId: workspace.current_org_id,
    projectId: pid,
  };
  client.setQueryData(source.scriptWorkspaceKey(pid), workspace);
  client.setQueryData([...source.scriptScopeKey(scope), "sources", version], {
    pages: [{ version_id: version, items: [summary] }],
    pageParams: [0],
  });
  client.setQueryData(
    [...source.scriptScopeKey(scope), "source", version, lineage],
    { ...summary, title: "旧主体私有正文", document: { type: "doc" } },
  );
  return client;
}
it("已有私有缓存仍先等挂载后的工作区证明，换主体只读取新主体作用域", async () => {
  let resolveWorkspace!: (value: typeof workspace) => void;
  const freshWorkspace = new Promise<typeof workspace>((resolve) => {
    resolveWorkspace = resolve;
  });
  vi.mocked(source.getWorkspace).mockReturnValue(freshWorkspace);
  mount(pid, primedPrivateCache());
  await screen.findByRole("heading", { name: "独占项目 · 剧本" });
  expect(source.getWorkspace).toHaveBeenCalledOnce();
  expect(screen.queryByRole("region", { name: "选中来源正文" })).toBeNull();
  expect(screen.queryByText("旧主体私有正文")).toBeNull();
  expect(source.listSources).not.toHaveBeenCalled();
  expect(source.getSource).not.toHaveBeenCalled();
  const nextActor = "77777777-7777-4777-8777-777777777777";
  vi.mocked(listFileImports).mockResolvedValue({
    current_actor_id: nextActor,
    current_org_id: workspace.current_org_id,
    items: [],
  });
  vi.mocked(source.getWorkspace).mockResolvedValue({
    ...workspace,
    current_actor_id: nextActor,
  });
  await act(async () =>
    resolveWorkspace({
      ...workspace,
      current_actor_id: nextActor,
    }),
  );
  await screen.findByRole("button", { name: "测试保存来源" });
  expect(screen.queryByText("旧主体私有正文")).toBeNull();
  expect(source.getSource).toHaveBeenCalledWith(
    expect.objectContaining({ actorId: nextActor }),
    lineage,
    version,
    expect.any(AbortSignal),
  );
  expect(
    vi
      .mocked(source.getSource)
      .mock.calls.every(([scope]) => scope.actorId === nextActor),
  ).toBe(true);
});
it.each([403, 404])(
  "首次工作区%d拒绝不把残留缓存当证明，人工重读成功才挂载正文",
  async (status) => {
    vi.mocked(source.getWorkspace).mockRejectedValue(
      new ApiError(status, "scope_denied"),
    );
    mount(pid, primedPrivateCache());
    await screen.findByRole("button", { name: "重试读取剧本工作区" });
    expect(screen.queryByText("旧主体私有正文")).toBeNull();
    expect(source.listSources).not.toHaveBeenCalled();
    expect(source.getSource).not.toHaveBeenCalled();
    vi.mocked(source.getWorkspace).mockResolvedValue(workspace);
    fireEvent.click(screen.getByRole("button", { name: "重试读取剧本工作区" }));
    await screen.findByRole("button", { name: "测试保存来源" });
    await waitFor(() => expect(source.getSource).toHaveBeenCalled());
  },
);
it("已证明主体的普通头刷新保留草稿，刷新拒绝后隐藏私有子树", async () => {
  const client = primedPrivateCache();
  mount(pid, client);
  const dirty = await screen.findByRole("button", { name: "测试草稿变化" });
  await waitFor(() => expect(dirty.matches(":disabled")).toBe(false));
  fireEvent.click(dirty);
  let rejectWorkspace!: (error: Error) => void;
  vi.mocked(source.getWorkspace).mockImplementation(
    () =>
      new Promise((_, reject) => {
        rejectWorkspace = reject;
      }),
  );
  let refreshing!: Promise<void>;
  await act(async () => {
    refreshing = client.invalidateQueries({
      queryKey: source.scriptWorkspaceKey(pid),
    });
  });
  expect(screen.getByRole("button", { name: "测试保存来源" })).toBeTruthy();
  expect(screen.queryByText("正在读取项目与剧本工作区…")).toBeNull();
  await act(async () => {
    rejectWorkspace(new ApiError(403, "scope_denied"));
    await refreshing;
  });
  expect(
    await screen.findByRole("button", { name: "重试读取剧本工作区" }),
  ).toBeTruthy();
  expect(screen.queryByRole("button", { name: "测试保存来源" })).toBeNull();
});
it("合法大写UUID地址恢复为同一项目与历史来源作用域，不产生分裂缓存或意图", async () => {
  const canonicalProject = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa";
  const canonicalVersion = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb";
  const canonicalLineage = "cccccccc-cccc-4ccc-8ccc-cccccccccccc";
  nav.params = `script_version=${canonicalVersion.toUpperCase()}&script_source=${canonicalLineage.toUpperCase()}`;
  vi.mocked(source.getWorkspace).mockResolvedValue({
    ...workspace,
    state: { ...workspace.state, project_id: canonicalProject },
  });
  vi.mocked(source.listSources).mockResolvedValue({
    version_id: canonicalVersion,
    items: [],
  });
  mount(canonicalProject.toUpperCase());
  await waitFor(() =>
    expect(source.getSource).toHaveBeenCalledWith(
      expect.objectContaining({ projectId: canonicalProject }),
      canonicalLineage,
      canonicalVersion,
      expect.any(AbortSignal),
    ),
  );
  expect(getProject).toHaveBeenCalledWith(
    canonicalProject,
    expect.any(AbortSignal),
  );
  expect(source.getWorkspace).toHaveBeenCalledWith(
    canonicalProject,
    expect.any(AbortSignal),
  );
  expect(source.runSourceIntent).not.toHaveBeenCalled();
});
it("项目与工作区独立取数、选中正文懒读；本地草稿阻止直接切换或刷新覆盖", async () => {
  mount();
  expect(
    await screen.findByRole("heading", { name: "独占项目 · 剧本" }),
  ).toBeTruthy();
  expect(
    await screen.findByRole("button", { name: "测试草稿变化" }),
  ).toBeTruthy();
  await waitFor(() => expect(source.getSource).toHaveBeenCalled());
  fireEvent.click(screen.getByRole("button", { name: "测试草稿变化" }));
  fireEvent.click(screen.getByRole("button", { name: "新建来源" }));
  expect(
    await screen.findByRole("dialog", { name: "保留当前来源草稿" }),
  ).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "继续编辑当前草稿" }));
  expect(screen.getByRole("button", { name: "测试保存来源" })).toBeTruthy();
});
it("unknown锁住写入并保留原键核验入口，刷新事实不是自动重新提交", async () => {
  vi.mocked(source.runSourceIntent).mockRejectedValue(
    new Error("response lost"),
  );
  mount();
  const save = await screen.findByRole("button", { name: "测试保存来源" });
  await waitFor(() => expect(save.hasAttribute("disabled")).toBe(false));
  fireEvent.click(save);
  expect(
    await screen.findByRole("button", { name: "人工使用原键核验保存" }),
  ).toBeTruthy();
  expect(save.hasAttribute("disabled")).toBe(true);
  expect(source.runSourceIntent).toHaveBeenCalledTimes(1);
});
it("文件导入unknown刷新自动恢复在模态框原键核验，锁住普通来源写入", async () => {
  saveFileImportIntent(sessionStorage, {
    version: 1,
    key: lineage,
    origin: window.location.origin,
    projectId: pid,
    actorId: workspace.current_actor_id,
    orgId: workspace.current_org_id,
    action: "create",
    body: {
      expected_revision: 1,
      base_version_id: version,
      rights_confirmed: true,
      asset_ids: [lineage],
    },
  });
  mount();
  expect(
    await screen.findByRole("dialog", { name: "文件导入任务" }),
  ).toBeTruthy();
  expect(
    screen.getByRole("button", { name: "人工使用原键核验导入请求" }),
  ).toBeTruthy();
  expect(
    screen
      .getByRole("button", { name: "关闭文件导入任务" })
      .matches(":disabled"),
  ).toBe(true);
  expect(source.runSourceIntent).not.toHaveBeenCalled();
});
