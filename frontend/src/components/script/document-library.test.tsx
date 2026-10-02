import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { DocumentLibrary } from "./document-library";
import { listDocuments, uploadDocument } from "./document-media";
import type { DocumentAsset } from "./document-media";
import { getWorkspace } from "./source-queries";
import { saveDocumentUpload } from "./document-upload-intent";
vi.mock("./document-media", async (original) => ({
  ...(await original<typeof import("./document-media")>()),
  listDocuments: vi.fn(),
  uploadDocument: vi.fn(),
}));
vi.mock("./source-queries", async (original) => ({
  ...(await original<typeof import("./source-queries")>()),
  getWorkspace: vi.fn(),
}));
const id = (n: number) =>
  `${String(n).repeat(8)}-${String(n).repeat(4)}-4${String(n).repeat(3)}-8${String(n).repeat(3)}-${String(n).repeat(12)}`;
const scope = {
  origin: window.location.origin,
  actorId: id(1),
  orgId: id(2),
  projectId: id(3),
};
const asset = {
  id: id(4),
  project_id: scope.projectId,
  kind: "document" as const,
  file_name: "原件.txt",
  mime_type: "text/plain" as const,
  byte_size: 6,
  revision: 1,
};
beforeEach(() => {
  vi.clearAllMocks();
  sessionStorage.clear();
  vi.mocked(listDocuments).mockResolvedValue({
    items: [asset],
    next_cursor: null,
  });
  vi.mocked(getWorkspace).mockResolvedValue({
    current_actor_id: scope.actorId,
    current_org_id: scope.orgId,
    state: {
      project_id: scope.projectId,
      org_id: scope.orgId,
      revision: 0,
      updated_at: "2026-10-02T00:00:00Z",
    },
  });
  vi.stubGlobal(
    "ResizeObserver",
    class {
      observe() {}
      unobserve() {}
      disconnect() {}
    },
  );
});
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});
function mount(onImport?: (assets: DocumentAsset[]) => void) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  const select = vi.fn();
  render(
    <QueryClientProvider client={client}>
      <DocumentLibrary
        scope={scope}
        disabled={false}
        onSelect={select}
        onImport={onImport}
      />
    </QueryClientProvider>,
  );
  return select;
}
it("未完成上传刷新自动呈现原键，但缺确切原文件不能重放或关闭", async () => {
  saveDocumentUpload(sessionStorage, {
    ...scope,
    version: 1,
    key: id(5),
    fileName: asset.file_name,
    byteSize: asset.byte_size,
    sha256: "a".repeat(64),
    localReviewConfirmed: true,
  });
  mount();
  expect(
    await screen.findByRole("dialog", { name: "恢复原文上传" }),
  ).toBeTruthy();
  expect(screen.getByText(/刷新不会自动重传/)).toBeTruthy();
  expect(
    screen
      .getByRole("button", { name: "人工使用原键核验上传" })
      .matches(":disabled"),
  ).toBe(true);
  expect(uploadDocument).not.toHaveBeenCalled();
  fireEvent.keyDown(screen.getByRole("dialog"), { key: "Escape" });
  expect(screen.getByRole("dialog", { name: "恢复原文上传" })).toBeTruthy();
  expect(
    screen.getByRole("button", { name: "关闭原文上传" }).matches(":disabled"),
  ).toBe(true);
});
it("跨页原件多选保点击顺序，可显式调整，导入交完整原asset而不直接写来源", async () => {
  const second = { ...asset, id: id(6), file_name: "第二.txt" };
  vi.mocked(listDocuments).mockResolvedValue({
    items: [asset, second],
    next_cursor: null,
  });
  const importer = vi.fn();
  mount(importer);
  const firstSelect = await screen.findByRole("checkbox", {
    name: `导入选择 ${asset.file_name} ${asset.id}`,
  });
  await waitFor(() => expect(firstSelect.matches(":disabled")).toBe(false));
  fireEvent.click(
    screen.getByRole("checkbox", {
      name: `导入选择 ${second.file_name} ${second.id}`,
    }),
  );
  fireEvent.click(firstSelect);
  fireEvent.click(screen.getByRole("button", { name: "导入所选 2 个原件" }));
  expect(importer).toHaveBeenLastCalledWith([second, asset]);
  fireEvent.click(
    screen.getByRole("button", { name: `上移所选原件 ${asset.id}` }),
  );
  fireEvent.click(screen.getByRole("button", { name: "导入所选 2 个原件" }));
  expect(importer).toHaveBeenLastCalledWith([asset, second]);
  expect(uploadDocument).not.toHaveBeenCalled();
});
it("文档页只选择同项目确切原asset；非法本地视频不被当成原文上传", async () => {
  const select = mount();
  fireEvent.click(
    await screen.findByRole("button", { name: "选择原件 原件.txt" }),
  );
  expect(select).toHaveBeenCalledWith(asset);
  const open = screen.getByRole("button", { name: "上传文档原件" });
  await waitFor(() => expect(open.matches(":disabled")).toBe(false));
  fireEvent.click(open);
  fireEvent.change(screen.getByLabelText("选择本地原件"), {
    target: {
      files: [new File(["video"], "video.mp4", { type: "video/mp4" })],
    },
  });
  expect(await screen.findByRole("alert")).toBeTruthy();
  expect(
    screen.getByRole("button", { name: "确认并上传原件" }).matches(":disabled"),
  ).toBe(true);
  expect(uploadDocument).not.toHaveBeenCalled();
});
