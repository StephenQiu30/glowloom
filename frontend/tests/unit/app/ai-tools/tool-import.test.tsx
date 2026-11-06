import {
  cleanup,
  fireEvent,
  render,
  screen,
  within,
} from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { listMediaLibrary, previewLibraryMediaAsset } from "@/api/media";
import {
  categories,
  type ImportTool,
} from "@/app/ai-tools/_components/catalog";
import { ToolImportDialog } from "@/app/ai-tools/_components/tool-import-dialog";

const actorID = "11111111-1111-4111-8111-111111111111";
vi.mock("@/api/media", () => ({
  listMediaLibrary: vi.fn(),
  previewLibraryMediaAsset: vi.fn(),
}));
vi.mock("@/components/auth/session", () => ({
  useOptionalSession: () => ({ actor: { id: actorID } }),
}));
vi.mock("@/components/layout/basic-layout", () => ({
  useLoginDialog: () => vi.fn(),
}));

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
  vi.unstubAllGlobals();
});

function tool(id: string) {
  const entry = categories
    .flatMap(({ tools }) => tools)
    .find((entry) => entry.id === id);
  if (!entry || !("media" in entry)) throw new Error("expected import tool");
  return entry as ImportTool;
}

it("validates the media type and clears the local preview on unmount", async () => {
  const revokeObjectURL = vi.fn();
  vi.stubGlobal(
    "URL",
    class extends URL {
      static createObjectURL = vi.fn(() => "blob:http://localhost/test-image");
      static revokeObjectURL = revokeObjectURL;
    },
  );
  const view = render(
    <ToolImportDialog
      tool={tool("image-inpaint")}
      onClose={() => {}}
      onCloseAutoFocus={() => {}}
    />,
  );
  const input = screen.getByLabelText("从本地上传");
  fireEvent.change(input, {
    target: {
      files: [new File(["video"], "sample.mp4", { type: "video/mp4" })],
    },
  });
  expect(screen.getByText("文件类型与此工具不匹配。")).toBeTruthy();
  fireEvent.change(input, {
    target: {
      files: [new File(["image"], "sample.png", { type: "image/png" })],
    },
  });
  expect(screen.getByRole("img", { name: "sample.png" })).toBeTruthy();
  expect(screen.queryByRole("button", { name: "生成" })).toBeNull();
  view.unmount();
  expect(revokeObjectURL).toHaveBeenCalledWith(
    "blob:http://localhost/test-image",
  );
});

it("rejects a changed asset version and can retry the same selection", async () => {
  const assetID = "22222222-2222-4222-8222-222222222222";
  const itemID = "33333333-3333-4333-8333-333333333333";
  const media = { id: assetID, revision: 2, kind: "image" };
  vi.mocked(listMediaLibrary).mockResolvedValue({
    current_actor_id: actorID,
    current_org_id: actorID,
    library_id: actorID,
    scope: { kind: "personal" },
    revision: 1,
    page: 1,
    page_size: 12,
    total: 1,
    folders: [],
    items: [
      {
        id: itemID,
        asset_id: assetID,
        media,
        kind: "image",
        title: "参考图",
        revision: 1,
        favorite: false,
        catalog_state: "active",
        updated_at: "2026-11-07T00:00:00Z",
      },
    ],
  });
  vi.mocked(previewLibraryMediaAsset)
    .mockResolvedValueOnce({ asset: { ...media, revision: 3 } })
    .mockResolvedValueOnce({
      asset: media,
      url: "https://media.invalid/sample.png",
      expires_at: new Date(Date.now() + 60_000).toISOString(),
    });
  render(
    <ToolImportDialog
      tool={tool("smart-edit")}
      onClose={() => {}}
      onCloseAutoFocus={() => {}}
    />,
  );
  fireEvent.click(screen.getByRole("button", { name: "从资产库选择" }));
  const picker = screen.getByRole("dialog", { name: "资产库" });
  expect(
    within(picker)
      .getAllByRole("tab")
      .map((tab) => tab.textContent),
  ).toEqual(["图片", "视频"]);
  const confirm = within(picker).getByRole<HTMLButtonElement>("button", {
    name: "确认",
  });
  expect(confirm.disabled).toBe(true);
  fireEvent.click(
    await within(picker).findByRole("button", { name: "参考图" }),
  );
  fireEvent.click(confirm);
  expect(await within(picker).findByRole("alert")).toBeTruthy();
  expect(within(picker).getByText("已选择 1 个素材")).toBeTruthy();
  fireEvent.click(confirm);
  expect(await screen.findByRole("img", { name: "参考图" })).toBeTruthy();
  expect(previewLibraryMediaAsset).toHaveBeenCalledWith(
    { item_id: itemID, scope: "personal" },
    expect.objectContaining({ signal: expect.any(AbortSignal) }),
  );
});
