import {
  DEFAULT_PAGE_SIZE,
  appendPageItems,
  nextPageCursor,
} from "@/lib/pagination";
import { useEffect, useState } from "react";
import { listSubjects } from "@/api/subjects";
import { RequestError } from "@/lib/request";
import {
  requireUnifiedSubjectCursorPage,
  subjectDateRange,
  subjectError,
  type UnifiedSubjectCursorPage,
} from "./unified-contracts";
import type { SubjectFilter } from "./unified-toolbar";

type Position = { key: string; cursor?: string; attempt: number };
type Result = Position & {
  page?: UnifiedSubjectCursorPage;
  loadedCursors?: ReadonlySet<string>;
  failure?: { status: number; message: string };
};

export function useSubjectList(
  actorID: string,
  scopeID: string,
  query: string,
  filter: SubjectFilter,
  limit = DEFAULT_PAGE_SIZE,
) {
  const [revision, setRevision] = useState(0);
  const key = JSON.stringify([
    actorID,
    scopeID,
    query,
    filter,
    limit,
    revision,
  ]);
  const [position, setPosition] = useState<Position>({ key, attempt: 0 });
  const [result, setResult] = useState<Result | null>(null);
  if (position.key !== key) {
    setPosition({ key, attempt: 0 });
    setResult(null);
  }
  const cursor = position.key === key ? position.cursor : undefined;
  const attempt = position.key === key ? position.attempt : 0;
  const current = result?.key === key ? result : null;
  const settled = current?.cursor === cursor && current?.attempt === attempt;
  const failure = settled ? current?.failure : undefined;

  useEffect(() => {
    const controller = new AbortController();
    listSubjects(
      {
        q: query.trim() || undefined,
        type: filter.type || undefined,
        favorite: filter.favorite || undefined,
        ...subjectDateRange(filter.from, filter.through),
        time_field: filter.order.startsWith("created") ? "created" : "updated",
        order: filter.order,
        limit,
        cursor,
      },
      { signal: controller.signal },
    )
      .then((value) =>
        requireUnifiedSubjectCursorPage(value, actorID, scopeID, limit),
      )
      .then((page) => {
        if (controller.signal.aborted) return;
        setResult((previous) => {
          const prior = cursor && previous?.key === key ? previous : undefined;
          const oldItems = prior?.page?.items ?? [];
          const loadedCursors = new Set(prior?.loadedCursors);
          if (cursor) loadedCursors.add(cursor);
          const next = nextPageCursor(
            page.next_cursor,
            page.items.length,
            loadedCursors,
          );
          return {
            key,
            cursor,
            attempt,
            page: {
              ...page,
              items: appendPageItems(oldItems, page.items),
              next_cursor: next,
            },
            loadedCursors,
          };
        });
      })
      .catch((reason) => {
        if (controller.signal.aborted) return;
        const status = reason instanceof RequestError ? reason.status : 0;
        const denied = status === 401 || status === 403;
        setResult((previous) => ({
          ...(!denied && previous?.key === key ? previous : undefined),
          key,
          cursor,
          attempt,
          failure: {
            status,
            message: denied ? subjectError(reason) : "加载失败，请重试。",
          },
        }));
      });
    return () => controller.abort();
  }, [actorID, scopeID, query, filter, key, cursor, attempt, limit]);

  const next = current?.page?.next_cursor;
  return {
    page: current?.page,
    hasMore: !!next,
    error: failure?.message,
    loading: !settled,
    restartRequired:
      !!failure && (failure.status === 400 || failure.status === 422),
    loadMore: () => {
      if (settled && !failure && next)
        setPosition({ key, cursor: next, attempt: 0 });
    },
    retry: () => setPosition({ key, cursor, attempt: attempt + 1 }),
    refresh: () => setRevision((value) => value + 1),
  };
}
