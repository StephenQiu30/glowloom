"use client";
import { useContext, useEffect, useMemo, useState } from "react";
import { getMediaPreview } from "@/api/media";
import { requirePreview } from "@/components/media/library";
import { CanvasMediaContext } from "../lib/canvas-media-context";
import type { RichTextReference } from "../runtime/components/editor/types/rich-text";
import type { CanvasNodeData } from "../types/canvas";
export function useBoundReferences(node: CanvasNodeData): RichTextReference[] {
  const context = useContext(CanvasMediaContext);
  const composer = node.serviceData?.config?.composer;
  const projectID = context?.projectId;
  const [previews, setPreviews] = useState<Record<string, string>>({});
  const references = useMemo(
    () =>
      (composer?.references ?? []).map((reference) => {
        const allocation = composer?.labelState?.allocations?.[reference.id!];
        const label = `${({ image: "图片", video: "视频", audio: "音频", subject: "主体" } as Record<string, string>)[allocation?.labelKind ?? reference.mediaKind ?? "image"]}${allocation?.ordinal ?? ""}`;
        return {
          id: reference.id!,
          kind: reference.mediaKind as "image" | "video" | "audio",
          label,
          title: label,
          ...(reference.source?.kind === "media"
            ? { sourceId: reference.source.sourceId }
            : {}),
        };
      }),
    [composer],
  );
  useEffect(() => {
    if (!projectID) return;
    const abort = new AbortController();
    for (const reference of references) {
      if (!reference.sourceId) continue;
      const id = reference.sourceId;
      void getMediaPreview(
        { pid: projectID, asset_id: id },
        { signal: abort.signal },
      )
        .then((result) => {
          const url = requirePreview(result, id);
          if (!abort.signal.aborted)
            setPreviews((old) => ({ ...old, [reference.id]: url }));
        })
        .catch(() => {});
    }
    return () => abort.abort();
  }, [projectID, references]);
  return references.map((reference) => ({
    ...reference,
    previewUrl: previews[reference.id],
  }));
}
