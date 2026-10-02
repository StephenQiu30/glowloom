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
import { saveTransferIntent } from "@/components/library/transfer-intent";
import { defaultLibraryFilter, type LibraryPage } from "./library-model";
import {
  transferIdentity,
  transferInput,
  transferJob,
  transferIDs,
  transferLibrary,
  transferProjectSummary,
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
  getLibraryDetail: vi.fn(),
}));
vi.mock("@/components/project/queries", () => ({
  listProjects: vi.fn(),
  getProject: vi.fn(),
}));
vi.mock("@/components/library/transfer-queries", async (original) => ({
  ...(await original<typeof import("@/components/library/transfer-queries")>()),
  listTransfers: vi.fn(),
  getTransfer: vi.fn(),
  runTransferIntent: vi.fn(),
}));
const item = (id: string, title: string, revision: number) => ({
  id,
  asset_id: null,
  folder_id: null,
  kind: "text" as const,
  title,
  category: "material" as const,
  tags: [],
  source_label: "",
  note: "",
  favorite: false,
  catalog_state: "active" as const,
  trashed_at: null,
  position: 0,
  revision,
  created_at: transferJob.created_at,
  updated_at: transferJob.updated_at,
});
const first = item(transferIDs.item, "第一页完整正文", 1),
  second = item(transferIDs.folder, "第二页完整正文", 2);
const page: LibraryPage = {
  ...transferLibrary,
  page_size: 40,
  revision: 3,
  total: 80,
  items: [first],
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
  vi.mocked(library.listLibrary).mockImplementation(async (_, query) => ({
    ...page,
    page: query?.page ?? 1,
    page_size: query?.page_size ?? 40,
    items: query?.page === 2 ? [second] : [first],
  }));
  vi.mocked(projects.listProjects).mockResolvedValue({
    items: [transferProjectSummary],
    next_cursor: null,
  });
  vi.mocked(transfers.listTransfers).mockResolvedValue({
    current_actor_id: transferIDs.actor,
    current_org_id: transferIDs.org,
    page: 1,
    page_size: 20,
    items: [],
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
        identity={transferIdentity}
        initial={page}
        filter={{
          ...defaultLibraryFilter(transferIdentity.scope),
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
it("正式Library入口跨页保留选中UUID/CAS全序，动态表单审阅全部选中，不造当前页迁移", async () => {
  const mounted = mount();
  fireEvent.click(
    await screen.findByRole("checkbox", { name: `选择 ${first.title}` }),
  );
  mounted.rerender(mounted.tree(2));
  fireEvent.click(
    await screen.findByRole("checkbox", { name: `选择 ${second.title}` }),
  );
  const trigger = screen.getByRole("button", { name: "加入项目素材库" });
  await waitFor(() => expect(trigger.matches(":disabled")).toBe(false));
  trigger.focus();
  fireEvent.click(trigger);
  await screen.findByRole("dialog", { name: "素材迁移任务" });
  const full = await screen.findByLabelText("全部所选素材ID与冻结版本");
  expect(full).toHaveProperty(
    "value",
    `${first.id} · 条目版本 1\n${second.id} · 条目版本 2`,
  );
  expect(transfers.runTransferIntent).not.toHaveBeenCalled();
  fireEvent.keyDown(screen.getByRole("dialog"), { key: "Escape" });
  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  await waitFor(() => expect(document.activeElement).toBe(trigger));
});
it("已main素材页跨刷新unknown自动恢复，原正文保留，scope切换/目录改动阻断，不自动写", async () => {
  const original = {
    ...transferIdentity,
    version: 1 as const,
    key: transferIDs.project,
    action: "create" as const,
    body: { ...transferInput, expected_source_revision: 3 },
  };
  saveTransferIntent(sessionStorage, original);
  mount();
  await screen.findByRole("dialog", { name: "素材迁移任务" });
  await screen.findByText(`原键 ${original.key}`);
  expect(
    screen
      .getByRole("button", { name: "新建目录", hidden: true })
      .matches(":disabled"),
  ).toBe(true);
  expect(screen.getByLabelText("素材库范围").matches(":disabled")).toBe(true);
  expect(transfers.runTransferIntent).not.toHaveBeenCalled();
});
