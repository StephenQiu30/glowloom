export function hsvToHex(h: number, s: number, v: number) {
  const c = v * s,
    x = c * (1 - Math.abs(((h / 60) % 2) - 1)),
    m = v - c;
  const rgb =
    h < 60
      ? [c, x, 0]
      : h < 120
        ? [x, c, 0]
        : h < 180
          ? [0, c, x]
          : h < 240
            ? [0, x, c]
            : h < 300
              ? [x, 0, c]
              : [c, 0, x];
  return (
    "#" +
    rgb
      .map((n) =>
        Math.round((n + m) * 255)
          .toString(16)
          .padStart(2, "0"),
      )
      .join("")
      .toUpperCase()
  );
}
export function hexToHSV(hex: string) {
  const rgb = [1, 3, 5].map((i) => parseInt(hex.slice(i, i + 2), 16) / 255);
  const max = Math.max(...rgb),
    min = Math.min(...rgb),
    d = max - min;
  let h =
    d === 0
      ? 0
      : max === rgb[0]
        ? 60 * (((rgb[1] - rgb[2]) / d) % 6)
        : max === rgb[1]
          ? 60 * ((rgb[2] - rgb[0]) / d + 2)
          : 60 * ((rgb[0] - rgb[1]) / d + 4);
  if (h < 0) h += 360;
  return { h, s: max === 0 ? 0 : d / max, v: max };
}
export function paletteHarmony(hex: string, mode: string, count: number) {
  const { h, s, v } = hexToHSV(hex);
  return Array.from({ length: count }, (_, i) =>
    hsvToHex(
      (h +
        (mode === "互补色"
          ? (i % 2) * 180
          : mode === "三等分取色"
            ? (i % 3) * 120
            : (i - (count - 1) / 2) * 15) +
        360) %
        360,
      s,
      v,
    ),
  );
}
/** Quantized frequency bins retain actual sampled pixel means, excluding transparent pixels. */
export function sampledPalette(data: Uint8ClampedArray, count = 5) {
  const bins = new Map<
    string,
    { count: number; r: number; g: number; b: number }
  >();
  for (let i = 0; i < data.length; i += 4) {
    if (data[i + 3] < 128) continue;
    const key = [data[i], data[i + 1], data[i + 2]]
      .map((v) => v >> 5)
      .join(",");
    const b = bins.get(key) ?? { count: 0, r: 0, g: 0, b: 0 };
    b.count++;
    b.r += data[i];
    b.g += data[i + 1];
    b.b += data[i + 2];
    bins.set(key, b);
  }
  return [...bins.values()]
    .sort((a, b) => b.count - a.count)
    .slice(0, count)
    .map(
      (b) =>
        "#" +
        [b.r, b.g, b.b]
          .map((v) =>
            Math.round(v / b.count)
              .toString(16)
              .padStart(2, "0"),
          )
          .join("")
          .toUpperCase(),
    );
}
export async function paletteFromImage(file: File) {
  if (!file.type.startsWith("image/") || file.size > 20 * 1024 * 1024)
    throw new Error("请选择20MB以内的图片");
  const bitmap = await createImageBitmap(file);
  try {
    const canvas = document.createElement("canvas");
    canvas.width = 64;
    canvas.height = 64;
    const ctx = canvas.getContext("2d");
    if (!ctx) throw new Error("当前浏览器无法读取图片颜色");
    ctx.drawImage(bitmap, 0, 0, 64, 64);
    const colors = sampledPalette(ctx.getImageData(0, 0, 64, 64).data);
    if (!colors.length) throw new Error("图片中没有可用的不透明颜色");
    return colors;
  } finally {
    bitmap.close();
  }
}
