import { z } from "zod";
import { CanvasNodeType, type CanvasNodeData, type Position } from "./model";
import type { MediaAsset } from "./queries";

const mib = 1024 * 1024;
const formats: Record<string, { maximum: number; mime: readonly string[] }> = {
  glb: { maximum: 64 * mib, mime: ["model/gltf-binary"] },
  jpg: { maximum: 20 * mib, mime: ["image/jpeg"] },
  jpeg: { maximum: 20 * mib, mime: ["image/jpeg"] },
  png: { maximum: 20 * mib, mime: ["image/png"] },
  webp: { maximum: 20 * mib, mime: ["image/webp"] },
  webm: {
    maximum: 500 * mib,
    mime: ["video/webm", "video/webm;codecs=vp8", "video/webm;codecs=vp9"],
  },
  mp4: { maximum: 500 * mib, mime: ["video/mp4"] },
  mov: { maximum: 500 * mib, mime: ["video/quicktime"] },
  mp3: { maximum: 100 * mib, mime: ["audio/mpeg", "audio/mp3"] },
  wav: {
    maximum: 100 * mib,
    mime: ["audio/wav", "audio/x-wav", "audio/wave", "audio/vnd.wave"],
  },
  m4a: { maximum: 100 * mib, mime: ["audio/mp4", "audio/x-m4a"] },
};

/** 浏览器只做初筛；内容、时长与使用权限由正式上传端点确认。 */
export function validateCanvasMediaFiles(
  files: readonly File[],
  remainingSlots = 20,
  maximumFiles = 20,
): { valid: boolean; errors: string[] } {
  const errors: string[] = [];
  if (!files.length) errors.push("请先选择本地媒体文件。");
  if (
    !Number.isInteger(maximumFiles) ||
    maximumFiles < 1 ||
    maximumFiles > 25 ||
    files.length > maximumFiles
  )
    errors.push(`每批最多选择 ${maximumFiles} 个文件。`);
  if (
    !Number.isInteger(remainingSlots) ||
    remainingSlots < 0 ||
    files.length > remainingSlots
  )
    errors.push("文件数量超过画布剩余节点容量。");
  let total = 0;
  for (const file of files) {
    const extension = file.name.split(".").at(-1)?.toLowerCase() ?? "";
    const format = Object.hasOwn(formats, extension)
      ? formats[extension]
      : undefined;
    const mime = file.type.toLowerCase();
    if (
      !format ||
      (mime &&
        mime !== "application/octet-stream" &&
        !format.mime.includes(mime))
    )
      errors.push(`${file.name}：不支持此格式，或文件类型与扩展名不一致。`);
    if (!Number.isSafeInteger(file.size) || file.size <= 0)
      errors.push(`${file.name}：文件为空或大小无效。`);
    else {
      total += file.size;
      if (format && file.size > format.maximum)
        errors.push(
          `${file.name}：超过此格式的 ${format.maximum / mib} MiB 上限。`,
        );
    }
  }
  if (total > 700 * mib) errors.push("每批文件总大小不能超过 700 MiB。");
  return { valid: errors.length === 0, errors };
}

const uuid = z.string().uuid();
function coordinate(value: number) {
  return Number.isFinite(value) && Math.abs(value) <= 1e6;
}
function mediaDimensions(asset: MediaAsset) {
  if (asset.kind === "audio") return { width: 320, height: 180 };
  if (!asset.width || !asset.height) return { width: 320, height: 240 };
  const scale = Math.min(480 / asset.width, 376 / asset.height);
  return {
    width: Math.max(120, Math.round(asset.width * scale)),
    height: Math.max(80, Math.round(asset.height * scale) + 44),
  };
}

/** 只投影正式身份；不将临时授权地址、原始 DTO 或任意 metadata 写入文档。 */
export function mediaAssetsToCanvasNodes(
  assets: readonly MediaAsset[],
  worldPosition: Position,
): CanvasNodeData[] {
  if (
    !assets.length ||
    assets.length > 2000 ||
    !coordinate(worldPosition.x) ||
    !coordinate(worldPosition.y)
  )
    throw new Error("素材数量或画布位置超出允许范围。");
  const projectId = assets[0].project_id;
  return assets.map((asset, index) => {
    if (
      !uuid.safeParse(asset.id).success ||
      !uuid.safeParse(asset.project_id).success ||
      asset.project_id !== projectId ||
      !["image", "video", "audio", "model"].includes(asset.kind) ||
      typeof asset.file_name !== "string" ||
      !Number.isSafeInteger(asset.byte_size) ||
      asset.byte_size <= 0 ||
      !Number.isInteger(asset.revision) ||
      asset.revision < 1 ||
      [asset.width, asset.height].some(
        (value) =>
          value !== undefined && (!Number.isFinite(value) || value <= 0),
      )
    )
      throw new Error("素材身份、项目或尺寸无效，请重新读取正式素材。");
    const position = {
      x: worldPosition.x + (index % 4) * 512,
      y: worldPosition.y + Math.floor(index / 4) * 452,
    };
    if (!coordinate(position.x) || !coordinate(position.y))
      throw new Error("画布位置超出允许范围。");
    const fileTitle =
      asset.file_name.split(/[\\/]/).at(-1)?.trim() ||
      { image: "图片", video: "视频", audio: "音频", model: "3D 模型" }[
        asset.kind
      ];
    return {
      id: crypto.randomUUID(),
      type: asset.kind as CanvasNodeType,
      title: Array.from(fileTitle).slice(0, 128).join(""),
      position,
      ...mediaDimensions(asset),
      zIndex: 0,
      assetId: asset.id,
      metadata: {},
    };
  });
}
