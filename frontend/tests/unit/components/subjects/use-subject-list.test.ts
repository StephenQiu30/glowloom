import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { useSubjectList } from "@/components/subjects/use-subject-list";
import type { SubjectFilter } from "@/components/subjects/unified-toolbar";

const api = vi.hoisted(() => ({ list: vi.fn() }));
vi.mock("@/api/subjects", () => ({ listSubjects: api.list }));
const actorID = "11111111-1111-4111-8111-111111111111";
const scopeID = "22222222-2222-4222-8222-222222222222";
const all: SubjectFilter = {
  favorite: false,
  order: "updated_desc",
  from: "",
  through: "",
};
const favorites: SubjectFilter = { ...all, favorite: true };

function batch(page: number, favorite = false) {
  return {
    current_actor_id: actorID,
    current_scope_id: scopeID,
    page,
    page_size: 20,
    total: favorite ? 1 : 21,
    items: [
      {
        id: `33333333-3333-4333-8333-${String(favorite ? 3 : page).padStart(12, "0")}`,
        owner_scope_id: scopeID,
        revision: 1,
        version_id: "44444444-4444-4444-8444-444444444444",
        name: favorite ? "收藏主体" : `第${page}批主体`,
        description: "",
        primary: {
          binding_id: "55555555-5555-4555-8555-555555555555",
          asset_id: "66666666-6666-4666-8666-666666666666",
          asset_revision: 1,
          kind: "image",
          mime_type: "image/png",
          file_name: "主体.png",
        },
        references: [],
        favorite,
        created_at: "2026-10-28T00:00:00Z",
        updated_at: "2026-10-28T00:00:00Z",
      },
    ],
    capabilities: { intelligent_voice: { available: false } },
  };
}

afterEach(() => {
  cleanup();
  vi.resetAllMocks();
});

it("已加载第二批后切收藏再返回，从第一批重读且不混入旧批次", async () => {
  api.list.mockImplementation(async ({ page, favorite }) =>
    batch(page, !!favorite),
  );
  const { result, rerender } = renderHook(
    ({ filter }) => useSubjectList(actorID, scopeID, "", filter),
    { initialProps: { filter: all } },
  );
  await waitFor(() => expect(result.current.page?.page).toBe(1));
  act(() => result.current.loadMore());
  await waitFor(() => expect(result.current.page?.page).toBe(2));
  expect(result.current.page?.items.map((item) => item.name)).toEqual([
    "第1批主体",
    "第2批主体",
  ]);

  rerender({ filter: favorites });
  expect(result.current.page).toBeUndefined();
  await waitFor(() =>
    expect(result.current.page?.items[0].name).toBe("收藏主体"),
  );
  rerender({ filter: all });
  expect(result.current.page).toBeUndefined();
  expect(result.current.loading).toBe(true);
  await waitFor(() => expect(api.list).toHaveBeenCalledTimes(4));
  await waitFor(() =>
    expect(result.current.page?.items.map((item) => item.name)).toEqual([
      "第1批主体",
    ]),
  );
  expect(
    api.list.mock.calls.map(([query]) => [query.favorite ?? false, query.page]),
  ).toEqual([
    [false, 1],
    [false, 2],
    [true, 1],
    [false, 1],
  ]);
  expect(result.current.hasMore).toBe(true);
  act(() => result.current.loadMore());
  await waitFor(() => expect(result.current.page?.page).toBe(2));
  expect(result.current.page?.items.map((item) => item.name)).toEqual([
    "第1批主体",
    "第2批主体",
  ]);
});

it("收藏请求尚未完成就返回时也从第一批重读，丢弃迟到的收藏回执", async () => {
  let resolveFavorite!: (value: unknown) => void;
  api.list.mockImplementation(({ page, favorite }) =>
    favorite
      ? new Promise((resolve) => {
          resolveFavorite = resolve;
        })
      : Promise.resolve(batch(page)),
  );
  const { result, rerender } = renderHook(
    ({ filter }) => useSubjectList(actorID, scopeID, "", filter),
    { initialProps: { filter: all } },
  );
  await waitFor(() => expect(result.current.page?.page).toBe(1));
  act(() => result.current.loadMore());
  await waitFor(() => expect(result.current.page?.page).toBe(2));
  rerender({ filter: favorites });
  const favoriteSignal = api.list.mock.calls[2][1].signal;
  rerender({ filter: all });
  expect(favoriteSignal.aborted).toBe(true);
  await waitFor(() => expect(result.current.page?.page).toBe(1));
  await act(async () => resolveFavorite(batch(1, true)));
  expect(result.current.page?.items.map((item) => item.name)).toEqual([
    "第1批主体",
  ]);
  expect(api.list.mock.calls.at(-1)![0]).toMatchObject({
    favorite: undefined,
    page: 1,
    page_size: 20,
  });
  expect(result.current.error).toBeUndefined();
});
