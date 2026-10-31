"use client";
import { useEffect, useRef, useState } from "react";
import useLiveQuery, {
  matchesProjection,
} from "@/components/canvas/runtime/lib/use-live-query";
import { onlineProjections } from "@/components/canvas/runtime/lib/online-projections";
import * as scanApi from "@/components/canvas/runtime/api/canvas-history-scan";
import * as api from "@/components/canvas/runtime/api/canvas-document-bff";
import { unwrap } from "@/components/canvas/runtime/lib/request";
import type { CanvasScope } from "./use-canvas-document";

export function useCanvasHistory({
  open,
  enabled,
  scope,
  identity,
  document,
}: {
  open: boolean;
  enabled: boolean;
  scope: CanvasScope;
  identity: string;
  document: CanvasContract.CanvasVO;
}) {
  const [browse, setBrowse] = useState({
    page: 1,
    cursors: [null] as (string | null)[],
    epoch: 0,
  });
  const page = browse.page;
  const goneRecoveries = useRef(0);
  const handles = useRef(new Map<number, { start: string; close: string }>());
  const release = (epoch: number) => {
    const cursor = handles.current.get(epoch)?.close;
    handles.current.delete(epoch);
    if (cursor)
      void scanApi.closeCanvasCheckpointScan({ cursor }).catch(() => undefined);
  };
  const restart = () => {
    goneRecoveries.current = 0;
    release(browse.epoch);
    setBrowse((previous) => ({
      page: 1,
      cursors: [null],
      epoch: previous.epoch + 1,
    }));
  };
  useEffect(() => {
    const handlesForEpoch = handles.current;
    const epoch = browse.epoch;
    return () => {
      const cursor = handlesForEpoch.get(epoch)?.close;
      handlesForEpoch.delete(epoch);
      if (cursor)
        void scanApi
          .closeCanvasCheckpointScan({ cursor })
          .catch(() => undefined);
    };
  }, [browse.epoch]);
  const canvasId = String(document.canvasId);
  useEffect(() => {
    if (!open || !enabled || scope !== "USER") return;
    return onlineProjections.subscribe((event) => {
      if (
        event.type !== "recover" &&
        matchesProjection(
          ["canvas-version-history", identity, scope, canvasId],
          event,
        )
      ) {
        goneRecoveries.current = 0;
        setBrowse((previous) => ({
          page: 1,
          cursors: [null],
          epoch: previous.epoch + 1,
        }));
      }
    });
  }, [open, enabled, scope, identity, canvasId]);
  const history = useLiveQuery(
    enabled && open
      ? [
          "canvas-version-history",
          identity,
          scope,
          canvasId,
          document.revision,
          document.projectBindingRevision,
          page,
          browse.epoch,
          browse.cursors[page - 1],
        ]
      : null,
    async (key, options = {}) => {
      if (!key) throw new Error("版本历史范围不可用");
      const [, , readScope, readCanvas, , , readPage, readEpoch, cursor] = key;
      if (readScope === "LOCAL") {
        const local = unwrap(
          await api.localListCanvasCheckpoints(
            {
              canvasId: String(readCanvas),
              current: Number(readPage),
              pageSize: 10,
            },
            options,
          ),
        );
        return {
          ...local,
          hasMore: Number(readPage) * 10 < Number(local.total),
          nextCursor: undefined as string | undefined,
        };
      }
      const start = cursor
        ? String(cursor)
        : handles.current.get(Number(readEpoch))?.start;
      const slice = unwrap(
        await scanApi.scanCanvasCheckpoints(
          start
            ? { cursor: start }
            : {
                canvasId: String(readCanvas),
                pageSize: 10,
                includeTotal: false,
              },
          options,
        ),
      );
      if (slice.closeCursor && slice.startCursor)
        handles.current.set(Number(readEpoch), {
          start: slice.startCursor,
          close: slice.closeCursor,
        });
      return slice;
    },
    {
      shouldRetryOnError: false,
      revalidateOnFocus: false,
      onError: (error) => {
        if (
          Math.floor(Number(error?.code) / 100) === 410 &&
          goneRecoveries.current < 1
        ) {
          goneRecoveries.current++;
          setBrowse((previous) =>
            previous.epoch === browse.epoch
              ? { page: 1, cursors: [null], epoch: previous.epoch + 1 }
              : previous,
          );
        }
      },
    },
  );
  return {
    history,
    page,
    restart,
    previous: () =>
      setBrowse((previous) => ({
        ...previous,
        page: Math.max(1, previous.page - 1),
      })),
    next: () => {
      if (!history.data?.hasMore) return;
      const next = history.data.nextCursor;
      setBrowse((previous) => ({
        ...previous,
        page: previous.page + 1,
        cursors: [...previous.cursors.slice(0, previous.page), next ?? null],
      }));
    },
  };
}
