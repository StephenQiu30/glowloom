import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { UnifiedSubjectSourcePicker } from "@/components/subjects/unified-source-picker";
const api = vi.hoisted(() => ({ list: vi.fn(), get: vi.fn() }));
vi.mock("@/api/subjects", () => ({
  listSubjectSources: api.list,
  getSubjectSource: api.get,
}));
vi.mock("@/components/subjects/unified-media", () => ({
  UnifiedSubjectMedia: () => <span aria-hidden="true">预览</span>,
}));
const actor = "11111111-1111-4111-8111-111111111111",
  team = "22222222-2222-4222-8222-222222222222",
  personalOrg = "33333333-3333-4333-8333-333333333333",
  project = "44444444-4444-4444-8444-444444444444",
  item = "55555555-5555-4555-8555-555555555555",
  asset = "66666666-6666-4666-8666-666666666666";
const entry = {
  id: asset,
  source: "personal_asset",
  title: "香氛主图.png",
  kind: "image",
  revision: 3,
  favorite: false,
  upscaled: false,
  created_at: "2026-10-28T00:00:00Z",
  updated_at: "2026-10-28T00:00:00Z",
  media: {
    id: asset,
    revision: 7,
    kind: "image",
    mime_type: "image/png",
    file_name: "香氛主图.png",
    byte_size: 20,
    origin: "upload",
  },
};
const page = {
  current_actor_id: actor,
  current_scope_id: team,
  page: 1,
  page_size: 30,
  total: 1,
  items: [entry],
  capabilities: {
    upscaled_filter: true,
  },
};
beforeEach(() => {
  vi.clearAllMocks();
  api.list.mockImplementation(async (query) =>
    query.kind === "image"
      ? {
          ...page,
          items: [
            {
              ...entry,
              favorite: !!query.favorite_only,
              ...(query.quality === "upscaled"
                ? {
                    id: item,
                    source: "generated_output",
                    project_id: project,
                    upscaled: true,
                    media: { ...entry.media, origin: "generated" },
                  }
                : {}),
            },
          ],
        }
      : { ...page, page_size: query.page_size, total: 0, items: [] },
  );
  api.get.mockResolvedValue(entry);
});
afterEach(cleanup);
it("团队目标空间通过专用来源读取本人资产，确认使用正式asset事实", async () => {
  const onSelect = vi.fn();
  render(
    <UnifiedSubjectSourcePicker
      actorID={actor}
      scopeID={team}
      allowedKinds={["image", "video"]}
      onClose={vi.fn()}
      onSelect={onSelect}
    />,
  );
  expect(screen.getByRole("button", { name: "确认" })).toHaveProperty(
    "disabled",
    true,
  );
  fireEvent.click(await screen.findByRole("button", { name: "香氛主图.png" }));
  fireEvent.click(screen.getByRole("button", { name: "确认" }));
  await waitFor(() =>
    expect(onSelect).toHaveBeenCalledWith(
      expect.objectContaining({
        asset_id: asset,
        asset_revision: 7,
        library_item_id: asset,
        library_source: "subject",
      }),
    ),
  );
  expect(api.list.mock.calls[0][0]).toMatchObject({
    kind: "image",
    page: 1,
    page_size: 30,
  });
  expect(api.list.mock.calls[0][0]).not.toHaveProperty("scope");
  expect(api.get).toHaveBeenCalledWith({ item_id: asset }, expect.any(Object));
});
it("图片/视频、收藏与真实超清分组均由服务器完整过滤", async () => {
  render(
    <UnifiedSubjectSourcePicker
      actorID={actor}
      scopeID={team}
      allowedKinds={["image", "video"]}
      onClose={vi.fn()}
      onSelect={vi.fn()}
    />,
  );
  await screen.findByRole("button", { name: "香氛主图.png" });
  expect(screen.getByRole("tab", { name: "超清" })).toHaveProperty(
    "disabled",
    false,
  );
  fireEvent.mouseDown(screen.getByRole("tab", { name: "收藏" }), {
    button: 0,
    ctrlKey: false,
  });
  await waitFor(() =>
    expect(api.list.mock.calls.at(-1)![0]).toMatchObject({
      kind: "image",
      favorite_only: true,
      page: 1,
    }),
  );
  fireEvent.mouseDown(screen.getByRole("tab", { name: "超清" }), {
    button: 0,
    ctrlKey: false,
  });
  await waitFor(() =>
    expect(api.list.mock.calls.at(-1)![0]).toMatchObject({
      kind: "image",
      quality: "upscaled",
      favorite_only: undefined,
      page: 1,
    }),
  );
  fireEvent.mouseDown(screen.getByRole("tab", { name: "视频" }), {
    button: 0,
    ctrlKey: false,
  });
  await waitFor(() =>
    expect(api.list.mock.calls.at(-1)![0]).toMatchObject({
      kind: "video",
      favorite_only: undefined,
      quality: undefined,
      page: 1,
    }),
  );
  expect(screen.queryByRole("tab", { name: "超清" })).toBeNull();
});

