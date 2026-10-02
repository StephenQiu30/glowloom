import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { LibraryBrowser } from "./library-browser";
import * as library from "./library-queries";
import * as projects from "@/components/project/queries";
import * as transfers from "@/components/library/transfer-queries";
import * as purges from "./library-purge-query";
import * as storage from "./library-storage-query";
import { defaultLibraryFilter } from "./library-model";
import {
  purgeTestIdentity as identity,
  purgeTestReview as review,
  purgeTestJob as job,
} from "./library-purge-fixtures";
import {
  transferLibrary,
  transferIDs as ids,
} from "@/components/library/transfer-test-fixtures";
vi.mock("next/navigation", () => ({
  useSearchParams: () => new URLSearchParams(),
  usePathname: () => "/assets",
  useRouter: () => ({ replace: vi.fn() }),
}));
vi.mock("./library-queries", async (original) => ({
  ...(await original<typeof import("./library-queries")>()),
  listLibrary: vi.fn(),
  freshLibrary: vi.fn(),
}));
vi.mock("@/components/project/queries", () => ({
  listProjects: vi.fn(),
  getProject: vi.fn(),
}));
vi.mock("@/components/library/transfer-queries", async (original) => ({
  ...(await original<typeof import("@/components/library/transfer-queries")>()),
  listTransfers: vi.fn(),
}));
vi.mock("./library-purge-query", () => ({
  reviewLibraryPurge: vi.fn(),
  listLibraryPurges: vi.fn(),
  getLibraryPurge: vi.fn(),
  runLibraryPurge: vi.fn(),
}));
vi.mock("./library-storage-query", () => ({ getLibraryStorageUsage: vi.fn() }));
const item = {
  id: ids.item,
  asset_id: null,
  folder_id: null,
  kind: "text" as const,
  title: "第一页回收正文",
  category: "material" as const,
  tags: [],
  source_label: "",
  note: "",
  favorite: false,
  catalog_state: "trashed" as const,
  trashed_at: job.created_at,
  position: 0,
  revision: 2,
  created_at: job.created_at,
  updated_at: job.updated_at,
};
const page = {
  ...transferLibrary,
  revision: 4,
  page_size: 40,
  total: 80,
  items: [item],
};
beforeEach(() => {
  vi.clearAllMocks();
  sessionStorage.clear();
  vi.stubGlobal(
    "ResizeObserver",
    class {
      observe() {}
      unobserve() {}
      disconnect() {}
    },
  );
  vi.stubGlobal(
    "IntersectionObserver",
    class {
      observe() {}
      unobserve() {}
      disconnect() {}
    },
  );
  vi.mocked(library.freshLibrary).mockResolvedValue({ ...page, page_size: 1 });
  vi.mocked(library.listLibrary).mockImplementation(async (_, filter) => ({
    ...page,
    page: filter?.page ?? 1,
    page_size: filter?.page_size ?? 40,
    items:
      filter?.page === 2
        ? [{ ...item, id: ids.folder, title: "第二页回收正文", revision: 3 }]
        : [item],
  }));
  vi.mocked(projects.listProjects).mockResolvedValue({
    items: [],
    next_cursor: null,
  });
  vi.mocked(transfers.listTransfers).mockResolvedValue({
    current_actor_id: ids.actor,
    current_org_id: ids.org,
    page: 1,
    page_size: 20,
    items: [],
  });
  vi.mocked(purges.reviewLibraryPurge).mockResolvedValue(review);
  vi.mocked(purges.listLibraryPurges).mockResolvedValue([]);
  vi.mocked(storage.getLibraryStorageUsage).mockResolvedValue({
    current_actor_id: ids.actor,
    current_org_id: ids.org,
    library_id: identity.libraryId,
    scope: identity.scope,
    used_bytes: 8192,
    object_count: 2,
    limit_bytes: null,
    calculated_at: job.created_at,
  });
});
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});
function mount() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  const tree = (number: number) => (
    <QueryClientProvider client={client}>
      <LibraryBrowser
        identity={identity}
        initial={page}
        filter={{
          ...defaultLibraryFilter(identity.scope),
          catalog_state: "trashed",
          page: number,
        }}
        selected={null}
        view="grid"
        refreshContext={vi.fn().mockResolvedValue(page)}
      />
    </QueryClientProvider>
  );
  return { tree, ...render(tree(1)) };
}
it("正式主素材页显示真实容量，跨页所选永久删除审阅全部CAS且点击入口不发送", async () => {
  const surface = mount();
  await screen.findByText(/已占用 8 KiB.*未配置容量上限/);
  fireEvent.click(
    await screen.findByRole("checkbox", { name: "选择 第一页回收正文" }),
  );
  surface.rerender(surface.tree(2));
  fireEvent.click(
    await screen.findByRole("checkbox", { name: "选择 第二页回收正文" }),
  );
  const trigger = screen.getByRole("button", { name: "永久删除所选素材" });
  await waitFor(() => expect(trigger.matches(":disabled")).toBe(false));
  trigger.focus();
  fireEvent.click(trigger);
  await screen.findByRole("dialog", { name: "永久清理与任务记录" });
  await waitFor(() =>
    expect(purges.reviewLibraryPurge).toHaveBeenCalledWith(
      identity,
      [
        { id: ids.item, revision: 2 },
        { id: ids.folder, revision: 3 },
      ],
      expect.any(AbortSignal),
    ),
  );
  expect(purges.runLibraryPurge).not.toHaveBeenCalled();
  fireEvent.keyDown(screen.getByRole("dialog"), { key: "Escape" });
  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  await waitFor(() => expect(document.activeElement).toBe(trigger));
});
it("正式清空入口传全部scope而不是当前页、选中列表或筛选集合，未确认不删除", async () => {
  mount();
  const trigger = await screen.findByRole("button", { name: "清空整个回收站" });
  await waitFor(() => expect(trigger.matches(":disabled")).toBe(false));
  fireEvent.click(trigger);
  await screen.findByLabelText("全部永久清理条目及冻结版本");
  expect(purges.reviewLibraryPurge).toHaveBeenCalledWith(
    identity,
    undefined,
    expect.any(AbortSignal),
  );
  expect(purges.runLibraryPurge).not.toHaveBeenCalled();
  expect(screen.getByLabelText("素材库范围").matches(":disabled")).toBe(true);
});
