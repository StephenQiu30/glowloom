// Pixel crop/split/resize adapted from the source editor's canvas-image-data.
// Keep computation independent from media identity, uploading and graph persistence.
export type ImageOperation =
  | { kind: "crop"; x: number; y: number; width: number; height: number }
  | { kind: "split"; rows: number; columns: number }
  | { kind: "resize"; longEdge: number };
type Region = { x: number; y: number; width: number; height: number };
export function dragImageCrop(
  crop: Region,
  dx: number,
  dy: number,
  mode: "move" | "resize",
): Region {
  const clamp = (n: number, min: number, max: number) =>
    Math.round(Math.max(min, Math.min(max, n)) * 100) / 100;
  return mode === "move"
    ? {
        ...crop,
        x: clamp(crop.x + dx, 0, 100 - crop.width),
        y: clamp(crop.y + dy, 0, 100 - crop.height),
      }
    : {
        ...crop,
        width: clamp(crop.width + dx, 1, 100 - crop.x),
        height: clamp(crop.height + dy, 1, 100 - crop.y),
      };
}
const validSize = (n: number) => Number.isSafeInteger(n) && n > 0;
export function imageRegions(
  width: number,
  height: number,
  operation: Exclude<ImageOperation, { kind: "resize" }>,
): Region[] {
  if (!validSize(width) || !validSize(height))
    throw new Error("图片尺寸无效。");
  if (operation.kind === "crop") {
    const { x, y, width: w, height: h } = operation;
    if (
      ![x, y, w, h].every(Number.isFinite) ||
      x < 0 ||
      y < 0 ||
      w <= 0 ||
      h <= 0 ||
      x + w > 1.000001 ||
      y + h > 1.000001
    )
      throw new Error("裁剪范围必须位于图片内。");
    const left = Math.floor(x * width),
      top = Math.floor(y * height);
    return [
      {
        x: left,
        y: top,
        width: Math.min(width - left, Math.max(1, Math.ceil(w * width))),
        height: Math.min(height - top, Math.max(1, Math.ceil(h * height))),
      },
    ];
  }
  const { rows, columns } = operation;
  if (
    ![rows, columns].every((n) => validSize(n) && n <= 6) ||
    rows > height ||
    columns > width
  )
    throw new Error("拆图行列须为1～6，且不能超过原图像素尺寸。");
  return Array.from({ length: rows * columns }, (_, index) => {
    const row = Math.floor(index / columns),
      column = index % columns;
    const x = Math.floor((column * width) / columns),
      y = Math.floor((row * height) / rows);
    return {
      x,
      y,
      width: Math.floor(((column + 1) * width) / columns) - x,
      height: Math.floor(((row + 1) * height) / rows) - y,
    };
  });
}
export function resizeDimensions(
  width: number,
  height: number,
  longEdge: number,
) {
  if (![width, height, longEdge].every(validSize) || longEdge > 4096)
    throw new Error("缩放最长边须为1～4096像素。");
  const scale = longEdge / Math.max(width, height);
  return {
    width: Math.max(1, Math.round(width * scale)),
    height: Math.max(1, Math.round(height * scale)),
  };
}
export async function processCanvasImage(
  blob: Blob,
  operation: ImageOperation,
  signal: AbortSignal,
): Promise<File[]> {
  signal.throwIfAborted();
  const image = await createImageBitmap(blob, {
    imageOrientation: "from-image",
  });
  try {
    if (image.width * image.height > 64_000_000)
      throw new Error("图片超过本地处理尺寸，请使用小于6400万像素的图片。");
    const regions =
      operation.kind === "resize"
        ? [{ x: 0, y: 0, width: image.width, height: image.height }]
        : imageRegions(image.width, image.height, operation);
    const files: File[] = [];
    for (const [index, region] of regions.entries()) {
      signal.throwIfAborted();
      const size =
        operation.kind === "resize"
          ? resizeDimensions(image.width, image.height, operation.longEdge)
          : region;
      const canvas = document.createElement("canvas");
      canvas.width = size.width;
      canvas.height = size.height;
      try {
        const context = canvas.getContext("2d");
        if (!context) throw new Error("浏览器不支持图像处理。");
        context.imageSmoothingQuality = "high";
        context.drawImage(
          image,
          region.x,
          region.y,
          region.width,
          region.height,
          0,
          0,
          size.width,
          size.height,
        );
        const result = await new Promise<Blob>((resolve, reject) =>
          canvas.toBlob(
            (value) =>
              value ? resolve(value) : reject(new Error("图片编码失败。")),
            "image/png",
          ),
        );
        if (result.size > 20 * 1024 * 1024)
          throw new Error("处理结果超过20 MB，请减小图片尺寸。");
        files.push(
          new File([result], `${operation.kind}-${index + 1}.png`, {
            type: "image/png",
          }),
        );
      } finally {
        canvas.width = 0;
        canvas.height = 0;
      }
    }
    signal.throwIfAborted();
    return files;
  } finally {
    image.close();
  }
}
