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
import { ProjectFolderDialog } from "./project-folder-dialog";
import { loadFolderIntent, saveFolderIntent } from "./folder-intent";

const ports = vi.hoisted(() => ({
  list: vi.fn(),
  find: vi.fn(),
  project: vi.fn(),
  run: vi.fn(),
}));
vi.mock("./folder-queries", () => ({
  FOLDERS_KEY: ["projects", "folders"],
  listFolders: ports.list,
  findFolder: ports.find,
  findMovableProject: ports.project,
  runFolderIntent: ports.run,
  requireFolderScope: (
    page: { current_actor_id: string; current_org_id: string },
    scope: { actorId: string; orgId: string },
  ) => {
    if (
      page.current_actor_id !== scope.actorId ||
      page.current_org_id !== scope.orgId
    )
      throw new ApiError(409, "scope_changed");
  },
}));
const scope = {
  origin: window.location.origin,
  actorId: "acafc61c-2bf8-48ba-b6c4-f7a84d95c518",
  orgId: "1a39f9d1-a43c-4d98-8947-901b385d5457",
};
const folder = {
  id: "9f3e948c-550b-47c8-8fa2-f2c31c88927c",
  name: "旧名",
  cover: null,
  cover_unavailable: false,
  project_count: 2,
  revision: 3,
  is_delete: false as const,
  delete_time: null,
  create_time: "2026-10-02T00:00:00Z",
  update_time: "2026-10-02T00:00:00Z",
};
const page = {
  current_actor_id: scope.actorId,
  current_org_id: scope.orgId,
  items: [folder],
  next_cursor: null,
};
const close = vi.fn();
const changed = vi.fn();
beforeEach(() => {
  vi.resetAllMocks();
  sessionStorage.clear();
  ports.list.mockResolvedValue(page);
  ports.find.mockResolvedValue(folder);
  ports.run.mockResolvedValue({ folder: { ...folder, revision: 4 } });
});
afterEach(cleanup);
function setup(
  selection: Parameters<typeof ProjectFolderDialog>[0]["selection"],
) {
  return render(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <ProjectFolderDialog
        scope={scope}
        selection={selection}
        onClose={close}
        onChanged={changed}
      />
    </QueryClientProvider>,
  );
}
it("名称草稿遇409保持，人工读取最新修订后新明确意图才发送", async () => {
  ports.run.mockRejectedValueOnce(new ApiError(409, "revision_conflict"));
  setup({ action: "rename", folder });
  await screen.findByText(/当前目录修订：3/);
  fireEvent.change(screen.getByRole("textbox", { name: "目录名称" }), {
    target: { value: "新名称" },
  });
  fireEvent.click(screen.getByRole("button", { name: "保存名称" }));
  await screen.findByText("目录操作未完成");
  expect(
    (screen.getByRole("textbox", { name: "目录名称" }) as HTMLInputElement)
      .value,
  ).toBe("新名称");
  const first = ports.run.mock.calls[0][0];
  ports.find.mockResolvedValue({ ...folder, revision: 8, name: "他人修改" });
  fireEvent.click(
    screen.getByRole("button", { name: "读取最新事实，保留草稿" }),
  );
  await screen.findByText(/当前目录修订：8/);
  fireEvent.click(screen.getByRole("button", { name: "保存名称" }));
  await waitFor(() => expect(ports.run).toHaveBeenCalledTimes(2));
  expect(ports.run.mock.calls[1][0].body).toEqual({
    expected_revision: 8,
    name: "新名称",
  });
  expect(ports.run.mock.calls[1][0].key).not.toBe(first.key);
});
it("未知结果锁定编辑、关闭并持久同键，刷新仅由人工原体核验", async () => {
  ports.run.mockRejectedValueOnce(new ApiError(503, "dependency_unavailable"));
  const mounted = setup({ action: "rename", folder });
  await screen.findByText(/当前目录修订：3/);
  fireEvent.change(screen.getByRole("textbox", { name: "目录名称" }), {
    target: { value: "新名称" },
  });
  fireEvent.click(screen.getByRole("button", { name: "保存名称" }));
  const verify = await screen.findByRole("button", {
    name: "用原键核验目录请求",
  });
  await waitFor(() => expect(document.activeElement).toBe(verify));
  const original = loadFolderIntent(sessionStorage, scope);
  expect(original?.body).toEqual({ expected_revision: 3, name: "新名称" });
  expect(
    screen.getByRole("button", { name: "取消" }).hasAttribute("disabled"),
  ).toBe(true);
  mounted.unmount();
  setup(null);
  await screen.findByText("目录修改结果尚未确认");
  expect(ports.run).toHaveBeenCalledTimes(1);
  fireEvent.click(screen.getByRole("button", { name: "用原键核验目录请求" }));
  await waitFor(() => expect(ports.run).toHaveBeenCalledTimes(2));
  expect(ports.run.mock.calls[1][0]).toEqual(original);
  expect(loadFolderIntent(sessionStorage, scope)).toBeNull();
});
it("存储失败与scope变化不会发出目录DML，恢复操作可获得焦点", async () => {
  const set = vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => {
    throw new Error("denied");
  });
  setup({ action: "create" });
  fireEvent.change(screen.getByRole("textbox", { name: "目录名称" }), {
    target: { value: "新目录" },
  });
  fireEvent.click(screen.getByRole("button", { name: "创建目录" }));
  const recover = await screen.findByRole("button", {
    name: "恢复存储并核验原请求",
  });
  await waitFor(() => expect(document.activeElement).toBe(recover));
  expect(ports.run).not.toHaveBeenCalled();
  set.mockRestore();
  cleanup();
  sessionStorage.clear();
  saveFolderIntent(sessionStorage, {
    ...scope,
    version: 1,
    key: crypto.randomUUID(),
    action: "recycle",
    folderId: folder.id,
    body: { expected_revision: 3 },
  });
  ports.list.mockResolvedValue({ ...page, current_actor_id: folder.id });
  setup(null);
  fireEvent.click(
    await screen.findByRole("button", { name: "用原键核验目录请求" }),
  );
  await screen.findByText(/身份或组织已变化/);
  expect(ports.run).not.toHaveBeenCalled();
  expect(loadFolderIntent(sessionStorage, scope)).not.toBeNull();
});
it("回收明确包含全部成员，服务端inflight整体拒绝不会显示成功", async () => {
  ports.run.mockRejectedValue(new ApiError(409, "inflight_work"));
  setup({ action: "recycle", folder });
  await screen.findByText(/当前目录修订：3/);
  expect(screen.getByText(/包含当前全部 2 个项目/)).toBeTruthy();
  fireEvent.click(
    screen.getByRole("checkbox", { name: /确认回收目录和其中全部项目/ }),
  );
  fireEvent.click(
    screen.getByRole("button", { name: "确认回收目录及所有项目" }),
  );
  await screen.findByText("目录操作未完成");
  expect(changed).not.toHaveBeenCalled();
  expect(close).not.toHaveBeenCalled();
});

