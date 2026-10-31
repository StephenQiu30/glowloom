import * as files from "@/components/canvas/runtime/api/file-bff";
import { unwrap } from "@/components/canvas/runtime/lib/request";

export function assetMedia(asset: CanvasContract.AssetVO) {
  try {
    const value: unknown = JSON.parse(asset.mediaJson || "{}");
    if (!value || typeof value !== "object") return {};
    return value as {
      size?: number;
      contentType?: string;
      fileName?: string;
      media?: { width?: number; height?: number; duration?: number };
    };
  } catch {
    return {};
  }
}

export async function assetReadUrl(
  asset: CanvasContract.AssetVO,
  signal?: AbortSignal,
) {
  if (!asset.fileId || !asset.fileVersion)
    throw new Error("素材没有固定文件版本");
  const grant = unwrap(
    await files.readFileTransfer({ fileId: asset.fileId }, { signal }),
  );
  signal?.throwIfAborted();
  if (!grant.url?.startsWith("/api/file/content?ticket="))
    throw new Error("读取授权无效");
  return grant.url;
}
