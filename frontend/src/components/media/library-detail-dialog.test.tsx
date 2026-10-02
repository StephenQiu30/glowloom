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
import { LibraryDetailDialog } from "./library-detail-dialog";
import * as queries from "./library-queries";
import { ApiError } from "@/lib/request";
vi.mock("./library-queries", async (original) => ({
  ...(await original<typeof import("./library-queries")>()),
  freshLibrary: vi.fn(),
  getLibraryDetail: vi.fn(),
  downloadLibraryOriginal: vi.fn(),
  previewLibrary: vi.fn(),
}));
const id = "44444444-4444-4444-8444-444444444444",
  time = "2026-10-02T00:00:00Z";
const identity = {
  origin: window.location.origin,
  actorId: "11111111-1111-4111-8111-111111111111",
  orgId: "22222222-2222-4222-8222-222222222222",
  libraryId: "33333333-3333-5333-8333-333333333333",
  scope: { kind: "personal" as const },
};
const detail = {
  id,
  asset_id: null,
  folder_id: null,
  kind: "text" as const,
  title: "精确文本",
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
  plain_text: "  原始\r\n正文🙂  ",
};
beforeEach(() => {
  vi.resetAllMocks();
  vi.mocked(queries.freshLibrary).mockResolvedValue(
    {} as Awaited<ReturnType<typeof queries.freshLibrary>>,
  );
  vi.mocked(queries.getLibraryDetail).mockResolvedValue(detail);
});
afterEach(cleanup);
function mount(
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } }),
) {
  return render(
    <QueryClientProvider client={client}>
      <LibraryDetailDialog
        identity={identity}
        id={id}
        readOnly={false}
        onClose={vi.fn()}
        onEdit={vi.fn()}
      />
    </QueryClientProvider>,
  );
}

it("GIF详情使用完整原件而非首帧PNG，当前授权失败不挂原件，关闭释放来源", async () => {
  const media = {
    id,
    kind: "image" as const,
    file_name: "three-colors.gif",
    mime_type: "image/gif",
    byte_size: 339,
    width: 64,
    height: 64,
    duration_ms: null,
    revision: 1,
  };
  vi.mocked(queries.getLibraryDetail).mockResolvedValue({
    ...detail,
    kind: "image",
    asset_id: id,
    media,
    plain_text: undefined,
  });
  const expires = new Date(Date.now() + 60000).toISOString();
  vi.mocked(queries.previewLibrary).mockResolvedValue({
    asset: media,
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
  const view = mount();
  const image = await screen.findByRole("img", { name: detail.title });
  expect(image.getAttribute("src")).toBe("https://private.invalid/whole.gif");
  expect(image.getAttribute("crossorigin")).toBe("anonymous");
  expect(screen.getByText(/GIF 原件预览保留动画/)).toBeTruthy();
  expect(queries.freshLibrary).toHaveBeenCalledTimes(2);
  view.unmount();
  expect(image.hasAttribute("src")).toBe(false);
  vi.mocked(queries.freshLibrary).mockRejectedValue(
    new ApiError(403, "forbidden"),
  );
  mount();
  await screen.findByRole("button", { name: "重新读取素材详情" });
  expect(screen.queryByRole("img")).toBeNull();
});
it("选中正文懒读保留空白，无文本二进制预览或下载", async () => {
  mount();
  await waitFor(() =>
    expect(screen.getByTestId("library-plain-text").textContent).toBe(
      detail.plain_text,
    ),
  );
  expect(queries.previewLibrary).not.toHaveBeenCalled();
  expect(screen.queryByRole("button", { name: "下载素材原件" })).toBeNull();
});
it("缓存正文挂载前必须fresh scope成功，403不能借warm缓存读取", async () => {
  const cache = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  cache.setQueryData([...queries.libraryKey(identity), "detail", id], detail);
  let fail!: (reason: Error) => void;
  vi.mocked(queries.freshLibrary).mockImplementation(
    () =>
      new Promise((_, reject) => {
        fail = reject;
      }),
  );
  mount(cache);
  await waitFor(() => expect(queries.freshLibrary).toHaveBeenCalled());
  expect(screen.queryByTestId("library-plain-text")).toBeNull();
  expect(queries.getLibraryDetail).not.toHaveBeenCalled();
  await act(async () => fail(new ApiError(403, "forbidden")));
  await screen.findByRole("button", { name: "重新读取素材详情" });
  expect(screen.queryByTestId("library-plain-text")).toBeNull();
});
it("document仅正式attachment下载，展示实际完整SHA而不伪造预览", async () => {
  const media = {
    id,
    kind: "document" as const,
    file_name: "正式.txt",
    mime_type: "text/plain",
    byte_size: 4,
    width: null,
    height: null,
    duration_ms: null,
    revision: 1,
  };
  vi.mocked(queries.getLibraryDetail).mockResolvedValue({
    ...detail,
    kind: "document",
    asset_id: id,
    media,
    plain_text: undefined,
  });
  const sha = "a".repeat(64);
  vi.mocked(queries.downloadLibraryOriginal).mockResolvedValue({
    blob: new Blob(["real"], { type: "text/plain" }),
    sha256: sha,
    fileName: media.file_name,
  });
  vi.stubGlobal(
    "URL",
    class extends URL {
      static createObjectURL = vi.fn(() => "blob:owned");
      static revokeObjectURL = vi.fn();
    },
  );
  const click = vi
    .spyOn(HTMLAnchorElement.prototype, "click")
    .mockImplementation(() => {});
  mount();
  fireEvent.click(await screen.findByRole("button", { name: "下载素材原件" }));
  await screen.findByText(sha);
  expect(queries.previewLibrary).not.toHaveBeenCalled();
  expect(click).toHaveBeenCalledOnce();
  vi.unstubAllGlobals();
});
