import { useCallback, useEffect, useState } from "react";
import { listSubjectSources } from "@/api/subjects";
import { RequestError } from "@/lib/request";
import { subjectError } from "./unified-contracts";
import {
  requireSubjectSourcePage,
  type SubjectSourceItem,
  type SubjectSourcePage,
} from "./unified-source-contracts";

type Result = {
  key: string;
  page?: SubjectSourcePage;
  hasMore: boolean;
  error?: string;
  failedAttempt?: number;
};

// Keep the bounded API; the picker accumulates its own batches independently of the subject list.
export function useSubjectSources(
  actorID: string,
  scopeID: string,
  kind: SubjectSourceItem["kind"],
  group: string,
) {
  const batchSize = kind === "video" ? 50 : 30;
  const key = JSON.stringify([actorID, scopeID, kind, group]);
  const [position, setPosition] = useState({ key, page: 1, attempt: 0 });
  const [result, setResult] = useState<Result | null>(null);
  if (position.key !== key) {
    setPosition({ key, page: 1, attempt: 0 });
    setResult(null);
  }
  const pageIndex = position.key === key ? position.page : 1;
  const attempt = position.key === key ? position.attempt : 0;
  const current = result?.key === key ? result : null;
  const error = current?.failedAttempt === attempt ? current.error : undefined;
  const loading = !error && (!current?.page || current.page.page < pageIndex);

  useEffect(() => {
    const controller = new AbortController();
    listSubjectSources(
      {
        kind,
        favorite_only: group === "favorite" || undefined,
        quality: group === "upscaled" ? "upscaled" : undefined,
        page: pageIndex,
        page_size: batchSize,
      },
      { signal: controller.signal },
    )
      .then((value) => requireSubjectSourcePage(value, actorID, scopeID, kind))
      .then((page) => {
        if (controller.signal.aborted) return;
        if (page.page !== pageIndex || page.page_size !== batchSize)
          throw new RequestError("invalid_receipt");
        setResult((previous) => {
          const oldItems =
            pageIndex > 1 && previous?.key === key
              ? (previous.page?.items ?? [])
              : [];
          const ids = new Set(oldItems.map((item) => item.id));
          return {
            key,
            page: {
              ...page,
              items: [
                ...oldItems,
                ...page.items.filter((item) => !ids.has(item.id)),
              ],
            },
            // Count consumed pages, not unique IDs: a duplicate batch must not trap loading.
            hasMore:
              page.items.length > 0 && pageIndex * batchSize < page.total,
          };
        });
      })
      .catch((reason) => {
        if (controller.signal.aborted) return;
        setResult((previous) => ({
          ...(previous?.key === key ? previous : { key, hasMore: false }),
          error: subjectError(reason),
          failedAttempt: attempt,
        }));
      });
    return () => controller.abort();
  }, [actorID, scopeID, kind, group, key, batchSize, pageIndex, attempt]);

  const loadedPage = current?.page?.page ?? 0;
  const hasMore = current?.hasMore ?? false;
  const loadMore = useCallback(() => {
    if (!hasMore || loading || error) return;
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
  };
}
