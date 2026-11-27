"use client";

import { PAGE_SIZE_OPTIONS as pageSizes } from "@/lib/pagination";
import { PageSizeSelect } from "@/components/ui/page-size-select";
import { ChevronLeft, ChevronRight } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  Pagination,
  PaginationContent,
  PaginationItem,
} from "@/components/ui/pagination";
export { pageSizes };
export type PageSize = (typeof pageSizes)[number];

/** Offset APIs show totals; cursor APIs expose only the known page and next-page availability. */
export function DataPagination({
  label,
  page,
  pageSize,
  total,
  hasNext = false,
  disabled = false,
  compact = false,
  onPageChange,
  onPageSizeChange,
}: {
  label: string;
  page: number;
  pageSize: PageSize;
  total?: number;
  hasNext?: boolean;
  disabled?: boolean;
  compact?: boolean;
  onPageChange: (page: number) => void;
  onPageSizeChange?: (size: PageSize) => void;
}) {
  const pages = total === undefined ? undefined : Math.ceil(total / pageSize);
  if (compact && !hasNext && (pages ?? page) <= 1) return null;
  const current =
    pages === 0 ? 0 : pages === undefined ? page : Math.min(page, pages);
  return (
    <Pagination
      aria-label={label}
      className="data-pagination"
      data-compact={compact || undefined}
    >
      {!compact && onPageSizeChange && (
        <PageSizeSelect
          label={`${label}每页数量`}
          value={pageSize}
          disabled={disabled}
          onChange={onPageSizeChange}
        />
      )}
      <span
        className={compact ? "sr-only" : "data-pagination-status"}
        role="status"
      >
        <span aria-hidden="true">
          {pages === 0
            ? "0 项"
            : pages === undefined
              ? `第 ${current} 页`
              : `${current} / ${pages}`}
        </span>
        <span className="sr-only">
          {pages === 0
            ? "暂无结果"
            : pages === undefined
              ? `第 ${current} 页`
              : `共 ${total} 项，第 ${current} 页，共 ${pages} 页`}
        </span>
      </span>
      <PaginationContent className="gap-2">
        <PaginationItem>
          <Button
            type="button"
            variant="outline"
            size="icon"
            className="size-9 max-md:size-11"
            aria-label="上一页"
            disabled={disabled || current <= 1}
            onClick={() => onPageChange(current - 1)}
          >
            <ChevronLeft />
          </Button>
        </PaginationItem>
        <PaginationItem>
          <Button
            type="button"
            variant="outline"
            size="icon"
            className="size-9 max-md:size-11"
            aria-label="下一页"
            disabled={
              disabled || (pages === undefined ? !hasNext : current >= pages)
            }
            onClick={() => onPageChange(current + 1)}
          >
            <ChevronRight />
          </Button>
        </PaginationItem>
      </PaginationContent>
    </Pagination>
  );
}
