import { beforeEach, expect, it, vi } from "vitest";
import { reviewLibraryPurge } from "./library-purge-query";
import * as library from "./library-queries";
import {
  transferIdentity as identity,
  transferLibrary,
  transferIDs as ids,
  transferProject,
} from "@/components/library/transfer-test-fixtures";
import * as projects from "@/components/project/queries";
vi.mock("./library-queries", () => ({
  listLibrary: vi.fn(),
  freshLibrary: vi.fn(),
  getLibraryDetail: vi.fn(),
}));
vi.mock("@/components/project/queries", () => ({ getProject: vi.fn() }));
const makeItem = (id: string) => ({
  id,
  asset_id: null,
  kind: "text" as const,
  title: id,
  revision: 2,
  catalog_state: "trashed" as const,
  folder_id: null,
  category: "material" as const,
  tags: [],
  source_label: "",
  note: "",
  favorite: false,
  position: 0,
  trashed_at: "2026-10-02T00:00:00Z",
  created_at: "2026-10-02T00:00:00Z",
  updated_at: "2026-10-02T00:00:00Z",
});
beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(library.freshLibrary).mockResolvedValue({
    ...transferLibrary,
    revision: 4,
  });
  vi.mocked(projects.getProject).mockResolvedValue(transferProject);
});
it("清空全部读取无筛选整个回收站多页，200预算内冻结全序CAS，末尾再核版本", async () => {
  const items = Array.from({ length: 121 }, (_, i) =>
    makeItem(`10000000-0000-4000-8000-${String(i + 1).padStart(12, "0")}`),
  );
  vi.mocked(library.listLibrary).mockImplementation(async (_, f) => ({
    ...transferLibrary,
    revision: 4,
    page: f!.page,
    page_size: f!.page_size,
    total: 121,
    items: f!.page === 1 ? items.slice(0, 120) : items.slice(120),
  }));
  const result = await reviewLibraryPurge(identity);
  expect(result.items).toHaveLength(121);
  expect(result.items[120].id).toBe(items[120].id);
  expect(library.listLibrary).toHaveBeenCalledTimes(2);
  expect(library.listLibrary).toHaveBeenCalledWith(
    identity.scope,
    expect.objectContaining({
      catalog_state: "trashed",
      folder: "all",
      search: "",
      kind: "",
      category: "",
      page: 2,
    }),
    undefined,
    identity,
  );
  expect(library.freshLibrary).toHaveBeenCalledTimes(2);
});
it("超2000全部、分页revision变更、重复/缺项均停止，不能部分清空或接受新事实", async () => {
  for (const variant of ["budget", "revision", "duplicate", "missing"]) {
    vi.mocked(library.listLibrary).mockImplementation(async (_, f) => ({
      ...transferLibrary,
      revision: f!.page === 2 && variant === "revision" ? 5 : 4,
      page: f!.page,
      page_size: f!.page_size,
      total: variant === "budget" ? 2001 : 121,
      items:
        f!.page === 1
          ? Array.from({ length: 120 }, (_, i) =>
              makeItem(
                `10000000-0000-4000-8000-${String(i + 1).padStart(12, "0")}`,
              ),
            )
          : variant === "missing"
            ? []
            : [
                makeItem(
                  variant === "duplicate"
                    ? "10000000-0000-4000-8000-000000000001"
                    : ids.target,
                ),
              ],
    }));
    await expect(reviewLibraryPurge(identity)).rejects.toThrow();
  }
});
it("所选全量读取详情且逐项版本一致，project当前revision拒归档", async () => {
  vi.mocked(library.getLibraryDetail).mockResolvedValue({
    ...makeItem(ids.item),
    plain_text: "text",
    media: null,
  });
  const projectIdentity = {
    ...identity,
    scope: { kind: "project" as const, project_id: ids.project },
  };
  const result = await reviewLibraryPurge(projectIdentity, [
    { id: ids.item, revision: 2 },
  ]);
  expect(result.projectRevision).toBe(1);
  vi.mocked(library.getLibraryDetail).mockResolvedValue({
    ...makeItem(ids.item),
    revision: 3,
    plain_text: "text",
    media: null,
  });
  await expect(
    reviewLibraryPurge(projectIdentity, [{ id: ids.item, revision: 2 }]),
  ).rejects.toThrow();
  vi.mocked(projects.getProject).mockResolvedValue({
    ...transferProject,
    status: "archived",
  });
  await expect(
    reviewLibraryPurge(projectIdentity, [{ id: ids.item, revision: 2 }]),
  ).rejects.toThrow();
});