it("生成结果由服务端来源ID确认，采纳保留真实project_id与媒体revision", async () => {
  const generated = {
    ...entry,
    id: item,
    source: "generated_output",
    project_id: project,
    upscaled: true,
    media: { ...entry.media, origin: "generated" },
  };
  api.list.mockResolvedValue({ ...page, items: [generated] });
  api.get.mockResolvedValue(generated);
  const select = vi.fn();
  render(
    <UnifiedSubjectSourcePicker
      actorID={actor}
      scopeID={team}
      allowedKinds={["image"]}
      onClose={vi.fn()}
      onSelect={select}
    />,
  );
  fireEvent.click(await screen.findByRole("button", { name: "香氛主图.png" }));
  fireEvent.click(screen.getByRole("button", { name: "确认" }));
  await waitFor(() =>
    expect(select).toHaveBeenCalledWith(
      expect.objectContaining({
        asset_id: asset,
        asset_revision: 7,
        project_id: project,
        library_item_id: item,
        library_source: "subject",
      }),
    ),
  );
  expect(api.get).toHaveBeenCalledWith({ item_id: item }, expect.anything());
});
it("拒绝错误目标空间回执，不能显示其他空间来源", async () => {
  api.list.mockResolvedValue({ ...page, current_scope_id: personalOrg });
  render(
    <UnifiedSubjectSourcePicker
      actorID={actor}
      scopeID={team}
      allowedKinds={["image"]}
      onClose={vi.fn()}
      onSelect={vi.fn()}
    />,
  );
  await screen.findByRole("heading", { name: "暂时无法读取资产" });
  expect(screen.queryByText("香氛主图.png")).toBeNull();
});

it("确认前来源项目或媒体版本变化时不把新内容静默绑定", async () => {
  api.get.mockResolvedValue({
    ...entry,
    media: { ...entry.media, revision: 8 },
  });
  const select = vi.fn();
  render(
    <UnifiedSubjectSourcePicker
      actorID={actor}
      scopeID={team}
      allowedKinds={["image"]}
      onClose={vi.fn()}
      onSelect={select}
    />,
  );
  fireEvent.click(await screen.findByRole("button", { name: "香氛主图.png" }));
  fireEvent.click(screen.getByRole("button", { name: "确认" }));
  await screen.findByRole("heading", { name: "暂时无法读取资产" });
  expect(select).not.toHaveBeenCalled();
});

