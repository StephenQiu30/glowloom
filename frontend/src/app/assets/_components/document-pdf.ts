/** Loaded only after the user requests a PDF; never renders or requests a supplier artifact. */
import { PDFDocument, rgb } from "pdf-lib";
import fontkit from "@pdf-lib/fontkit";
let cachedFont: Uint8Array<ArrayBuffer> | undefined;
async function readFont(signal: AbortSignal) {
  if (cachedFont) return cachedFont;
  // eslint-disable-next-line no-restricted-syntax -- Load the bundled static OFL font asset; business reads use generated API clients.
  const response = await fetch(
    "/fonts/asset-document/NotoSansCJKsc-Regular.otf",
    { signal, cache: "force-cache", credentials: "same-origin" },
  );
  if (!response.ok)
    throw new Error("PDF字体暂时无法读取，请稍后重试或下载Markdown。");
  const bytes = new Uint8Array(await response.arrayBuffer());
  signal.throwIfAborted();
  cachedFont = bytes;
  return bytes;
}
export async function renderDocumentPdf(
  text: string,
  title: string,
  signal: AbortSignal,
) {
  signal.throwIfAborted();
  if (!text.trim() || text.length > 65536)
    throw new Error("文档正文无效，请重新读取后下载。");
  const pdf = await PDFDocument.create();
  pdf.registerFontkit(fontkit);
  const font = await pdf.embedFont(await readFont(signal), { subset: true });
  signal.throwIfAborted();
  const supported = new Set(font.getCharacterSet());
  const size = 11,
    lineHeight = 18,
    margin = 44,
    width = 595.28,
    height = 841.89;
  const advance = new Map<string, number>();
  let page = pdf.addPage([width, height]),
    y = height - margin;
  const draw = (line: string) => {
    if (y < margin + lineHeight) {
      page = pdf.addPage([width, height]);
      y = height - margin;
    }
    if (line)
      page.drawText(line, {
        x: margin,
        y,
        size,
        font,
        color: rgb(0.08, 0.08, 0.08),
      });
    y -= lineHeight;
  };
  let examined = 0;
  for (const paragraph of text
    .replace(/\r\n?/g, "\n")
    .replace(/\t/g, "    ")
    .split("\n")) {
    let line = "",
      used = 0;
    for (const point of paragraph) {
      if (!supported.has(point.codePointAt(0)!))
        throw new Error(
          "PDF暂不支持正文中的部分字符，请下载Markdown以保留完整正文。",
        );
      let pointWidth = advance.get(point);
      if (pointWidth === undefined) {
        pointWidth = font.widthOfTextAtSize(point, size);
        advance.set(point, pointWidth);
      }
      if (line && used + pointWidth > width - margin * 2) {
        draw(line);
        line = "";
        used = 0;
      }
      line += point;
      used += pointWidth;
      if (++examined % 1024 === 0) {
        await new Promise<void>((resolve) => setTimeout(resolve, 0));
        signal.throwIfAborted();
      }
    }
    draw(line);
  }
  pdf.setTitle(title);
  pdf.setCreator("Glowloom");
  pdf.setProducer("Glowloom PDF export");
  const bytes = await pdf.save();
  signal.throwIfAborted();
  return new Blob([Uint8Array.from(bytes).buffer], { type: "application/pdf" });
}
