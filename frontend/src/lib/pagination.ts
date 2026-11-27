export const PAGE_SIZE_OPTIONS = [10, 20, 50] as const;
export const DEFAULT_PAGE_SIZE = 20;

export function appendPageItems<T extends { id: string }>(
  current: readonly T[],
  incoming: readonly T[],
) {
  const seen = new Set(current.map((item) => item.id));
  return [...current, ...incoming.filter((item) => !seen.has(item.id))];
}

export function nextPageCursor(
  cursor: string | null | undefined,
  receivedCount: number,
  visited: ReadonlySet<string>,
) {
  return cursor && receivedCount > 0 && !visited.has(cursor) ? cursor : null;
}

export function paginationRange(
  total: number,
  current: number,
  pageSize: number,
) {
  const pages = Math.max(1, Math.ceil(total / pageSize));
  const page = Math.max(1, Math.min(current, pages));
  return {
    page,
    pages,
    start: total === 0 ? 0 : (page - 1) * pageSize + 1,
    end: Math.min(page * pageSize, total),
  };
}
