"use client";
import { useContext, useEffect, useId } from "react";
import useSWR, { useSWRConfig, unstable_serialize } from "swr";
import { useErrorNotification } from "@/components/canvas/runtime/components/feedback/error-notification";
import { CanvasMediaContext } from "../lib/canvas-media-context";
import { readCanvasFilePreview } from "../lib/canvas-file-preview";
import type { CanvasResourceReference } from "../lib/canvas-resource-references";
import { CANVAS_FILE_PREVIEW_CONCURRENCY } from "../constants/canvas-file-previews";

/** Grants belong to this mounted consumer, never to persisted nodes or another account. */
export function useCanvasReferencePreviews(
  references: CanvasResourceReference[],
) {
  const scope = useId();
  const context = useContext(CanvasMediaContext);
  const { cache } = useSWRConfig();
  const files = [
    ...new Map(
      references.flatMap((ref) =>
        ref.file
          ? [
              [
                `${ref.file.fileId}:${ref.file.version}`,
                { ...ref.file, nodeId: ref.nodeId },
              ] as const,
            ]
          : [],
      ),
    ).values(),
  ];
  const key = files.length
    ? [
        "canvas-reference-previews",
        scope,
        context?.canvasId,
        context?.bindingRevision,
        context?.snapshotId,
        files.map((file) => [
          file.fileId,
          file.version,
          file.nodeId,
          file.referenceKey,
        ]),
      ]
    : null;
  const query = useSWR(
    key,
    async () => {
      const urls: Record<string, string> = {};
      let error: unknown;
      // Grants are small control requests; do not fan out the entire graph at once.
      for (
        let start = 0;
        start < files.length;
        start += CANVAS_FILE_PREVIEW_CONCURRENCY
      ) {
        await Promise.all(
          files
            .slice(start, start + CANVAS_FILE_PREVIEW_CONCURRENCY)
            .map(async (file) => {
              try {
                urls[`${file.fileId}:${file.version}`] =
                  await readCanvasFilePreview(
                    file,
                    context
                      ? {
                          ...context,
                          nodeId: file.nodeId,
                          referenceKey: file.referenceKey,
                        }
                      : undefined,
                  );
              } catch (cause) {
                error ??= cause;
              }
            }),
        );
      }
      return { urls, error };
    },
    {
      revalidateOnMount: true,
      revalidateOnFocus: false,
      shouldRetryOnError: false,
      keepPreviousData: false,
    },
  );
  const { mutate } = query;
  const cacheKey = unstable_serialize(key);
  useEffect(
    () => () => {
      cache.delete(cacheKey);
    },
    [cache, cacheKey],
  );
  useErrorNotification(query.error ?? query.data?.error, "参考素材预览失败");
  return {
    references: references.map((ref) => ({
      ...ref,
      previewUrl: ref.file
        ? query.data?.urls[`${ref.file.fileId}:${ref.file.version}`]
        : ref.previewUrl,
    })),
    refresh: () => {
      void mutate();
    },
  };
}
