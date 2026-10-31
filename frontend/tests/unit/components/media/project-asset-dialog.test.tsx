import { render } from "../../../feedback";
import {
  act,
  cleanup,
  fireEvent,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { ProjectAssetDialog } from "@/components/media/project-asset-dialog";
const api = vi.hoisted(() => ({
  list: vi.fn(),
  preview: vi.fn(),
  upload: vi.fn(),
}));
vi.mock("@/api/media", () => ({
  listMediaLibrary: api.list,
  getMediaPreview: api.preview,
}));
vi.mock("@/components/media/project-media", () => ({
  ProjectMediaPreview: () => null,
}));
vi.mock("@/components/media/upload-dialog", () => ({
  UploadDialog: (props: unknown) => {
    api.upload(props);
    return null;
  },
}));
vi.mock("@/components/subjects/subject-list", () => ({
  SubjectList: () => null,
}));
const pid = "00000000-0000-4000-8000-000000000001";
const a = {
  id: "00000000-0000-4000-8000-000000000011",
  project_id: pid,
  kind: "image",
  revision: 1,
  file_name: "海边香氛.png",
};
const b = {
  ...a,
  id: "00000000-0000-4000-8000-000000000012",
  file_name: "蓝蝶.png",
};
function library(items: API.AssetSummary[], page = 1, total = items.length) {
  return {
    current_actor_id: pid,
    current_org_id: pid,
    library_id: pid,
    revision: 1,
    scope: { kind: "project", project_id: items[0]?.project_id ?? pid },
    page,
    page_size: 40,
    total,
    items: items.map((asset) => ({
      id: asset.id,
      asset_id: asset.id,
      kind: asset.kind,
      catalog_state: "active",
      media: { ...asset, project_id: undefined },
    })),
  };
}
beforeEach(() => {
  vi.resetAllMocks();
  api.preview.mockImplementation(({ asset_id }) =>
    Promise.resolve({ asset: asset_id === a.id ? a : b }),
  );
});
afterEach(cleanup);
it("单选参考上传同类素材时替换原选择，异类上传不改变参考类型", async () => {
  api.list.mockResolvedValue(library([a, b]));
  const add = vi.fn();
  render(
    <ProjectAssetDialog
      projectID={pid}
      reference
      initialKind="image"
      onClose={vi.fn()}
      onAdd={add}
    />,
  );
  fireEvent.click(
    await screen.findByRole("button", { name: "选择素材 海边香氛.png" }),
  );
  const replacement = screen.getByRole("button", { name: "选择素材 蓝蝶.png" });
  expect(replacement.hasAttribute("disabled")).toBe(false);
  fireEvent.click(replacement);
  expect(replacement.getAttribute("aria-pressed")).toBe("true");
  fireEvent.click(
    screen.getByRole("button", { name: "选择素材 海边香氛.png" }),
  );
  fireEvent.click(screen.getByRole("button", { name: "上传素材" }));
  act(() => api.upload.mock.lastCall![0].onUploaded(b));
  fireEvent.click(screen.getByRole("button", { name: "添加为参考" }));
  await waitFor(() => expect(add).toHaveBeenCalledWith([b]));
  fireEvent.click(screen.getByRole("button", { name: "上传素材" }));
  act(() => api.upload.mock.lastCall![0].onUploaded({ ...b, kind: "video" }));
  await waitFor(() => expect(api.list.mock.lastCall![0].kind).toBe("image"));
});
it("多选、搜索与分页保留已选资源身份，确认前不写入画布", async () => {
  api.list
    .mockResolvedValueOnce(library([a], 1, 41))
    .mockResolvedValueOnce(library([b], 2, 41))
    .mockResolvedValue(library([a, b]));
  const add = vi.fn();
  render(
    <ProjectAssetDialog
      projectID={pid}
      onClose={vi.fn()}
      onAdd={add}
      onSubject={vi.fn()}
    />,
  );
  fireEvent.click(
    await screen.findByRole("button", { name: "选择素材 海边香氛.png" }),
  );
  expect(add).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole("button", { name: "加载更多" }));
  fireEvent.click(
    await screen.findByRole("button", { name: "选择素材 蓝蝶.png" }),
  );
  fireEvent.change(screen.getByLabelText("搜索项目素材"), {
    target: { value: "蓝蝶" },
  });
  expect(
    screen.queryByRole("button", { name: "选择素材 海边香氛.png" }),
  ).toBeNull();
  expect(screen.getByText("已选择 2 个素材（最多 24 个）")).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "添加到画布" }));
  await waitFor(() => expect(add).toHaveBeenCalledExactlyOnceWith([a, b]));
  expect(api.list.mock.calls[1][0]).toMatchObject({
    scope: "project",
    project_id: pid,
    kind: "image",
    page: 2,
    page_size: 40,
  });
});
it("跨项目回执不进入可选素材，重新读取可恢复", async () => {
  api.list
    .mockResolvedValueOnce(library([{ ...a, project_id: a.id }]))
    .mockResolvedValueOnce(library([a]));
  render(
    <ProjectAssetDialog
      projectID={pid}
      onClose={vi.fn()}
      onAdd={vi.fn()}
      onSubject={vi.fn()}
    />,
  );
  expect(await screen.findByRole("alert")).toHaveProperty(
    "textContent",
    expect.stringContaining("素材暂时无法读取"),
  );
  expect(
    screen.queryByRole("button", { name: "选择素材 海边香氛.png" }),
  ).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "重试" }));
  await screen.findByRole("button", { name: "选择素材 海边香氛.png" });
});
it("关闭弹窗中止读取，迟到回执不能继续更新弹窗", async () => {
  let finish!: (v: unknown) => void;
  api.list.mockImplementation(
    () =>
      new Promise((resolve) => {
        finish = resolve;
      }),
  );
  const view = render(
    <ProjectAssetDialog
      projectID={pid}
      onClose={vi.fn()}
      onAdd={vi.fn()}
      onSubject={vi.fn()}
    />,
  );
  await waitFor(() => expect(api.list).toHaveBeenCalledOnce());
  const signal = api.list.mock.calls[0][1].signal as AbortSignal;
  view.unmount();
  expect(signal.aborted).toBe(true);
  await act(async () => finish(library([a])));
  expect(screen.queryByRole("dialog")).toBeNull();
});
it("真实空列表和已加载素材搜索无结果具有不同提示", async () => {
  api.list.mockImplementation(({ search }) =>
    Promise.resolve(library(search ? [] : [a])),
  );
  render(
    <ProjectAssetDialog
      projectID={pid}
      onClose={vi.fn()}
      onAdd={vi.fn()}
      onSubject={vi.fn()}
    />,
  );
  await screen.findByRole("button", { name: "选择素材 海边香氛.png" });
  fireEvent.change(screen.getByLabelText("搜索项目素材"), {
    target: { value: "不存在" },
  });
  expect(await screen.findByText("没有匹配的素材")).toBeTruthy();
  expect(
    screen.getByRole("button", { name: "添加到画布" }).hasAttribute("disabled"),
  ).toBe(true);
});

