export const imageRatios = [
  "1:1",
  "16:9",
  "9:16",
  "4:3",
  "3:4",
  "3:2",
  "2:3",
  "21:9",
];
const imageSizePixels: Record<string, number> = {
  "1K": 1024,
  "1.5K": 1536,
  "2K": 2048,
};

function gcd(a: number, b: number): number {
  return b ? gcd(b, a % b) : a;
}

/** Ratio is an editing convenience; only native size is persisted/submitted. */
export function imageSizeForRatio(
  size: string,
  ratio: string,
): string | undefined {
  if (ratio === "auto") return size;
  const pixels = imageSizePixels[size];
  const match = /^(\d+):(\d+)$/.exec(ratio);
  if (!pixels || !match) return undefined;
  const a = Number(match[1]),
    b = Number(match[2]);
  if (!a || !b || a / b < 1 / 16 || a / b > 16) return undefined;
  const divisor = gcd(a, b),
    width = a / divisor,
    height = b / divisor;
  const scale = Math.floor(pixels / Math.sqrt(width * height));
  const area = width * height * scale * scale;
  if (area < 921600 || area > 4624220) return undefined;
  return `${width * scale}x${height * scale}`;
}

export function readImageSize(value: string): { size: string; ratio: string } {
  const match = /^(\d+)x(\d+)$/.exec(value);
  if (!match) return { size: value, ratio: "auto" };
  const width = Number(match[1]),
    height = Number(match[2]);
  if (!width || !height) return { size: value, ratio: "auto" };
  const divisor = gcd(width, height);
  const ratio =
    imageRatios.find((candidate) => {
      const [a, b] = candidate.split(":").map(Number);
      return width * b === height * a;
    }) ?? `${width / divisor}:${height / divisor}`;
  const size = Object.keys(imageSizePixels).find(
    (size) => imageSizeForRatio(size, ratio) === value,
  );
  return { size: size ?? value, ratio };
}
