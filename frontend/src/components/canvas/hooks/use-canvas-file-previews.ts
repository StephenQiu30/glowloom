"use client";
import {
  useEffect,
  useMemo,
  useRef,
  type Dispatch,
  type SetStateAction,
} from "react";
import {
  readCanvasFilePreview,
  type CanvasFilePreviewContext,
} from "../lib/canvas-file-preview";
import { CANVAS_FILE_PREVIEW_CONCURRENCY } from "../constants/canvas-file-previews";
import { notifyError } from "@/components/canvas/runtime/components/feedback/error-notification";
import type { CanvasNodeData } from "../types/canvas";

type PreviewSession = {
  active: boolean;
  attempted: Set<string>;
  urls: Map<string, string>;
  queue: Array<() => Promise<void>>;
  drain: () => void;
};

function contentReferences(node: CanvasNodeData) {
  return (
    node.documentData?.fileReferences?.filter(
      (reference) => reference.purpose === "CONTENT",
    ) ?? []
  );
}

function previewKey(
  node: CanvasNodeData,
  reference: ReturnType<typeof contentReferences>[number],
) {
  return JSON.stringify([
    node.id,
    reference.referenceKey,
    reference.fileId,
    reference.version,
    reference.publicAssetId ?? null,
    reference.publicRevision ?? null,
  ]);
}

// Preview grants never become graph content. Only the pinned file version is persisted.
export function useCanvasFilePreviews(
  nodes: CanvasNodeData[],
  enabled: boolean,
  setNodes: Dispatch<SetStateAction<CanvasNodeData[]>>,
  context?: CanvasFilePreviewContext,
) {
  const { canvasId, bindingRevision, snapshotId, projectId } = context ?? {};
  const referenceContext = useMemo(
    () =>
      canvasId != null && bindingRevision != null
        ? { canvasId, bindingRevision, snapshotId, projectId }
        : undefined,
    [canvasId, bindingRevision, snapshotId, projectId],
  );
  const currentSession = useRef<PreviewSession | null>(null);
  const running = useRef(0);
  useEffect(() => {
    if (!enabled) return;
    const session: PreviewSession = {
      active: true,
      attempted: new Set(),
      urls: new Map(),
      queue: [],
      drain: () => {},
    };
    currentSession.current = session;
    return () => {
      session.active = false;
      session.queue = [];
      session.urls.clear();
      session.attempted.clear();
      if (currentSession.current === session) currentSession.current = null;
    };
  }, [enabled, referenceContext]);
  useEffect(() => {
    const session = currentSession.current;
    if (!enabled || !session) return;
    const identities = new Set(
      nodes.flatMap((node) => {
        return contentReferences(node).map((ref) => previewKey(node, ref));
      }),
    );
    for (const key of session.attempted) {
      if (identities.has(key)) continue;
      session.attempted.delete(key);
      session.urls.delete(key);
    }
    const restore = () => {
      if (!session.active) return;
      setNodes((latest) => {
        let changed = false;
        const restored = latest.map((node) => {
          const refs = contentReferences(node);
          if (node.type === "image" && refs.length > 1) {
            const previous = node.metadata?.images ?? [];
            const images = refs.map((ref) => {
              const existing = previous.find(
                (item) => item.id === ref.referenceKey,
              );
              const content =
                existing?.content ||
                session.urls.get(previewKey(node, ref)) ||
                "";
              if (existing && existing.content === content) return existing;
              return {
                id: ref.referenceKey,
                content,
                status: content ? ("success" as const) : ("loading" as const),
                naturalWidth: node.width,
                naturalHeight: node.height,
                bytes: 0,
                mimeType: "image/*",
              };
            });
            if (
              previous.length === images.length &&
              images.every((img, i) => img === previous[i])
            )
              return node;
            changed = true;
            return {
              ...node,
              metadata: {
                ...node.metadata,
                images,
                primaryImageId: refs[0].referenceKey,
                content: images[0].content,
                status: images[0].status,
              },
            };
          }
          const reference = refs[0];
          const url =
            reference && session.urls.get(previewKey(node, reference));
          if (!url || node.metadata?.content) return node;
          changed = true;
          return {
            ...node,
            metadata: {
              ...node.metadata,
              content: url,
              status: "success" as const,
            },
          };
        });
        return changed ? restored : latest;
      });
    };
    const drain = () => {
      while (
        session.active &&
        running.current < CANVAS_FILE_PREVIEW_CONCURRENCY &&
        session.queue.length
      ) {
        running.current++;
        void session.queue.shift()!().finally(() => {
          running.current--;
          currentSession.current?.drain();
        });
      }
    };
    session.drain = drain;
    let missingCachedPreview = false;
    for (const node of nodes) {
      const refs = contentReferences(node);
      for (const reference of refs) {
        if (
          refs.length > 1 && node.type === "image"
            ? node.metadata?.images?.some(
                (img) => img.id === reference.referenceKey && img.content,
              )
            : node.metadata?.content || reference !== refs[0]
        )
          continue;
        const key = previewKey(node, reference);
        if (session.urls.has(key)) {
          missingCachedPreview = true;
          continue;
        }
        if (session.attempted.has(key)) continue;
        session.attempted.add(key);
        session.queue.push(async () => {
          if (!session.active || !session.attempted.has(key)) return;
          try {
            const url = await readCanvasFilePreview(
              reference,
              referenceContext
                ? {
                    ...referenceContext,
                    nodeId: node.id,
                    referenceKey: reference.referenceKey,
                  }
                : undefined,
            );
            if (!session.active || !session.attempted.has(key)) return;
            session.urls.set(key, url);
            restore();
          } catch (error) {
            if (session.active) notifyError(error, "画布素材预览失败");
          }
        });
      }
    }
    if (missingCachedPreview) restore();
    drain();
  }, [nodes, enabled, setNodes, referenceContext]);
}