it("真实空分页显示空态而非回执错误", async () => {
  api.list.mockResolvedValue(library([]));
  render(
    <ProjectAssetDialog
      projectID={pid}
      onClose={vi.fn()}
      onAdd={vi.fn()}
      onSubject={vi.fn()}
    />,
  );
  expect(await screen.findByText("当前项目暂无此类素材")).toBeTruthy();
  expect(screen.queryByText(/连接中断|结果未确认/)).toBeNull();
});

it("搜索交给完整服务端分页，不遍历或客户端过滤，选中项保留", async () => {
  api.list.mockImplementation(({ search }) =>
    Promise.resolve(search ? library([b]) : library([a], 1, 100)),
  );
  render(
    <ProjectAssetDialog projectID={pid} onClose={vi.fn()} onAdd={vi.fn()} />,
  );
  fireEvent.click(
    await screen.findByRole("button", { name: "选择素材 海边香氛.png" }),
  );
  fireEvent.change(screen.getByLabelText("搜索项目素材"), {
    target: { value: "蓝蝶" },
  });
  expect(
    await screen.findByRole("button", { name: "选择素材 蓝蝶.png" }),
  ).toBeTruthy();
  expect(screen.getByText("已选择 1 个素材（最多 24 个）")).toBeTruthy();
  expect(api.list).toHaveBeenCalledTimes(2);
  expect(api.list.mock.lastCall![0]).toMatchObject({ search: "蓝蝶", page: 1 });
  fireEvent.click(screen.getByRole("button", { name: "选择本页" }));
  expect(screen.getByText("已选择 2 个素材（最多 24 个）")).toBeTruthy();
});
it("确认核验失败保留选择且不调用添加", async () => {
  api.list.mockResolvedValue(library([a]));
  api.preview.mockRejectedValue(new Error("素材不可用"));
  const add = vi.fn();
  render(<ProjectAssetDialog projectID={pid} onClose={vi.fn()} onAdd={add} />);
  fireEvent.click(
    await screen.findByRole("button", { name: "选择素材 海边香氛.png" }),
  );
  fireEvent.click(screen.getByRole("button", { name: "添加到画布" }));
  expect((await screen.findByRole("alert")).textContent).toContain(
    "素材不可用",
  );
  expect(add).not.toHaveBeenCalled();
  expect(screen.getByText("已选择 1 个素材（最多 24 个）")).toBeTruthy();
});