function openPicker(
  extra: Partial<React.ComponentProps<typeof UnifiedSubjectSourcePicker>> = {},
) {
  return render(
    <UnifiedSubjectSourcePicker
      actorID={actor}
      scopeID={team}
      allowedKinds={["image", "video"]}
      onClose={vi.fn()}
      onSelect={vi.fn()}
      {...extra}
    />,
  );
}
function tab(name: string) {
  fireEvent.mouseDown(screen.getByRole("tab", { name }), {
    button: 0,
    ctrlKey: false,
  });
}
const second = {
  ...entry,
  id: item,
  title: "另一张图片",
  media: { ...entry.media, id: item },
};
it("素材选择器没有搜索或页码，超清无能力时仍可进入真实空态", async () => {
  api.list.mockImplementation(async (query) => ({
    ...page,
    capabilities: { upscaled_filter: false },
    ...(query.quality ? { total: 0, items: [] } : { total: 31 }),
  }));
  openPicker();
  await screen.findByRole("button", { name: entry.title });
  expect(screen.queryByRole("textbox", { name: "搜索资产" })).toBeNull();
  expect(screen.queryByRole("navigation", { name: "主体资产分页" })).toBeNull();
  expect(screen.queryByText("暂无可用的超清来源。")).toBeNull();
  expect(screen.getByRole("tab", { name: "超清" })).not.toHaveProperty(
    "disabled",
    true,
  );
  tab("超清");
  await screen.findByText("暂未找到相关内容");
  expect(api.list.mock.calls.at(-1)![0]).toMatchObject({
    quality: "upscaled",
    page_size: 30,
  });
});
it("图片跨分组保留多选并逐项核验后一次采纳，切换类型清空", async () => {
  api.list.mockImplementation(async (query) => ({
    ...page,
    page_size: query.page_size,
    items:
      query.kind !== "image" ? [] : query.favorite_only ? [second] : [entry],
    total: query.kind === "image" ? 1 : 0,
  }));
  api.get.mockImplementation(async ({ item_id }) =>
    item_id === asset ? entry : second,
  );
  const onSelect = vi.fn(),
    onSelectMany = vi.fn();
  openPicker({ onSelect, onSelectMany });
  fireEvent.click(await screen.findByRole("button", { name: entry.title }));
  tab("收藏");
  fireEvent.click(await screen.findByRole("button", { name: second.title }));
  expect(screen.getByText(/已选 2 张图片/)).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "确认" }));
  await waitFor(() =>
    expect(onSelectMany).toHaveBeenCalledWith([
      expect.objectContaining({ asset_id: asset }),
      expect.objectContaining({ asset_id: item }),
    ]),
  );
  expect(api.get).toHaveBeenCalledTimes(2);
  expect(onSelect).not.toHaveBeenCalled();
  tab("视频");
  await screen.findByText(/已选 0 个视频/);
  expect(api.list.mock.calls.at(-1)![0]).toMatchObject({ page_size: 50 });
  tab("图片");
  await screen.findByText(/已选 0 张图片/);
});
it("图片遵守调用方剩余参考容量，取消选中后可选择另一个", async () => {
  api.list.mockResolvedValue({ ...page, items: [entry, second], total: 2 });
  openPicker({ onSelectMany: vi.fn(), maxSelection: 1 });
  const first = await screen.findByRole("button", { name: entry.title });
  const other = screen.getByRole("button", { name: second.title });
  fireEvent.click(first);
  expect(other).toHaveProperty("disabled", true);
  fireEvent.click(first);
  expect(other).not.toHaveProperty("disabled", true);
  fireEvent.click(other);
  expect(other).toHaveProperty("ariaPressed", "true");
});
it("多选中任何一项版本变化，不部分采纳其它项", async () => {
  api.list.mockResolvedValue({ ...page, items: [entry, second], total: 2 });
  api.get.mockImplementation(async ({ item_id }) =>
    item_id === asset ? entry : { ...second, revision: 4 },
  );
  const onSelectMany = vi.fn();
  openPicker({ onSelectMany });
  fireEvent.click(await screen.findByRole("button", { name: entry.title }));
  fireEvent.click(screen.getByRole("button", { name: second.title }));
  fireEvent.click(screen.getByRole("button", { name: "确认" }));
  await screen.findByRole("heading", { name: "暂时无法读取资产" });
  expect(onSelectMany).not.toHaveBeenCalled();
  expect(api.get).toHaveBeenCalledTimes(2);
});
it("切换空间立即隐藏旧选择并中止正在核验的请求", async () => {
  let resolve!: (value: unknown) => void;
  api.get.mockImplementation(
    () =>
      new Promise((done) => {
        resolve = done;
      }),
  );
  const onSelect = vi.fn();
  const view = openPicker({ onSelect });
  fireEvent.click(await screen.findByRole("button", { name: entry.title }));
  fireEvent.click(screen.getByRole("button", { name: "确认" }));
  const signal = api.get.mock.calls[0][1].signal;
  view.rerender(
    <UnifiedSubjectSourcePicker
      actorID={actor}
      scopeID={personalOrg}
      allowedKinds={["image"]}
      onClose={vi.fn()}
      onSelect={onSelect}
    />,
  );
  expect(signal.aborted).toBe(true);
  resolve(entry);
  await waitFor(() =>
    expect(screen.getByRole("button", { name: "确认" })).toHaveProperty(
      "disabled",
      true,
    ),
  );
  expect(onSelect).not.toHaveBeenCalled();
});

