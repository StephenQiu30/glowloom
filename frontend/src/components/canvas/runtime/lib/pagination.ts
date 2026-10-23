export const PAGE_SIZE_OPTIONS = [12, 24, 36, 48] as const;
export const DEFAULT_PAGE_SIZE = 12;

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
