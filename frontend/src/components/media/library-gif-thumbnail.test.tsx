import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { LibraryItems } from "./library-items";
import * as queries from "./library-queries";

vi.mock("./library-queries", async (original) => ({
  ...(await original<typeof import("./library-queries")>()),
  freshLibrary: vi.fn(),
  previewLibrary: vi.fn(),
}));
class VisibleObserver {
  static current: VisibleObserver;
  constructor(readonly callback: IntersectionObserverCallback) {
    VisibleObserver.current = this;
  }
  observe() {
    this.show(true);
  }
  disconnect() {}
  show(isIntersecting: boolean) {
    this.callback(
      [{ isIntersecting } as IntersectionObserverEntry],
      this as unknown as IntersectionObserver,
    );
  }
}
afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});
it("GIF列表只挂真实首帧PNG缩略图，完整GIF留给详情且离屏释放", async () => {
  vi.stubGlobal("IntersectionObserver", VisibleObserver);
  vi.spyOn(document, "hidden", "get").mockReturnValue(false);
  const identity = {
    origin: window.location.origin,
    actorId: "11111111-1111-4111-8111-111111111111",
    orgId: "22222222-2222-4222-8222-222222222222",
    libraryId: "33333333-3333-5333-8333-333333333333",
    scope: { kind: "personal" as const },
  };
  const asset = {
    id: "44444444-4444-4444-8444-444444444444",
    kind: "image" as const,
    file_name: "three-colors.gif",
    mime_type: "image/gif",
    byte_size: 339,
    width: 64,
    height: 64,
    duration_ms: null,
    revision: 1,
  };
  const time = new Date().toISOString();
  const item = {
    id: asset.id,
    asset_id: asset.id,
    folder_id: null,
    kind: "image" as const,
    title: "三帧原件",
    category: "material" as const,
    tags: [],
    source_label: "",
    note: "",
    favorite: false,
    catalog_state: "active" as const,
    trashed_at: null,
    position: 0,
    revision: 1,
    created_at: time,
    updated_at: time,
    media: asset,
  };
  vi.mocked(queries.freshLibrary).mockResolvedValue(
    {} as Awaited<ReturnType<typeof queries.freshLibrary>>,
  );
  const expires = new Date(Date.now() + 60000).toISOString();
  vi.mocked(queries.previewLibrary).mockResolvedValue({
    asset,
    url: "https://private.invalid/whole.gif",
    expires_at: expires,
    renditions: [
      {
        kind: "thumb_256",
        url: "https://private.invalid/first-frame.png",
        expires_at: expires,
        width: 64,
        height: 64,
      },
    ],
  });
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0 } },
  });
  const view = render(
    <QueryClientProvider client={client}>
      <LibraryItems
        identity={identity}
        items={[item]}
        view="grid"
        selection={[]}
        disabled={false}
        readOnly={false}
        onToggle={vi.fn()}
        onView={vi.fn()}
        onEdit={vi.fn()}
        onCopyText={vi.fn()}
      />
    </QueryClientProvider>,
  );
  const image = await screen.findByRole("img", { name: item.title });
  expect(image.getAttribute("src")).toBe(
    "https://private.invalid/first-frame.png",
  );
  expect(queries.freshLibrary).toHaveBeenCalledOnce();
  act(() => VisibleObserver.current.show(false));
  expect(image.hasAttribute("src")).toBe(false);
  expect(screen.queryByRole("img")).toBeNull();
  view.unmount();
  client.clear();
});
