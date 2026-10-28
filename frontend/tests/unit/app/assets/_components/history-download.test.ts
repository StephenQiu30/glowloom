import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { downloadHistoryCards } from "@/app/assets/_components/history-download";
import { requirePersonalHistory } from "@/app/assets/_components/asset-query";
import { historyCardTargets } from "@/app/assets/_components/history-results";
const api = vi.hoisted(() => ({
  original: vi.fn(),
  document: vi.fn(),
  operation: vi.fn(),
}));
vi.mock("@/api/media", () => ({
  downloadMediaOriginal: api.original,
  downloadDocumentAsset: api.document,
}));
vi.mock("@/api/operations", () => ({ getOperation: api.operation }));
const id = "11111111-1111-4111-8111-111111111111",
  pid = "22222222-2222-4222-8222-222222222222";
const ids = [
  "33333333-3333-4333-8333-333333333333",
  "44444444-4444-4444-8444-444444444444",
];
const blobs: Blob[] = [],
  names: string[] = [];
beforeEach(() => {
  vi.clearAllMocks();
  blobs.length = 0;
  names.length = 0;
  const BrowserURL = URL;
  vi.stubGlobal(
    "URL",
    class extends BrowserURL {
      static createObjectURL(blob: Blob) {
        blobs.push(blob);
        return "blob:verified-download";
      }
      static revokeObjectURL() {}
    },
  );
  vi.spyOn(HTMLAnchorElement.prototype, "click").mockImplementation(function (
    this: HTMLAnchorElement,
  ) {
    names.push(this.download);
  });
});
afterEach(() => {
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});
function cards(kind: "image" | "document") {
  const task = {
    id,
    project_id: pid,
    status: "completed",
    capability: kind === "image" ? "image.generate" : "text.structured",
    create_time: "2026-10-28T00:00:00Z",
    prompt_preview: "原始结果",
    history_revision: 1,
    outputs: ids.map((id) => ({
      id,
      kind: kind === "image" ? "image" : "json",
      media_asset_id: kind === "image" ? id : null,
      moderation_status: "passed",
      favorite: false,
      metadata_revision: 1,
      resolution: "",
      ratio: "",
      generation_mode: "",
      process_types: [],
      can_download: true,
      can_publish: false,
    })),
  };
  return historyCardTargets(
    requirePersonalHistory({
      items: [task],
      next_cursor: null,
    } as unknown as API.PersonalHistoryPageResponse).items,
    kind,
  );
}
it("unfinished formal candidates cannot bypass download eligibility", async () => {
  const selected = cards("document").map((card) => ({
    ...card,
    task: { ...card.task, status: "ingesting" as const },
    outputs: card.outputs.map((output) => ({
      ...output,
      kind: "document",
      media_asset_id: id,
    })),
  }));
  const results = await downloadHistoryCards(
    selected,
    "pdf",
    new AbortController().signal,
    () => {},
  );
  expect(results.every((item) => !item.ok)).toBe(true);
  expect(api.document).not.toHaveBeenCalled();
  expect(api.operation).not.toHaveBeenCalled();
});
it("逐项授权原件下载保留部分失败，真实MIME决定文件扩展名", async () => {
  api.original
    .mockResolvedValueOnce(new Blob(["actual-file"], { type: "image/webp" }))
    .mockRejectedValueOnce(new Error("revoked"));
  const controller = new AbortController(),
    progress = vi.fn();
  const results = await downloadHistoryCards(
    cards("image"),
    "pdf",
    controller.signal,
    progress,
  );
  expect(results.map((item) => item.ok)).toEqual([true, false]);
  expect(api.original).toHaveBeenNthCalledWith(
    1,
    { pid, asset_id: ids[0] },
    { signal: controller.signal, responseType: "blob" },
  );
  expect(api.document).not.toHaveBeenCalled();
  expect(names[0]).toMatch(/\.webp$/);
  expect(blobs).toHaveLength(1);
  expect(progress).toHaveBeenLastCalledWith(2, 2);
});
it("同任务文档只读取一次完整任务，导出所选有限正文而不伪造预览下载", async () => {
  api.operation.mockResolvedValue({
    id,
    project_id: pid,
    status: "completed",
    capability: "text.structured",
    outputs: ids.map((id, index) => ({
      id,
      kind: "json",
      moderation_status: "passed",
      text: JSON.stringify({ result: index }),
    })),
  });
  const results = await downloadHistoryCards(
    cards("document"),
    "md",
    new AbortController().signal,
    () => {},
  );
  expect(results.every((item) => item.ok)).toBe(true);
  expect(api.operation).toHaveBeenCalledTimes(1);
  expect(names).toHaveLength(2);
  expect(names.every((name) => name.endsWith(".md"))).toBe(true);
  expect(blobs.every((blob) => blob.type.startsWith("text/markdown"))).toBe(
    true,
  );
  expect(api.original).not.toHaveBeenCalled();
  expect(api.document).not.toHaveBeenCalled();
});
it("最终正文无权或缺失时失败，取消后不再启动后续下载", async () => {
  api.operation.mockResolvedValue({
    id,
    project_id: pid,
    status: "completed",
    capability: "text.structured",
    outputs: [{ id: ids[0], kind: "json", moderation_status: "pending" }],
  });
  const result = await downloadHistoryCards(
    cards("document").slice(0, 1),
    "pdf",
    new AbortController().signal,
    () => {},
  );
  expect(result[0].ok).toBe(false);
  expect(names).toHaveLength(0);
  const controller = new AbortController();
  controller.abort();
  expect(
    await downloadHistoryCards(
      cards("image"),
      "md",
      controller.signal,
      () => {},
    ),
  ).toEqual([]);
  expect(api.original).not.toHaveBeenCalled();
});

