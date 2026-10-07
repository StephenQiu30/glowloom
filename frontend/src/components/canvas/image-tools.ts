// Derived from BeefTV 1ae25027: web/src/lib/canvas/canvas-image-data.ts and
// canvas-grid-split.ts. MIT; see workspace/content/licenses/beeftv.txt.
// Adaptation: typed geometry, bounded work, cancellation, explicit errors and
// File outputs for Lanverse's existing media upload/registration contract.
import type { AnnotationOperation } from "./image-annotation";
export type ImageCrop = { x: number; y: number; width: number; height: number };
export type ImagePoint = { x: number; y: number };
export type ImageMark = AnnotationOperation;
export type ImageToolOperation =
  | { kind: "crop"; crop: ImageCrop }
  | { kind: "split"; rows: number; columns: number }
  | {
      kind: "resize";
      longEdge: number;
      algorithm: "nearest" | "bilinear" | "high";
    }
  | {
      kind: "annotate";
      marks: ImageMark[];
      label?: string;
      labelColor?: string;
    };

function dimensions(width: number, height: number) {
  if (
    ![width, height].every(
      (value) => Number.isSafeInteger(value) && value > 0 && value <= 16384,
    ) ||
    width * height > 64_000_000
  )
    throw new Error("图片尺寸无效或超过工具处理范围。");
}

export function imageCropPixels(
  width: number,
  height: number,
  crop: ImageCrop,
) {
  dimensions(width, height);
  if (
    ![crop.x, crop.y, crop.width, crop.height].every(Number.isFinite) ||
    crop.x < 0 ||
    crop.y < 0 ||
    crop.x >= 1 ||
    crop.y >= 1 ||
    crop.width <= 0 ||
    crop.height <= 0
  )
    throw new Error("请选择图片内有效的裁切区域。");
  const x = Math.floor(crop.x * width),
    y = Math.floor(crop.y * height);
  return {
    x,
    y,
    width: Math.min(width - x, Math.ceil(crop.width * width)),
    height: Math.min(height - y, Math.ceil(crop.height * height)),
  };
}

export function imageGridCells(
  width: number,
  height: number,
  rows: number,
  columns: number,
) {
  dimensions(width, height);
  if (
    ![rows, columns].every(
      (value) => Number.isInteger(value) && value >= 1 && value <= 5,
    ) ||
    rows * columns < 2 ||
    rows > height ||
    columns > width
  )
    throw new Error("宫格支持1～5行/列，并须包含至少两个非空切片。");
  return Array.from({ length: rows * columns }, (_, index) => {
    const row = Math.floor(index / columns),
      column = index % columns;
    const x = Math.floor((column * width) / columns),
      y = Math.floor((row * height) / rows);
    return {
      row,
      column,
      x,
      y,
      width: Math.floor(((column + 1) * width) / columns) - x,
      height: Math.floor(((row + 1) * height) / rows) - y,
    };
  });
}

export function imageResizeSize(
  width: number,
  height: number,
  requestedLongEdge: number,
) {
  dimensions(width, height);
  if (!Number.isFinite(requestedLongEdge) || requestedLongEdge <= 0)
    throw new Error("请填写有效的目标长边。");
  const longEdge = Math.min(4096, Math.max(1, Math.round(requestedLongEdge)));
  const scale = longEdge / Math.max(width, height);
  return {
    width: Math.max(1, Math.round(width * scale)),
    height: Math.max(1, Math.round(height * scale)),
  };
}

