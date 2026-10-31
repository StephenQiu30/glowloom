"use client";

import { readFileTransfer } from "@/components/canvas/runtime/api/file-bff";
import { unwrap } from "@/components/canvas/runtime/lib/request";
import { toast } from "sonner";

export function startFileDownload(url: string) {
  if (!url.startsWith("/api/file/content?ticket="))
    throw new Error("文件下载授权无效");
  const anchor = document.createElement("a");
  anchor.href = url;
  // An empty attribute preserves the authoritative Content-Disposition filename.
  anchor.download = "";
  anchor.rel = "noreferrer";
  document.body.append(anchor);
  anchor.click();
  anchor.remove();
  toast.info("已发起下载");
}

export async function downloadFixedFile(
  reference: { fileId?: string; fileVersion?: string },
  signal?: AbortSignal,
) {
  if (!reference.fileId || !reference.fileVersion)
    throw new Error("素材没有已发布的固定文件版本");
  const grant = unwrap(
    await readFileTransfer({ fileId: reference.fileId }, { signal }),
  );
  signal?.throwIfAborted();
  if (!grant.url?.startsWith("/api/file/content?ticket="))
    throw new Error("原件下载授权无效");
  startFileDownload(grant.url);
}
