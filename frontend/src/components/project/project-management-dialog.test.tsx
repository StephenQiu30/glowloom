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
import { ProjectManagementDialog } from "./project-management-dialog";
import {
  saveProjectCoverIntent,
  loadProjectCoverIntent,
} from "./project-cover-intent";
import type { FolderScope } from "./folder-intent";
import { useState } from "react";

const api = vi.hoisted(() => ({
  get: vi.fn(),
  update: vi.fn(),
  transition: vi.fn(),
  presets: vi.fn(),
  scope: vi.fn(),
}));
vi.mock("./folder-queries", async (original) => ({
  ...(await original<typeof import("./folder-queries")>()),
  listFolders: api.scope,
}));
vi.mock("./project-cover-preview", () => ({
  ProjectCoverPreview: ({ assetId }: { assetId: string }) => (
    <span>主图编号 {assetId}</span>
  ),
}));
vi.mock("./project-cover-picker", () => ({
  ProjectCoverPicker: ({
    onSelected,
    onClose,
  }: {
    onSelected: (id: string) => void;
    onClose: () => void;
  }) => (
    <div role="dialog" aria-label="选择项目主图">
      <button
        onClick={() => onSelected("c13b18f1-cd43-4f35-bef4-f06801746458")}
      >
        选择正式图片
      </button>
      <button onClick={onClose}>取消选择</button>
    </div>
  ),
}));
const scope = {
  origin: window.location.origin,
  actorId: "d3a6a735-cbdd-4f7f-8316-fc6e08e93c34",
  orgId: "de0d143b-27bc-4f1c-90a8-edc099be4b2d",
};
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
  cover_asset_id: null,
  cover_unavailable: false,
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
  sessionStorage.clear();
  api.scope.mockResolvedValue({
    current_actor_id: scope.actorId,
    current_org_id: scope.orgId,
    items: [],
    next_cursor: null,
  });
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
  const view = render(
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
        scope={scope}
        onClose={close}
        onChanged={changed}
      />
    </QueryClientProvider>,
  );
  return { close, changed, unmount: view.unmount };
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
it("选图只改草稿，保存联合名称和主图；明确清除发送 null", async () => {
  api.update.mockResolvedValue({ ...detail, revision: 8 });
  setup();
  fireEvent.change(await screen.findByRole("textbox", { name: "项目名称" }), {
    target: { value: "有主图的作品" },
  });
  fireEvent.click(screen.getByRole("button", { name: "选择项目图片" }));
  fireEvent.click(await screen.findByRole("button", { name: "选择正式图片" }));
  expect(api.update).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole("button", { name: "保存项目设置" }));
  await waitFor(() =>
    expect(api.update).toHaveBeenCalledWith(
      summary.id,
      {
        expected_revision: 7,
        name: "有主图的作品",
        cover_asset_id: "c13b18f1-cd43-4f35-bef4-f06801746458",
      },
      expect.any(String),
    ),
  );
  cleanup();
  api.get.mockResolvedValue({
    ...detail,
    cover_asset_id: "c13b18f1-cd43-4f35-bef4-f06801746458",
  });
  setup();
  fireEvent.click(await screen.findByRole("button", { name: "清除主图" }));
  fireEvent.click(screen.getByRole("button", { name: "保存项目设置" }));
  await waitFor(() =>
    expect(api.update).toHaveBeenLastCalledWith(
      summary.id,
      { expected_revision: 7, cover_asset_id: null },
      expect.any(String),
    ),
  );
});
it("刷新后的未知主图请求只重放原 key/body，当前详情变化不能替换请求", async () => {
  const key = "bbcc1326-63af-4027-bc30-9a5017492de2";
  const body = { expected_revision: 3, cover_asset_id: null, name: "原名字" };
  saveProjectCoverIntent(sessionStorage, {
    ...scope,
    projectId: summary.id,
    version: 1,
    key,
    body,
  });
  api.get.mockResolvedValue({ ...detail, revision: 10 });
  api.update.mockResolvedValue({ ...detail, revision: 4 });
  setup();
  expect(api.update).not.toHaveBeenCalled();
  expect(
    await screen.findByRole("button", { name: "核验原修改请求" }),
  ).toBeTruthy();
  expect(screen.getByRole("button", { name: "关闭" })).toHaveProperty(
    "disabled",
    true,
  );
  fireEvent.click(screen.getByRole("button", { name: "核验原修改请求" }));
  await waitFor(() =>
    expect(api.update).toHaveBeenCalledExactlyOnceWith(summary.id, body, key),
  );
  expect(loadProjectCoverIntent(sessionStorage, scope, summary.id)).toBeNull();
});
it("409 读取最新设置不丢草稿，明确核对后使用新 revision/key", async () => {
  api.update
    .mockRejectedValueOnce(new ApiError(409, "revision_conflict"))
    .mockResolvedValue({ ...detail, revision: 10 });
  setup();
  const name = await screen.findByRole("textbox", { name: "项目名称" });
  fireEvent.change(name, { target: { value: "我的草稿" } });
  fireEvent.click(screen.getByRole("button", { name: "保存项目设置" }));
  await screen.findByRole("button", { name: "读取最新设置，保留草稿" });
  api.get.mockResolvedValue({
    ...detail,
    name: "别人保存的名称",
    description: "另一个编辑器更新的描述",
    revision: 9,
  });
  fireEvent.click(
    screen.getByRole("button", { name: "读取最新设置，保留草稿" }),
  );
  const acknowledge = await screen.findByRole("button", {
    name: "已核对最新设置，保留草稿继续保存",
  });
  expect((name as HTMLInputElement).value).toBe("我的草稿");
  expect(screen.getByText("别人保存的名称")).toBeTruthy();
  expect(screen.getByRole("button", { name: "保存项目设置" })).toHaveProperty(
    "disabled",
    true,
  );
  fireEvent.click(acknowledge);
  expect(
    (screen.getByRole("textbox", { name: "项目描述" }) as HTMLTextAreaElement)
      .value,
  ).toBe("另一个编辑器更新的描述");
  fireEvent.click(screen.getByRole("button", { name: "保存项目设置" }));
  await waitFor(() => expect(api.update).toHaveBeenCalledTimes(2));
  expect(api.update.mock.calls[1][1]).toEqual({
    expected_revision: 9,
    name: "我的草稿",
  });
  expect(api.update.mock.calls[1][2]).not.toBe(api.update.mock.calls[0][2]);
});
it("归档设置主图只读，恢复未知请求遇到撤权仍保留原记录", async () => {
  api.get.mockResolvedValue({ ...detail, status: "archived" });
  setup();
  expect(
    await screen.findByRole("button", { name: "上传主图" }),
  ).toHaveProperty("disabled", true);
  expect(screen.getByRole("button", { name: "选择项目图片" })).toHaveProperty(
    "disabled",
    true,
  );
  cleanup();
  const stored = {
    ...scope,
    projectId: summary.id,
    version: 1 as const,
    key: "bbcc1326-63af-4027-bc30-9a5017492de2",
    body: { expected_revision: 3, cover_asset_id: null },
  };
  saveProjectCoverIntent(sessionStorage, stored);
  api.scope.mockRejectedValue(new ApiError(403, "forbidden"));
  setup();
  fireEvent.click(
    await screen.findByRole("button", { name: "核验原修改请求" }),
  );
  await waitFor(() => expect(api.scope).toHaveBeenCalled());
  expect(api.update).not.toHaveBeenCalled();
  expect(loadProjectCoverIntent(sessionStorage, scope, summary.id)).toEqual(
    stored,
  );
  expect(screen.getByRole("button", { name: "关闭" })).toHaveProperty(
    "disabled",
    true,
  );
});
it("冲突后最新设置读取失败仍保留已编辑名称，不能卸载草稿", async () => {
  api.update.mockRejectedValue(new ApiError(409, "revision_conflict"));
  setup();
  const name = await screen.findByRole("textbox", { name: "项目名称" });
  fireEvent.change(name, { target: { value: "不丢失的草稿" } });
  fireEvent.click(screen.getByRole("button", { name: "保存项目设置" }));
  await screen.findByRole("button", { name: "读取最新设置，保留草稿" });
  api.get.mockRejectedValue(new ApiError(503, "dependency_unavailable"));
  fireEvent.click(
    screen.getByRole("button", { name: "读取最新设置，保留草稿" }),
  );
  await screen.findByText("项目设置未能读取");
  expect(
    (screen.getByRole("textbox", { name: "项目名称" }) as HTMLInputElement)
      .value,
  ).toBe("不丢失的草稿");
});
it("组件存活期间 scope 换人不能把原 key/body 作为新主体请求发送", async () => {
  const intent = {
    ...scope,
    projectId: summary.id,
    version: 1 as const,
    key: "bbcc1326-63af-4027-bc30-9a5017492de2",
    body: { expected_revision: 3, cover_asset_id: null },
  };
  saveProjectCoverIntent(sessionStorage, intent);
  let changeScope: (next: FolderScope) => void = () => {};
  function Harness() {
    const [current, setCurrent] = useState(scope);
    changeScope = setCurrent;
    return (
      <ProjectManagementDialog
        project={summary}
        action="settings"
        scope={current}
        onClose={vi.fn()}
        onChanged={vi.fn()}
      />
    );
  }
  render(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <Harness />
    </QueryClientProvider>,
  );
  await screen.findByRole("button", { name: "核验原修改请求" });
  const next = { ...scope, actorId: "be22b4c2-bfc3-4af3-910d-5345d9b574af" };
  api.scope.mockResolvedValue({
    current_actor_id: next.actorId,
    current_org_id: next.orgId,
    items: [],
    next_cursor: null,
  });
  await act(async () => changeScope(next));
  fireEvent.click(screen.getByRole("button", { name: "核验原修改请求" }));
  await waitFor(() => expect(screen.getByText(/身份或组织/)).toBeTruthy());
  expect(api.update).not.toHaveBeenCalled();
  expect(loadProjectCoverIntent(sessionStorage, scope, summary.id)).toEqual(
    intent,
  );
});

it("提交后卸载保留原回执请求，迟到成功不关闭新界面或清除恢复记录", async () => {
  let complete: (value: typeof detail) => void = () => {};
  api.update.mockReturnValue(
    new Promise<typeof detail>((resolve) => {
      complete = resolve;
    }),
  );
  const { close, changed, unmount } = setup();
  fireEvent.change(await screen.findByRole("textbox", { name: "项目名称" }), {
    target: { value: "等待核验的修改" },
  });
  fireEvent.click(screen.getByRole("button", { name: "保存项目设置" }));
  await waitFor(() => expect(api.update).toHaveBeenCalledOnce());
  const original = loadProjectCoverIntent(sessionStorage, scope, summary.id);
  expect(original?.body).toEqual({
    expected_revision: 7,
    name: "等待核验的修改",
  });
  unmount();
  await act(async () => complete({ ...detail, revision: 8 }));
  expect(close).not.toHaveBeenCalled();
  expect(changed).not.toHaveBeenCalled();
  expect(loadProjectCoverIntent(sessionStorage, scope, summary.id)).toEqual(
    original,
  );
});
