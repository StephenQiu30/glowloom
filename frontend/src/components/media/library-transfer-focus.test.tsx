import {
  act,
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
import { defaultLibraryFilter } from "./library-model";
import * as transfers from "@/components/library/transfer-queries";
import { saveTransferIntent } from "@/components/library/transfer-intent";
import {
  transferIdentity,
  transferInput,
  transferJob,
  transferIDs,
  transferLibrary,
} from "@/components/library/transfer-test-fixtures";

const { replace } = vi.hoisted(() => ({ replace: vi.fn() }));
vi.mock("@/hooks/use-mobile", () => ({ useIsMobile: () => false }));
vi.mock("next/navigation", () => ({
  useSearchParams: () =>
    new URLSearchParams(`scope=personal&asset_id=${transferIDs.item}`),
  usePathname: () => "/assets",
  useRouter: () => ({ replace }),
}));
vi.mock("./library-queries", async (original) => ({
  ...(await original<typeof import("./library-queries")>()),
  listLibrary: vi.fn(),
  freshLibrary: vi.fn(),
  getLibraryDetail: vi.fn(),
}));
vi.mock("@/components/project/queries", () => ({
  listProjects: vi.fn().mockResolvedValue({ items: [], next_cursor: null }),
  getProject: vi.fn(),
}));
vi.mock("@/components/library/transfer-queries", async (original) => ({
  ...(await original<typeof import("@/components/library/transfer-queries")>()),
  listTransfers: vi.fn(),
  getTransfer: vi.fn(),
  runTransferIntent: vi.fn(),
}));
const detail = {
  id: transferIDs.item,
  asset_id: null,
  folder_id: null,
  kind: "text" as const,
  title: "保留URL的完整正文",
  category: "material" as const,
  tags: [],
  source_label: "",
  note: "",
  favorite: false,
  catalog_state: "active" as const,
  trashed_at: null,
  position: 0,
  revision: 1,
  created_at: transferJob.created_at,
  updated_at: transferJob.updated_at,
  plain_text: "原正文仍完整保留。",
};
beforeEach(() => {
  vi.clearAllMocks();
  sessionStorage.clear();
  for (const name of ["ResizeObserver", "IntersectionObserver"])
    vi.stubGlobal(
      name,
      class {
        observe() {}
        unobserve() {}
        disconnect() {}
      },
    );
  vi.mocked(library.listLibrary).mockResolvedValue(transferLibrary);
  vi.mocked(library.freshLibrary).mockResolvedValue(transferLibrary);
  vi.mocked(library.getLibraryDetail).mockResolvedValue(detail);
  vi.mocked(transfers.listTransfers).mockResolvedValue({
    current_actor_id: transferIDs.actor,
    current_org_id: transferIDs.org,
    page: 1,
    page_size: 20,
    items: [],
  });
  vi.mocked(transfers.getTransfer).mockResolvedValue(transferJob);
  vi.mocked(transfers.runTransferIntent).mockResolvedValue(transferJob);
});
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});
function mount() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <LibraryBrowser
        identity={transferIdentity}
        initial={transferLibrary}
        filter={defaultLibraryFilter(transferIdentity.scope)}
        selected={transferIDs.item}
        view="grid"
        refreshContext={vi.fn().mockResolvedValue(transferLibrary)}
      />
    </QueryClientProvider>,
  );
}
it("详情URL刷新遇到未确认迁移时只保留一个focus trap，原键受理后恢复原详情和焦点", async () => {
  saveTransferIntent(sessionStorage, {
    ...transferIdentity,
    version: 1,
    key: transferIDs.project,
    action: "create",
    body: transferInput,
  });
  mount();
  await screen.findByRole("dialog", { name: "素材迁移任务" });
  await screen.findByText(`原键 ${transferIDs.project}`);
  expect(screen.getAllByRole("dialog", { hidden: true })).toHaveLength(1);
  expect(library.getLibraryDetail).not.toHaveBeenCalled();
  expect(replace).not.toHaveBeenCalled();
  expect(transfers.runTransferIntent).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole("button", { name: "使用原键与原正文核验" }));
  const restored = await screen.findByRole("dialog", { name: detail.title });
  await waitFor(() =>
    expect(restored.contains(document.activeElement)).toBe(true),
  );
  expect(screen.getAllByRole("dialog", { hidden: true })).toHaveLength(1);
  expect(screen.getByTestId("library-plain-text").textContent).toBe(
    detail.plain_text,
  );
  expect(replace).not.toHaveBeenCalled();
});
it("scopefresh核验完成前不抢开详情，确认无迁移意图后按原URL打开", async () => {
  let release!: () => void;
  const pending = new Promise<typeof transferLibrary>((resolve) => {
    release = () => resolve(transferLibrary);
  });
  vi.mocked(library.freshLibrary).mockReturnValue(pending);
  mount();
  await waitFor(() => expect(library.freshLibrary).toHaveBeenCalled());
  expect(screen.queryAllByRole("dialog", { hidden: true })).toHaveLength(0);
  expect(library.getLibraryDetail).not.toHaveBeenCalled();
  await act(async () => release());
  await screen.findByRole("dialog", { name: detail.title });
  expect(screen.getAllByRole("dialog", { hidden: true })).toHaveLength(1);
  expect(replace).not.toHaveBeenCalled();
});
