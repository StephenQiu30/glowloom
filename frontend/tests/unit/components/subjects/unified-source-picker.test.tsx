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
  page_size: 12,
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
      : { ...page, total: 0, items: [] },
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
    page_size: 12,
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
