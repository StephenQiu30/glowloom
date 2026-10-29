import { useCallback, useEffect, useState } from "react";
import { listSubjects } from "@/api/subjects";
import { RequestError } from "@/lib/request";
import {
  requireUnifiedSubjectPage,
  subjectDateRange,
  subjectError,
  type UnifiedSubjectPage,
} from "./unified-contracts";
import type { SubjectFilter } from "./unified-toolbar";

const BATCH_SIZE = 20;
type Result = {
  key: string;
  page?: UnifiedSubjectPage;
  hasMore: boolean;
  error?: string;
  failedAttempt?: number;
};

// The API keeps its bounded offset contract; only the subject view accumulates batches.
export function useSubjectList(
  actorID: string,
  scopeID: string,
  query: string,
  filter: SubjectFilter,
) {
  const [revision, setRevision] = useState(0);
  const key = JSON.stringify([actorID, scopeID, query, filter, revision]);
  const [position, setPosition] = useState({ key, page: 1, attempt: 0 });
  const pageIndex = position.key === key ? position.page : 1;
  const attempt = position.key === key ? position.attempt : 0;
  const [result, setResult] = useState<Result | null>(null);
  if (position.key !== key) {
    setPosition({ key, page: 1, attempt: 0 });
    setResult(null);
  }
  const current = result?.key === key ? result : null;
  const error = current?.failedAttempt === attempt ? current.error : undefined;
  const loading = !error && (!current?.page || current.page.page < pageIndex);

  useEffect(() => {
    const controller = new AbortController();
    listSubjects(
      {
        q: query.trim() || undefined,
        favorite: filter.favorite || undefined,
        ...subjectDateRange(filter.from, filter.through),
        time_field: filter.order.startsWith("created") ? "created" : "updated",
        order: filter.order,
        page: pageIndex,
        page_size: BATCH_SIZE,
      },
      { signal: controller.signal },
    )
      .then((value) => requireUnifiedSubjectPage(value, actorID, scopeID))
      .then((page) => {
        if (controller.signal.aborted) return;
        if (page.page !== pageIndex || page.page_size !== BATCH_SIZE)
          throw new Error("subject_batch_mismatch");
        setResult((previous) => {
          const oldItems =
            pageIndex > 1 && previous?.key === key
              ? (previous.page?.items ?? [])
              : [];
          const ids = new Set(oldItems.map((item) => item.id));
          const added = page.items.filter((item) => !ids.has(item.id));
          return {
            key,
            page: { ...page, items: [...oldItems, ...added] },
            hasMore:
              page.items.length > 0 && pageIndex * BATCH_SIZE < page.total,
          };
        });
      })
      .catch((reason) => {
        if (controller.signal.aborted) return;
        setResult((previous) => ({
          ...(previous?.key === key ? previous : { key, hasMore: false }),
          error:
            reason instanceof RequestError && [401, 403].includes(reason.status)
              ? subjectError(reason)
              : "加载失败，请重试。",
          failedAttempt: attempt,
        }));
      });
    return () => controller.abort();
  }, [actorID, scopeID, query, filter, key, pageIndex, attempt]);

  const loadedPage = current?.page?.page ?? 0;
  const hasMore = current?.hasMore ?? false;
  const loadMore = useCallback(() => {
    if (!hasMore || loading || error) return;
    // Multiple observer callbacks in the same frame still request exactly one batch.
    setPosition((previous) =>
      previous.key === key && previous.page > loadedPage
        ? previous
        : { key, page: loadedPage + 1, attempt: 0 },
    );
  }, [key, loadedPage, hasMore, loading, error]);

  return {
    page: current?.page,
    error,
    loading,
    hasMore,
    loadMore,
    retry: () => setPosition({ key, page: pageIndex, attempt: attempt + 1 }),
    refresh: () => setRevision((value) => value + 1),
  };
}