it("列表内部触底追加，尾批失败保留已选素材并在原处重试", async () => {
  api.list
    .mockResolvedValueOnce({ ...page, total: 31 })
    .mockRejectedValueOnce(new Error("offline"))
    .mockResolvedValueOnce({ ...page, page: 2, total: 31, items: [second] });
  openPicker({ onSelectMany: vi.fn() });
  fireEvent.click(await screen.findByRole("button", { name: entry.title }));
  const list = screen.getByRole("region", { name: "资产列表" });
  Object.defineProperties(list, {
    clientHeight: { configurable: true, value: 400 },
    scrollHeight: { configurable: true, value: 900 },
    scrollTop: { configurable: true, value: 200 },
  });
  fireEvent.scroll(list);
  await screen.findByRole("button", { name: "重试" });
  expect(
    screen
      .getByRole("button", { name: entry.title })
      .getAttribute("aria-pressed"),
  ).toBe("true");
  expect(screen.getByRole("button", { name: "确认" })).toHaveProperty(
    "disabled",
    false,
  );
  fireEvent.scroll(list);
  expect(api.list).toHaveBeenCalledTimes(2);
  fireEvent.click(screen.getByRole("button", { name: "重试" }));
  await screen.findByRole("button", { name: second.title });
  expect(api.list.mock.calls.map(([query]) => query.page)).toEqual([1, 2, 2]);
  expect(
    screen
      .getByRole("button", { name: entry.title })
      .getAttribute("aria-pressed"),
  ).toBe("true");
  fireEvent.scroll(list);
  expect(api.list).toHaveBeenCalledTimes(3);
});
it.each(["video", "audio"] as const)(
  "%s即使传多选回调仍单选，旧回调只接收最后选择",
  async (kind) => {
    const media = { ...entry.media, kind, mime_type: `${kind}/mp4` };
    const first = { ...entry, kind, media };
    const other = { ...second, kind, media: { ...media, id: item } };
    api.list.mockResolvedValue({
      ...page,
      page_size: kind === "video" ? 50 : 30,
      total: 2,
      items: [first, other],
    });
    api.get.mockResolvedValue(other);
    const onSelect = vi.fn(),
      onSelectMany = vi.fn();
    openPicker({ allowedKinds: [kind], onSelect, onSelectMany });
    fireEvent.click(await screen.findByRole("button", { name: first.title }));
    fireEvent.click(screen.getByRole("button", { name: other.title }));
    expect(
      screen
        .getByRole("button", { name: first.title })
        .getAttribute("aria-pressed"),
    ).toBe("false");
    fireEvent.click(screen.getByRole("button", { name: "确认" }));
    await waitFor(() =>
      expect(onSelect).toHaveBeenCalledWith(
        expect.objectContaining({ asset_id: item, kind }),
      ),
    );
    expect(onSelectMany).not.toHaveBeenCalled();
  },
);
it("遮罩点击不关闭，显式关闭按钮仍可退出", async () => {
  const onClose = vi.fn();
  openPicker({ onClose });
  await screen.findByRole("button", { name: entry.title });
  const overlay = document.querySelector('[data-slot="dialog-overlay"]')!;
  fireEvent.pointerDown(overlay, { pointerType: "mouse", button: 0 });
  fireEvent.click(overlay);
  expect(onClose).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole("button", { name: "关闭" }));
  expect(onClose).toHaveBeenCalledTimes(1);
});