it("画布替换只列出允许类型，单选新素材替换旧选择", async () => {
  api.list.mockResolvedValue(library([a, b]));
  const add = vi.fn();
  render(
    <ProjectAssetDialog
      projectID={pid}
      allowedKinds={["image"]}
      selectionLimit={1}
      canvasPresentation
      onClose={vi.fn()}
      onAdd={add}
    />,
  );
  expect(screen.queryByRole("radio", { name: "视频" })).toBeNull();
  fireEvent.click(
    await screen.findByRole("button", { name: "选择素材 海边香氛.png" }),
  );
  fireEvent.click(screen.getByRole("button", { name: "选择素材 蓝蝶.png" }));
  expect(
    screen
      .getByRole("button", { name: "选择素材 海边香氛.png" })
      .getAttribute("aria-pressed"),
  ).toBe("false");
  fireEvent.click(screen.getByRole("button", { name: "添加到画布" }));
  await waitFor(() => expect(add).toHaveBeenCalledExactlyOnceWith([b]));
});
it("切换媒体类型清除图片多选规格并从第一页读取，切回不复活条件", async () => {
  api.list.mockResolvedValue(library([]));
  render(
    <ProjectAssetDialog projectID={pid} onClose={vi.fn()} onAdd={vi.fn()} />,
  );
  await screen.findByText("当前项目暂无此类素材");
  fireEvent.click(screen.getByRole("button", { name: "来源与规格筛选" }));
  fireEvent.click(screen.getByRole("checkbox", { name: "1K" }));
  fireEvent.click(screen.getByRole("checkbox", { name: "2K" }));
  fireEvent.click(screen.getByRole("checkbox", { name: "16:9" }));
  fireEvent.click(screen.getByRole("checkbox", { name: "1:1" }));
  await waitFor(() =>
    expect(api.list.mock.lastCall![0]).toMatchObject({
      kind: "image",
      resolutions: "1K,2K",
      ratios: "16:9,1:1",
      page: 1,
    }),
  );
  fireEvent.click(
    screen.getByRole("button", { name: "来源与规格筛选，已选 4 项" }),
  );
  fireEvent.click(screen.getByRole("radio", { name: "视频" }));
  await waitFor(() =>
    expect(api.list.mock.lastCall![0]).toMatchObject({
      kind: "video",
      resolutions: undefined,
      ratios: undefined,
      page: 1,
    }),
  );
  fireEvent.click(screen.getByRole("radio", { name: "图片" }));
  await waitFor(() =>
    expect(api.list.mock.lastCall![0]).toMatchObject({
      kind: "image",
      resolutions: undefined,
      ratios: undefined,
      page: 1,
    }),
  );
  fireEvent.click(screen.getByRole("button", { name: "来源与规格筛选" }));
  expect(
    screen.getByRole("checkbox", { name: "2K" }).getAttribute("data-state"),
  ).toBe("unchecked");
});
