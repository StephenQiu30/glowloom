// Derived from BeefTV 1ae25027: web/src/components/canvas/
// canvas-node-mask-edit-dialog.tsx. MIT; see docs/licenses/beeftv.txt.
// Selection strokes and alpha-mask semantics retained; bounded PNG Files replace data URLs.
export function maskDimensions(width: number, height: number) {
  if (
    ![width, height].every(
      (value) => Number.isInteger(value) && value > 0 && value <= 16384,
    ) ||
    width * height > 16_000_000
  )
    throw new Error("蒙版支持最多1600万像素，请先使用图片工具缩小原图。");
}
export function editMaskPixels(
  selection: Uint8ClampedArray,
  width: number,
  height: number,
) {
  maskDimensions(width, height);
  if (selection.length !== width * height * 4)
    throw new Error("蒙版像素与原图尺寸不匹配。");
  const result = new Uint8ClampedArray(selection.length).fill(255);
  let painted = false;
  for (let index = 3; index < selection.length; index += 4) {
    if (selection[index] > 0) {
      result[index] = 0;
      painted = true;
    }
  }
  if (!painted) throw new Error("请先涂抹局部修改区域。");
  return result;
}
export function drawMaskStroke(
  context: CanvasRenderingContext2D,
  from: { x: number; y: number },
  to: { x: number; y: number },
  size: number,
) {
  context.beginPath();
  if (from.x === to.x && from.y === to.y) {
    context.arc(to.x, to.y, size / 2, 0, Math.PI * 2);
    context.fill();
  } else {
    context.moveTo(from.x, from.y);
    context.lineTo(to.x, to.y);
    context.stroke();
  }
}
export function renderMaskPreview(
  selection: HTMLCanvasElement,
  preview: HTMLCanvasElement,
) {
  const context = preview.getContext("2d");
  if (!context) throw new Error("无法创建蒙版预览。");
  context.clearRect(0, 0, preview.width, preview.height);
  context.fillStyle = "rgba(37, 99, 235, .38)";
  context.fillRect(0, 0, preview.width, preview.height);
  context.globalCompositeOperation = "destination-in";
  context.drawImage(selection, 0, 0);
  context.globalCompositeOperation = "source-over";
}
export async function editMaskFile(
  selection: HTMLCanvasElement,
  title: string,
) {
  const context = selection.getContext("2d");
  if (!context) throw new Error("无法读取蒙版区域。");
  const pixels = editMaskPixels(
    context.getImageData(0, 0, selection.width, selection.height).data,
    selection.width,
    selection.height,
  );
  const result = document.createElement("canvas");
  result.width = selection.width;
  result.height = selection.height;
  const output = result.getContext("2d");
  if (!output) throw new Error("无法创建蒙版文件。");
  output.putImageData(new ImageData(pixels, result.width, result.height), 0, 0);
  try {
    const blob = await new Promise<Blob>((resolve, reject) =>
      result.toBlob(
        (value) =>
          value ? resolve(value) : reject(new Error("蒙版编码失败。")),
        "image/png",
      ),
    );
    if (blob.size > 20 * 1024 * 1024)
      throw new Error("蒙版文件超过20 MiB，请先缩小原图。");
    const name = Array.from(
      title
        .split(/[\\/]/)
        .at(-1)
        ?.replace(/\.[^.]+$/, "")
        .replace(/[<>:"|?*\u0000-\u001f]/g, "")
        .trim() || "图片",
    )
      .slice(0, 100)
      .join("");
    return new File([blob], `${name}-局部重绘蒙版.png`, { type: "image/png" });
  } finally {
    result.width = 0;
    result.height = 0;
  }
}
