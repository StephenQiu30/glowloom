"use client";
import {
  useEffect,
  useCallback,
  createContext,
  useReducer,
  useMemo,
  useRef,
  type Dispatch,
  type SetStateAction,
} from "react";
import {
  readCanvasFilePreviewGrant,
  type CanvasFilePreviewContext,
} from "../lib/canvas-file-preview";
import { CANVAS_FILE_PREVIEW_CONCURRENCY } from "../constants/canvas-file-previews";
import { notifyError } from "@/components/canvas/runtime/components/feedback/error-notification";
import { RequestError } from "@/lib/request";
import type { CanvasNodeData } from "../types/canvas";

export const CanvasPreviewRenewal = createContext<
  ((node: CanvasNodeData) => void) | null
>(null);
type Grant = Awaited<ReturnType<typeof readCanvasFilePreviewGrant>>;
type PreviewSession = {
  timers: Map<string, ReturnType<typeof setTimeout>>;
  retryable: Set<string>;

  active: boolean;
  attempted: Set<string>;
  urls: Map<string, Grant>;
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
  // Project media grants are pinned to the asset, independent of graph edits.
  const grantRevision = projectId ? projectId : bindingRevision;
  const referenceContext = useMemo(
    () =>
      canvasId != null && grantRevision != null
        ? { canvasId, bindingRevision: grantRevision, snapshotId, projectId }
        : undefined,
    [canvasId, grantRevision, snapshotId, projectId],
  );
  const currentSession = useRef<PreviewSession | null>(null);
  const [revision, wake] = useReducer((v: number) => v + 1, 0);
  const running = useRef(0);
  useEffect(() => {
    if (!enabled) return;
    const session: PreviewSession = {
      active: true,
      timers: new Map(),
      retryable: new Set(),
      attempted: new Set(),
      urls: new Map(),
      queue: [],
      drain: () => {},
    };
    currentSession.current = session;
    function online() {
      for (const key of session.retryable) {
        session.attempted.delete(key);
      }
      session.retryable.clear();
      wake();
    }
    window.addEventListener("online", online);
    return () => {
      window.removeEventListener("online", online);
      for (const timer of session.timers.values()) clearTimeout(timer);
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
      clearTimeout(session.timers.get(key));
      session.timers.delete(key);
      session.retryable.delete(key);
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
                session.urls.get(previewKey(node, ref))?.url ||
                existing?.content ||
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
            reference && session.urls.get(previewKey(node, reference))?.url;
          if (
            !url ||
            (node.metadata?.content === url &&
              node.metadata?.status === "success")
          )
            return node;
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
        if (node.type !== "image" && reference !== refs[0]) continue;
        const key = previewKey(node, reference);
        const cached = session.urls.get(key);
        if (cached) {
          const visible =
            refs.length > 1 && node.type === "image"
              ? node.metadata?.images?.find(
                  (i) => i.id === reference.referenceKey,
                )?.content
              : node.metadata?.content;
          if (visible !== cached.url || node.metadata?.status !== "success")
            missingCachedPreview = true;
          continue;
        }
        if (session.attempted.has(key)) continue;
        session.attempted.add(key);
        session.queue.push(async () => {
          if (!session.active || !session.attempted.has(key)) return;
          try {
            const grant = await readCanvasFilePreviewGrant(
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
            session.urls.set(key, grant);
            session.retryable.delete(key);
            clearTimeout(session.timers.get(key));
            session.timers.set(
              key,
              setTimeout(
                () => {
                  session.urls.delete(key);
                  session.attempted.delete(key);
                  wake();
                },
                Math.max(1000, grant.expiresAt - Date.now()),
              ),
            );

            restore();
          } catch (error) {
            if (!session.active) return;
            notifyError(error, "画布素材预览失败");
            const status = error instanceof RequestError ? error.status : 0;
            if (
              status === 0 ||
              status === 408 ||
              status === 429 ||
              status >= 500
            ) {
              session.retryable.add(key);
              clearTimeout(session.timers.get(key));
              session.timers.set(
                key,
                setTimeout(() => {
                  session.attempted.delete(key);
                  wake();
                }, 10000),
              );
            }
          }
        });
      }
    }
    if (missingCachedPreview) restore();
    drain();
  }, [nodes, enabled, setNodes, referenceContext, revision]);
  return useCallback((node: CanvasNodeData) => {
    const session = currentSession.current;
    if (!session) return;
    for (const ref of contentReferences(node)) {
      const key = previewKey(node, ref);
      if (!session.urls.has(key) && !session.retryable.has(key)) continue;
      clearTimeout(session.timers.get(key));
      session.urls.delete(key);
      session.attempted.delete(key);
    }
    wake();
  }, []);
}
