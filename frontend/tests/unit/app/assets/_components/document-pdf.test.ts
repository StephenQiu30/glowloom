import { readFile } from "node:fs/promises";
import path from "node:path";
import { PDFDocument, PDFDict, PDFName } from "pdf-lib";
import { afterEach, expect, it, vi } from "vitest";
import { renderDocumentPdf } from "@/app/assets/_components/document-pdf";
afterEach(() => vi.unstubAllGlobals());
function buffer(blob: Blob) {
  return new Promise<ArrayBuffer>((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve(reader.result as ArrayBuffer);
    reader.onerror = reject;
    reader.readAsArrayBuffer(blob);
  });
}
it("CJK正文导出为分页且带Unicode文本映射的真实PDF，并子集嵌入授权字体", async () => {
  const font = await readFile(
    path.resolve("public/fonts/asset-document/NotoSansCJKsc-Regular.otf"),
  );
  const fetcher = vi
    .fn()
    .mockResolvedValue(new Response(Uint8Array.from(font).buffer));
  vi.stubGlobal("fetch", fetcher);
  const blob = await renderDocumentPdf(
    "山谷晨光，创作说明 ABC123。\n".repeat(120),
    "合成验收",
    new AbortController().signal,
  );
  expect(blob.type).toBe("application/pdf");
  expect(blob.size).toBeLessThan(2_000_000);
  const bytes = await buffer(blob);
  expect(new TextDecoder().decode(bytes.slice(0, 5))).toBe("%PDF-");
  const document = await PDFDocument.load(bytes);
  expect(document.getPageCount()).toBeGreaterThan(1);
  expect(document.getTitle()).toBe("合成验收");
  const fonts = document
    .getPage(0)
    .node.Resources()!
    .lookup(PDFName.of("Font"), PDFDict);
  expect(
    fonts
      .keys()
      .some((key) => fonts.lookup(key, PDFDict).has(PDFName.of("ToUnicode"))),
  ).toBe(true);
  expect(fetcher).toHaveBeenCalledWith(
    "/fonts/asset-document/NotoSansCJKsc-Regular.otf",
    expect.objectContaining({
      credentials: "same-origin",
      signal: expect.any(AbortSignal),
    }),
  );
}, 20000);
it("取消或无正文不开始生成，缺少字形不静默丢失正文", async () => {
  const controller = new AbortController();
  controller.abort();
  await expect(
    renderDocumentPdf("正文", "验收", controller.signal),
  ).rejects.toThrow();
  await expect(
    renderDocumentPdf("", "验收", new AbortController().signal),
  ).rejects.toThrow("正文无效");
  await expect(
    renderDocumentPdf("正文😀", "验收", new AbortController().signal),
  ).rejects.toThrow("部分字符");
});