export function imageToolFileName(
  name: string,
  kind: ImageToolOperation["kind"],
  index?: number,
) {
  const base =
    name
      .split(/[\\/]/)
      .at(-1)
      ?.replace(/\.[^.]+$/, "")
      .replace(/[<>:"|?*\u0000-\u001f]/g, "")
      .trim() || "图片";
  const label = {
    crop: "裁切",
    split: "宫格",
    resize: "放大",
    annotate: "标注",
  }[kind];
  return `${Array.from(base).slice(0, 100).join("")}-${label}${index === undefined ? "" : `-${index + 1}`}.png`;
}

function canvas(width: number, height: number) {
  const result = document.createElement("canvas");
  result.width = width;
  result.height = height;
  const context = result.getContext("2d");
  if (!context) throw new Error("浏览器无法创建图片编辑画布。");
  return { result, context };
}

function loadImage(url: string, signal: AbortSignal) {
  return new Promise<HTMLImageElement>((resolve, reject) => {
    signal.throwIfAborted();
    const image = new Image();
    image.crossOrigin = "anonymous";
    const finish = (error?: Error) => {
      clearTimeout(timeout);
      signal.removeEventListener("abort", abort);
      image.onload = null;
      image.onerror = null;
      if (error) {
        image.src = "";
        reject(error);
      } else resolve(image);
    };
    const abort = () =>
      finish(new DOMException("已取消图片处理", "AbortError"));
    image.onload = () => finish();
    image.onerror = () =>
      finish(new Error("图片读取失败，请重新授权素材预览后重试。"));
    signal.addEventListener("abort", abort, { once: true });
    const timeout = setTimeout(
      () => finish(new Error("图片读取超时，请重试。")),
      30_000,
    );
    image.src = url;
  });
}

function pngFile(canvas: HTMLCanvasElement, name: string, signal: AbortSignal) {
  signal.throwIfAborted();
  return new Promise<File>((resolve, reject) => {
    try {
      canvas.toBlob((blob) => {
        if (signal.aborted)
          reject(new DOMException("已取消图片处理", "AbortError"));
        else if (!blob) reject(new Error("图片编码失败。"));
        else resolve(new File([blob], name, { type: "image/png" }));
      }, "image/png");
    } catch {
      reject(new Error("图片无法编辑，请检查素材读取权限。"));
    }
  });
}

export async function runImageTool(
  url: string,
  name: string,
  operation: ImageToolOperation,
  signal: AbortSignal,
): Promise<File[]> {
  const image = await loadImage(url, signal);
  try {
    dimensions(image.naturalWidth, image.naturalHeight);
    const width = image.naturalWidth,
      height = image.naturalHeight;
    if (operation.kind === "crop" || operation.kind === "split") {
      const cells =
        operation.kind === "crop"
          ? [imageCropPixels(width, height, operation.crop)]
          : imageGridCells(width, height, operation.rows, operation.columns);
      const files: File[] = [];
      for (const [index, cell] of cells.entries()) {
        signal.throwIfAborted();
        const { result, context } = canvas(cell.width, cell.height);
        context.drawImage(
          image,
          cell.x,
          cell.y,
          cell.width,
          cell.height,
          0,
          0,
          cell.width,
          cell.height,
        );
        files.push(
          await pngFile(
            result,
            imageToolFileName(
              name,
              operation.kind,
              operation.kind === "split" ? index : undefined,
            ),
            signal,
          ),
        );
        result.width = 0;
        result.height = 0;
      }
      return files;
    }
    const size =
      operation.kind === "resize"
        ? imageResizeSize(width, height, operation.longEdge)
        : { width, height };
    const { result, context } = canvas(size.width, size.height);
    if (operation.kind === "resize") {
      context.imageSmoothingEnabled = operation.algorithm !== "nearest";
      context.imageSmoothingQuality =
        operation.algorithm === "bilinear" ? "medium" : "high";
    }
    context.drawImage(image, 0, 0, size.width, size.height);
    if (operation.kind === "annotate") {
      if (operation.marks.length > 100 || (operation.label?.length ?? 0) > 200)
        throw new Error("标注内容超过允许范围。");
      for (const mark of operation.marks) {
        const validPoint = (point: ImagePoint) =>
          [point.x, point.y].every(
            (value) => Number.isFinite(value) && value >= 0 && value <= 1,
          );
        if (
          !/^#[0-9a-f]{6}$/i.test(mark.color) ||
          !Number.isFinite(mark.size) ||
          mark.size < 1 ||
          mark.size > 96
        )
          throw new Error("标注数据无效。");
        context.strokeStyle = mark.color;
        context.fillStyle = mark.color;
        context.lineWidth = (mark.size * width) / 1000;
        context.lineCap = "round";
        context.lineJoin = "round";
        if (mark.type === "brush") {
          if (
            !mark.points.length ||
            mark.points.length > 3000 ||
            !mark.points.every(validPoint)
          )
            throw new Error("画笔数据无效。");
          context.beginPath();
          const first = mark.points[0];
          if (mark.points.length === 1) {
            context.arc(
              first.x * width,
              first.y * height,
              context.lineWidth / 2,
              0,
              Math.PI * 2,
            );
            context.fill();
          } else {
            context.moveTo(first.x * width, first.y * height);
            mark.points
              .slice(1)
              .forEach((point) =>
                context.lineTo(point.x * width, point.y * height),
              );
            context.stroke();
          }
        } else if (mark.type === "rectangle") {
          if (
            !validPoint(mark) ||
            !validPoint({ x: mark.x + mark.width, y: mark.y + mark.height }) ||
            mark.width <= 0 ||
            mark.height <= 0
          )
            throw new Error("矩形标注无效。");
          context.strokeRect(
            mark.x * width,
            mark.y * height,
            mark.width * width,
            mark.height * height,
          );
        } else if (mark.type === "text") {
          if (!validPoint(mark) || !mark.text.trim() || mark.text.length > 200)
            throw new Error("文字标注无效。");
          context.font = `600 ${(mark.size * width) / 1000}px sans-serif`;
          context.textBaseline = "top";
          context.fillText(
            mark.text,
            mark.x * width,
            mark.y * height,
            width - mark.x * width,
          );
        } else throw new Error("不支持的标注类型。");
      }
      if (operation.label?.trim()) {
        context.font = `bold ${Math.max(18, Math.round(width / 35))}px sans-serif`;
        context.fillStyle = /^#[0-9a-f]{6}$/i.test(operation.labelColor ?? "")
          ? operation.labelColor!
          : "#ff3535";
        context.fillText(
          operation.label.trim(),
          width * 0.04,
          height * 0.1,
          width * 0.92,
        );
      }
    }
    const file = await pngFile(
      result,
      imageToolFileName(name, operation.kind),
      signal,
    );
    result.width = 0;
    result.height = 0;
    return [file];
  } finally {
    image.src = "";
  }
}