it("非法写入DTO按未知效果保留创建原体，手动同键核验不自动创建第二个目录", async () => {
  ports.run.mockRejectedValueOnce(new ApiError(502, "invalid_response"));
  setup({ action: "create" });
  fireEvent.change(screen.getByRole("textbox", { name: "目录名称" }), {
    target: { value: "新目录😀" },
  });
  fireEvent.click(screen.getByRole("button", { name: "创建目录" }));
  const recover = await screen.findByRole("button", {
    name: "用原键核验目录请求",
  });
  const original = loadFolderIntent(sessionStorage, scope);
  expect(original?.body).toEqual({ name: "新目录😀" });
  expect(ports.run).toHaveBeenCalledTimes(1);
  expect(changed).not.toHaveBeenCalled();
  fireEvent.click(recover);
  await waitFor(() => expect(changed).toHaveBeenCalledOnce());
  expect(ports.run.mock.calls[1][0]).toEqual(original);
});

it("项目详情之后写入前重新确认scope，变化时冻结原3项CAS且不发送移动", async () => {
  const project = {
    id: "322f64a1-6ce2-40aa-bf94-6f1f85b8bba4",
    name: "待移动项目",
    cover_asset_id: null,
    cover_unavailable: false,
    status: "active" as const,
    revision: 8,
    folder_id: folder.id,
    placement_revision: 4,
    aspect_ratio: "16:9" as const,
    style_type: "realistic" as const,
    is_delete: false,
  };
  ports.project.mockResolvedValue(project);
  setup({ action: "move", project });
  await screen.findByText(/项目修订 8 · 放置修订 4/);
  await screen.findByRole("combobox", { name: "目标目录" });
  ports.list.mockResolvedValue({ ...page, current_org_id: folder.id });
  fireEvent.click(screen.getByRole("button", { name: "确认移动项目" }));
  await screen.findByText(/身份或组织已变化/);
  expect(ports.run).not.toHaveBeenCalled();
  const original = loadFolderIntent(sessionStorage, scope);
  expect(original?.body).toEqual({
    expected_project_revision: 8,
    expected_placement_revision: 4,
    folder_id: null,
    expected_folder_revision: 0,
  });
  ports.list.mockResolvedValue(page);
  fireEvent.click(screen.getByRole("button", { name: "用原键核验目录请求" }));
  await waitFor(() =>
    expect(ports.run).toHaveBeenCalledExactlyOnceWith(original),
  );
});