it("正式文档不能把DOCX原件伪装PDF，原件只走明确的辅助动作", async () => {
  const source = cards("document")[0];
  const formal = {
    ...source,
    outputs: [
      { ...source.outputs[0], kind: "document", media_asset_id: ids[0] },
    ],
  };
  api.document.mockResolvedValue(
    new Blob(["original-bytes"], {
      type: "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
    }),
  );
  const conversion = await downloadHistoryCards(
    [formal],
    "pdf",
    new AbortController().signal,
    () => {},
  );
  expect(conversion[0]).toMatchObject({ ok: true, format: "original" });
  expect(api.document).toHaveBeenCalledOnce();
  expect(names[0]).toMatch(/\.docx$/);
  const original = await downloadHistoryCards(
    [formal],
    "md",
    new AbortController().signal,
    () => {},
  );
  expect(original[0].ok).toBe(true);
  expect(names[0]).toMatch(/\.docx$/);
});

it("mixed document download exports body and original with per-output formats", async () => {
  const originals = cards("document"),
    formal = {
      ...originals[1],
      outputs: [
        {
          ...originals[1].outputs[0],
          kind: "document",
          media_asset_id: ids[1],
        },
      ],
    };
  api.operation.mockResolvedValue({
    id,
    project_id: pid,
    status: "completed",
    capability: "text.structured",
    outputs: [
      { id: ids[0], kind: "json", moderation_status: "passed", text: "body" },
      {
        id: ids[1],
        kind: "document",
        moderation_status: "passed",
        media_asset_id: ids[1],
      },
    ],
  });
  api.document.mockResolvedValue(
    new Blob(["original-bytes"], {
      type: "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
    }),
  );
  const results = await downloadHistoryCards(
    [originals[0], formal],
    "md",
    new AbortController().signal,
    () => {},
  );
  expect(results.map((item) => [item.ok, item.format])).toEqual([
    [true, "md"],
    [true, "original"],
  ]);
  expect(names[0]).toMatch(/\.md$/);
  expect(names[1]).toMatch(/\.docx$/);
});
