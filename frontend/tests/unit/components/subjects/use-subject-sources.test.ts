import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { useSubjectSources } from "@/components/subjects/use-subject-sources";
const api = vi.hoisted(() => ({ list: vi.fn() }));
vi.mock("@/api/subjects", () => ({ listSubjectSources: api.list }));
const actor = "11111111-1111-4111-8111-111111111111";
const scope = "22222222-2222-4222-8222-222222222222";
function item(index: number) {
  const id = `33333333-3333-4333-8333-${String(index).padStart(12, "0")}`;
  return {
    id,
    source: "personal_asset",
    title: `素材${index}`,
    kind: "image",
    revision: 1,
    favorite: false,
    upscaled: false,
    created_at: "2026-10-28T00:00:00Z",
    updated_at: "2026-10-28T00:00:00Z",
    media: {
      id,
      revision: 1,
      kind: "image",
      file_name: `${index}.png`,
      mime_type: "image/png",
      byte_size: 20,
      origin: "upload",
    },
  };
}
function batch(page: number, items = [item(page)], total = 61) {
  return {
    current_actor_id: actor,
    current_scope_id: scope,
    page,
    page_size: 30,
    items,
    total,
    capabilities: { upscaled_filter: false },
  };
}
beforeEach(() => vi.resetAllMocks());
afterEach(cleanup);
it("分批追加、ID去重、同帧只请求一次，并在total耗尽后停止", async () => {
  api.list.mockImplementation(async ({ page }) =>
    batch(page, page === 2 ? [item(1), item(2)] : [item(page)]),
  );
  const { result } = renderHook(() =>
    useSubjectSources(actor, scope, "image", "all"),
  );
  await waitFor(() => expect(result.current.page?.page).toBe(1));
  act(() => {
    result.current.loadMore();
    result.current.loadMore();
    result.current.loadMore();
  });
  await waitFor(() => expect(result.current.page?.page).toBe(2));
  expect(api.list).toHaveBeenCalledTimes(2);
  expect(result.current.page?.items.map((v) => v.title)).toEqual([
    "素材1",
    "素材2",
  ]);
  act(() => result.current.loadMore());
  await waitFor(() => expect(result.current.page?.page).toBe(3));
  expect(result.current.page?.items.map((v) => v.title)).toEqual([
    "素材1",
    "素材2",
    "素材3",
  ]);
  expect(result.current.hasMore).toBe(false);
  act(() => result.current.loadMore());
  expect(api.list).toHaveBeenCalledTimes(3);
});
it("重复批次按已消耗页推进，空批立即停止而不循环", async () => {
  api.list.mockImplementation(async ({ page }) =>
    batch(page, page === 3 ? [] : [item(1)], 120),
  );
  const { result } = renderHook(() =>
    useSubjectSources(actor, scope, "image", "all"),
  );
  await waitFor(() => expect(result.current.page?.page).toBe(1));
  act(() => result.current.loadMore());
  await waitFor(() => expect(result.current.page?.page).toBe(2));
  expect(result.current.hasMore).toBe(true);
  await waitFor(() => expect(result.current.page?.items).toHaveLength(1));
  act(() => result.current.loadMore());
  await waitFor(() => expect(result.current.page?.page).toBe(3));
  expect(result.current.hasMore).toBe(false);
});
it("尾批错误保留前批、停止自动加载，重试同一页且不重复", async () => {
  api.list
    .mockResolvedValueOnce(batch(1))
    .mockRejectedValueOnce(new Error("offline"))
    .mockResolvedValueOnce(batch(2, [item(2)], 31));
  const { result } = renderHook(() =>
    useSubjectSources(actor, scope, "image", "all"),
  );
  await waitFor(() => expect(result.current.page?.page).toBe(1));
  act(() => result.current.loadMore());
  await waitFor(() => expect(result.current.error).toBeTruthy());
  expect(result.current.page?.items).toHaveLength(1);
  act(() => result.current.loadMore());
  expect(api.list).toHaveBeenCalledTimes(2);
  act(() => result.current.retry());
  await waitFor(() => expect(result.current.page?.page).toBe(2));
  expect(api.list.mock.calls.map(([query]) => query.page)).toEqual([1, 2, 2]);
  expect(result.current.page?.items).toHaveLength(2);
  expect(result.current.error).toBeUndefined();
});
it.each(["resolve", "reject"])(
  "分组切换中止旧请求并忽略迟到的%s",
  async (outcome) => {
    let resolve!: (value: unknown) => void, reject!: (error: Error) => void;
    api.list
      .mockImplementationOnce(
        () =>
          new Promise((yes, no) => {
            resolve = yes;
            reject = no;
          }),
      )
      .mockResolvedValue(batch(1, [item(2)], 1));
    const { result, rerender } = renderHook(
      ({ group }) => useSubjectSources(actor, scope, "image", group),
      { initialProps: { group: "all" } },
    );
    const signal = api.list.mock.calls[0][1].signal;
    rerender({ group: "favorite" });
    expect(signal.aborted).toBe(true);
    await waitFor(() =>
      expect(result.current.page?.items[0].title).toBe("素材2"),
    );
    await act(async () => {
      if (outcome === "resolve") resolve(batch(1));
      else reject(new Error("stale"));
    });
    expect(result.current.page?.items[0].title).toBe("素材2");
    expect(result.current.error).toBeUndefined();
  },
);
it("离开已追加分组再返回从首批开始，不复用旧page位置", async () => {
  api.list.mockImplementation(async ({ page }) => batch(page));
  const { result, rerender } = renderHook(
    ({ group }) => useSubjectSources(actor, scope, "image", group),
    { initialProps: { group: "all" } },
  );
  await waitFor(() => expect(result.current.page?.page).toBe(1));
  act(() => result.current.loadMore());
  await waitFor(() => expect(result.current.page?.page).toBe(2));
  rerender({ group: "favorite" });
  await waitFor(() => expect(result.current.page?.page).toBe(1));
  rerender({ group: "all" });
  await waitFor(() => expect(api.list).toHaveBeenCalledTimes(4));
  expect(api.list.mock.calls.at(-1)![0].page).toBe(1);
  await waitFor(() => expect(result.current.page?.items).toHaveLength(1));
});
it("错误分页回执不进入列表，卸载取消未完成请求", async () => {
  api.list.mockResolvedValue(batch(2));
  const { result, unmount } = renderHook(() =>
    useSubjectSources(actor, scope, "image", "all"),
  );
  await waitFor(() => expect(result.current.error).toBeTruthy());
  expect(result.current.page).toBeUndefined();
  const signal = api.list.mock.calls[0][1].signal;
  unmount();
  expect(signal.aborted).toBe(true);
});
