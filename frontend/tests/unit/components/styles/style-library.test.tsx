import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { StyleLibraryPopover } from "@/components/styles/style-library-popover";
import { StyleEditorDialog } from "@/components/styles/style-editor-dialog";
import { RequestError } from "@/lib/request";

const api = vi.hoisted(() => ({
  list: vi.fn(),
  read: vi.fn(),
  create: vi.fn(),
  update: vi.fn(),
  remove: vi.fn(),
}));
vi.mock("@/components/styles/style-api", async (original) => ({
  ...(await original<typeof import("@/components/styles/style-api")>()),
  listStyles: api.list,
  readStyle: api.read,
  createStyle: api.create,
  updateStyle: api.update,
  deleteStyle: api.remove,
}));
vi.mock("@/components/media/personal-reference-picker", () => ({
  PersonalMediaPreview: () => null,
  PersonalLibraryPicker: ({
    onSelect,
  }: {
    onSelect: (items: unknown[]) => void;
  }) => (
    <button
      onClick={() =>
        onSelect([
          {
            id: "22222222-2222-4222-8222-222222222222",
            asset_id: "22222222-2222-4222-8222-222222222222",
            revision: 1,
            media: { id: "22222222-2222-4222-8222-222222222222", revision: 1 },
            kind: "image",
            title: "参考图",
          },
        ])
      }
    >
      选择测试参考图
    </button>
  ),
}));
vi.mock("@/components/styles/style-analysis-dialog", () => ({
  StyleAnalysisDialog: ({ onUse }: { onUse: (text: string) => void }) => (
    <button
      onClick={() =>
        onUse("风格名称：电影感\n\n风格描述：低饱和冷色、柔和侧光。")
      }
    >
      采用测试分析
    </button>
  ),
}));
const owner = { actorId: "actor", scopeId: "scope" };
const id = "11111111-1111-4111-8111-111111111111";
const value = {
  id,
  name: "水墨",
  category: "2d" as const,
  revision: 1,
  cover_asset_id: null,
  reference_asset_ids: [],
  owned: true,
  can_edit: true,
  prompt_fragment: "疏朗的水墨构图",
  negative_prompt: "避免霓虹色",
  references_available: true,
  reference_items: [],
};
beforeEach(() => {
  vi.clearAllMocks();
  api.list.mockResolvedValue({ items: [value], next_cursor: null });
  api.read.mockResolvedValue(value);
});
afterEach(cleanup);
async function openLibrary(onChange = vi.fn()) {
  render(
    <StyleLibraryPopover owner={owner} value={null} onChange={onChange} />,
  );
  fireEvent.click(screen.getByRole("button", { name: "选择风格" }));
  await screen.findByRole("button", { name: "选择水墨" });
  return onChange;
}
it("uses the detail revision instead of silently selecting the summary", async () => {
  const onChange = await openLibrary();
  api.read.mockResolvedValue({
    ...value,
    revision: 2,
    prompt_fragment: "真实详情",
  });
  fireEvent.click(screen.getByRole("button", { name: "选择水墨" }));
  await waitFor(() => expect(onChange).toHaveBeenCalled());
  expect(onChange.mock.calls[0][0].prompt_fragment).toBe("真实详情");
  expect(Object.isFrozen(onChange.mock.calls[0][0].reference_asset_ids)).toBe(
    true,
  );
});
it("leaves the selected draft unchanged if detail cannot be read", async () => {
  const change = vi.fn();
  render(<StyleLibraryPopover owner={owner} value={value} onChange={change} />);
  fireEvent.click(screen.getByRole("button", { name: "选择风格" }));
  api.read.mockRejectedValue(new RequestError("not_found", 404));
  fireEvent.click(await screen.findByRole("button", { name: "选择水墨" }));
  await screen.findByRole("alert");
  expect(change).not.toHaveBeenCalled();
});
it("clears style only through the explicit no-style control", async () => {
  const change = await openLibrary();
  fireEvent.click(screen.getByRole("button", { name: "不使用风格" }));
  expect(change).toHaveBeenCalledWith(null);
});
it("does not adopt a late detail after changing owner", async () => {
  let resolve!: (v: typeof value) => void;
  api.read.mockImplementation(
    () =>
      new Promise((r) => {
        resolve = r;
      }),
  );
  const change = vi.fn();
  const { rerender } = render(
    <StyleLibraryPopover owner={owner} value={null} onChange={change} />,
  );
  fireEvent.click(screen.getByRole("button", { name: "选择风格" }));
  fireEvent.click(await screen.findByRole("button", { name: "选择水墨" }));
  rerender(
    <StyleLibraryPopover
      owner={{ ...owner, actorId: "other" }}
      value={null}
      onChange={change}
    />,
  );
  resolve(value);
  await waitFor(() =>
    expect(screen.queryByRole("button", { name: "选择水墨" })).toBeNull(),
  );
  expect(change).not.toHaveBeenCalled();
});
it("does not adopt a detail after dismissing the picker", async () => {
  let resolve!: (data: typeof value) => void;
  api.read.mockImplementation(
    () =>
      new Promise((complete) => {
        resolve = complete;
      }),
  );
  const change = await openLibrary();
  fireEvent.click(screen.getByRole("button", { name: "选择水墨" }));
  fireEvent.keyDown(document, { key: "Escape" });
  await waitFor(() =>
    expect(screen.queryByRole("button", { name: "选择水墨" })).toBeNull(),
  );
  await act(async () => resolve(value));
  expect(change).not.toHaveBeenCalled();
});
it("can revisit the next cursor after moving back without treating it as a loop", async () => {
  api.list.mockImplementation(async (query) => ({
    items: [value],
    next_cursor: query.cursor ? null : "page-two",
  }));
  await openLibrary();
  fireEvent.click(screen.getByRole("button", { name: "下一页风格" }));
  await screen.findByText("第 2 页");
  await screen.findByRole("button", { name: "选择水墨" });
  fireEvent.click(screen.getByRole("button", { name: "上一页风格" }));
  await screen.findByText("第 1 页");
  await screen.findByRole("button", { name: "选择水墨" });
  fireEvent.click(screen.getByRole("button", { name: "下一页风格" }));
  await screen.findByText("第 2 页");
});
it("changes the real query when selecting personal or category tabs", async () => {
  await openLibrary();
  fireEvent.mouseDown(screen.getByRole("tab", { name: "我的风格" }), {
    button: 0,
    ctrlKey: false,
  });
  await waitFor(() =>
    expect(api.list.mock.calls.at(-1)?.[0]).toMatchObject({ mine: true }),
  );
  fireEvent.mouseDown(screen.getByRole("tab", { name: "3D" }), {
    button: 0,
    ctrlKey: false,
  });
  await waitFor(() =>
    expect(api.list.mock.calls.at(-1)?.[0]).toMatchObject({ category: "3d" }),
  );
});
it("counts Unicode code points for the twenty-character name limit", async () => {
  const saved = vi.fn();
  api.create.mockImplementation(async (input, key) => ({
    request_id: key,
    action: "create",
    preset: { ...value, ...input },
    deleted: false,
  }));
  render(
    <StyleEditorDialog owner={owner} onSaved={saved} onClose={() => {}} />,
  );
  const name = screen.getByRole("textbox", { name: "风格名称" });
  fireEvent.change(name, { target: { value: "😀".repeat(21) } });
  fireEvent.change(screen.getByRole("textbox", { name: "风格描述" }), {
    target: { value: "水墨构图" },
  });
  expect(
    screen.getByRole("button", { name: "保存并使用" }).hasAttribute("disabled"),
  ).toBe(true);
  fireEvent.change(name, { target: { value: "😀".repeat(20) } });
  fireEvent.click(screen.getByRole("button", { name: "保存并使用" }));
  await waitFor(() => expect(saved).toHaveBeenCalled());
  expect(saved.mock.calls[0][0].name).toBe("😀".repeat(20));
});
it("locks an unknown write and retries the identical payload and idempotency key", async () => {
  api.create
    .mockRejectedValueOnce(new Error("network"))
    .mockImplementation(async (input, key) => ({
      request_id: key,
      action: "create",
      preset: { ...value, ...input },
      deleted: false,
    }));
  const saved = vi.fn();
  render(
    <StyleEditorDialog owner={owner} onSaved={saved} onClose={() => {}} />,
  );
  fireEvent.change(screen.getByRole("textbox", { name: "风格名称" }), {
    target: { value: "水墨" },
  });
  fireEvent.change(screen.getByRole("textbox", { name: "风格描述" }), {
    target: { value: "水墨构图" },
  });
  fireEvent.click(screen.getByRole("button", { name: "保存并使用" }));
  const retry = await screen.findByRole("button", { name: "重试原保存" });
  expect(
    screen.getByRole("textbox", { name: "风格名称" }).hasAttribute("disabled"),
  ).toBe(true);
  expect(saved).not.toHaveBeenCalled();
  fireEvent.click(retry);
  await waitFor(() => expect(saved).toHaveBeenCalled());
  expect(api.create.mock.calls[0].slice(0, 2)).toEqual(
    api.create.mock.calls[1].slice(0, 2),
  );
});
it("keeps the editable style when a bound style delete returns a conflict", async () => {
  api.remove.mockRejectedValue(new RequestError("style_preset_in_use", 409));
  await openLibrary();
  fireEvent.click(screen.getByRole("button", { name: "删除水墨" }));
  await screen.findByText("风格仍被使用，暂不能删除。当前风格已保留。");
  expect(screen.getByRole("button", { name: "选择水墨" })).toBeTruthy();
});
it("fills an empty name from the explicit analysis label and preserves an existing name", async () => {
  const first = render(
    <StyleEditorDialog owner={owner} onSaved={() => {}} onClose={() => {}} />,
  );
  fireEvent.click(screen.getByRole("button", { name: "添加参考图" }));
  fireEvent.click(
    await screen.findByRole("button", { name: "选择测试参考图", hidden: true }),
  );
  fireEvent.click(screen.getByRole("button", { name: "分析风格" }));
  fireEvent.click(
    await screen.findByRole("button", { name: "采用测试分析", hidden: true }),
  );
  expect(
    (screen.getByRole("textbox", { name: "风格名称" }) as HTMLInputElement)
      .value,
  ).toBe("电影感");
  expect(
    (screen.getByRole("textbox", { name: "风格描述" }) as HTMLTextAreaElement)
      .value,
  ).toBe("低饱和冷色、柔和侧光。");
  first.unmount();
  render(
    <StyleEditorDialog
      owner={owner}
      value={value}
      onSaved={() => {}}
      onClose={() => {}}
    />,
  );
  fireEvent.click(screen.getByRole("button", { name: "添加参考图" }));
  fireEvent.click(
    await screen.findByRole("button", { name: "选择测试参考图", hidden: true }),
  );
  fireEvent.click(screen.getByRole("button", { name: "分析风格" }));
  fireEvent.click(
    await screen.findByRole("button", { name: "采用测试分析", hidden: true }),
  );
  expect(
    (screen.getByRole("textbox", { name: "风格名称" }) as HTMLInputElement)
      .value,
  ).toBe("水墨");
});
it("saves the reordered first image as cover and keeps the four-image limit", async () => {
  const references = Array.from({ length: 4 }, (_, index) => ({
    id: `${index + 1}2222222-2222-4222-8222-222222222222`,
    revision: 1,
    asset_id: `${index + 1}2222222-2222-4222-8222-222222222222`,
    asset_revision: 1,
    title: "参考图",
  }));
  api.update.mockImplementation(async (_id, input, key) => ({
    request_id: key,
    action: "patch",
    deleted: false,
    preset: {
      ...value,
      ...input,
      cover_asset_id: input.reference_asset_ids[0],
      reference_items: input.reference_asset_ids.map((assetID: string) =>
        references.find((item) => item.asset_id === assetID),
      ),
    },
  }));
  const saved = vi.fn();
  render(
    <StyleEditorDialog
      owner={owner}
      value={{
        ...value,
        reference_asset_ids: references.map((item) => item.asset_id),
        reference_items: references,
        cover_asset_id: references[0]!.asset_id,
        cover_item_id: references[0]!.id,
      }}
      onSaved={saved}
      onClose={() => {}}
    />,
  );
  expect(screen.queryByRole("button", { name: "添加参考图" })).toBeNull();
  fireEvent.click(
    screen.getByRole("button", { name: "设为封面第 3 张参考图" }),
  );
  fireEvent.click(screen.getByRole("button", { name: "移除第 2 张参考图" }));
  fireEvent.click(screen.getByRole("button", { name: "保存风格" }));
  await waitFor(() => expect(saved).toHaveBeenCalled());
  expect(saved.mock.calls[0][0].cover_asset_id).toBe(references[2]!.asset_id);
  expect(saved.mock.calls[0][0].reference_asset_ids).toEqual([
    references[2]!.asset_id,
    references[1]!.asset_id,
    references[3]!.asset_id,
  ]);
});
it("uses the persisted snapshot only after explicitly saving and using a newly created style", async () => {
  api.create.mockImplementation(async (input, key) => ({
    request_id: key,
    action: "create",
    deleted: false,
    preset: { ...value, ...input },
  }));
  const change = await openLibrary();
  fireEvent.click(screen.getByRole("button", { name: "创建风格" }));
  fireEvent.change(await screen.findByRole("textbox", { name: "风格名称" }), {
    target: { value: "新风格" },
  });
  fireEvent.change(screen.getByRole("textbox", { name: "风格描述" }), {
    target: { value: "保留胶片颗粒。" },
  });
  expect(change).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole("button", { name: "保存并使用" }));
  await waitFor(() => expect(change).toHaveBeenCalledTimes(1));
  expect(change.mock.calls[0][0]).toMatchObject({
    name: "新风格",
    prompt_fragment: "保留胶片颗粒。",
  });
  expect(Object.isFrozen(change.mock.calls[0][0])).toBe(true);
});
it("does not mutate the already selected draft when its library style is edited", async () => {
  api.update.mockImplementation(async (_id, input, key) => ({
    request_id: key,
    action: "patch",
    deleted: false,
    preset: { ...value, ...input },
  }));
  const change = vi.fn();
  render(<StyleLibraryPopover owner={owner} value={value} onChange={change} />);
  fireEvent.click(screen.getByRole("button", { name: "选择风格" }));
  fireEvent.click(await screen.findByRole("button", { name: "编辑水墨" }));
  fireEvent.change(await screen.findByRole("textbox", { name: "风格描述" }), {
    target: { value: "修改后的库内容。" },
  });
  fireEvent.click(screen.getByRole("button", { name: "保存风格" }));
  await screen.findByRole("button", { name: "选择水墨" });
  expect(change).not.toHaveBeenCalled();
  expect(value.prompt_fragment).toBe("疏朗的水墨构图");
});
