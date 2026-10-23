"use client";

import { useId } from "react";
import { ChevronLeft, ChevronRight } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Field, FieldLabel } from "@/components/ui/field";
import {
  Pagination,
  PaginationContent,
  PaginationItem,
} from "@/components/ui/pagination";
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  PAGE_SIZE_OPTIONS,
  paginationRange,
} from "@/components/canvas/runtime/lib/pagination";

type NumberedProps = {
  label: string;
  total: number;
  current: number;
  pageSize: number;
  disabled?: boolean;
  pending?: boolean;
  pageSizeOptions?: readonly number[];
  onPageChange: (page: number) => void;
  onPageSizeChange: (pageSize: number) => void;
};

type CursorProps = Omit<NumberedProps, "total"> & {
  mode: "cursor";
  itemCount?: number;
  hasNextPage: boolean;
};

export function ListPagination(props: NumberedProps | CursorProps) {
  const {
    label,
    current,
    pageSize,
    onPageChange,
    onPageSizeChange,
    pending = false,
    pageSizeOptions = PAGE_SIZE_OPTIONS,
  } = props;
  const disabled = props.disabled || pending;
  const sizeId = useId();
  const cursor = "mode" in props;
  const { page, pages, start, end } = paginationRange(
    cursor ? 0 : props.total,
    current,
    pageSize,
  );
  const activePage = cursor ? current : page;
  const summary = cursor
    ? props.itemCount === undefined
      ? "条数暂不可用"
      : `本页 ${props.itemCount} 条`
    : `第 ${start}–${end} 条，共 ${props.total} 条`;
  const canNext = cursor ? props.hasNextPage : page < pages;
  return (
    <Pagination
      aria-label={label}
      className="workspace-pagination flex-wrap items-center justify-between gap-3"
    >
      <span
        className="text-sm text-muted-foreground tabular-nums"
        aria-live="polite"
      >
        {pending ? "正在加载" : summary}
      </span>
      <div className="flex flex-wrap items-center gap-3">
        <Field
          orientation="horizontal"
          className="w-auto"
          data-disabled={disabled}
        >
          <FieldLabel htmlFor={sizeId}>每页</FieldLabel>
          <Select
            value={String(pageSize)}
            disabled={disabled}
            onValueChange={(value) => onPageSizeChange(Number(value))}
          >
            <SelectTrigger
              id={sizeId}
              size="sm"
              className="w-20 shrink-0"
              aria-label={cursor ? "每页条数" : `${label}每页数量`}
            >
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectGroup>
                {pageSizeOptions.map((size) => (
                  <SelectItem key={size} value={String(size)}>
                    {size} 条
                  </SelectItem>
                ))}
              </SelectGroup>
            </SelectContent>
          </Select>
        </Field>
        <PaginationContent>
          <PaginationItem>
            <Button
              type="button"
              variant="outline"
              size="sm"
              aria-label="上一页"
              disabled={disabled || activePage <= 1}
              onClick={() => onPageChange(activePage - 1)}
            >
              <ChevronLeft data-icon="inline-start" />
              <span className="workspace-pagination-direction">上一页</span>
            </Button>
          </PaginationItem>
          <PaginationItem
            className="min-w-24 shrink-0 px-2 text-center text-sm whitespace-nowrap tabular-nums"
            aria-live="polite"
          >
            {cursor ? `第 ${activePage} 页` : `第 ${page} / ${pages} 页`}
          </PaginationItem>
          <PaginationItem>
            <Button
              type="button"
              variant="outline"
              size="sm"
              aria-label="下一页"
              disabled={disabled || !canNext}
              onClick={() => onPageChange(activePage + 1)}
            >
              <span className="workspace-pagination-direction">下一页</span>
              <ChevronRight data-icon="inline-end" />
            </Button>
          </PaginationItem>
        </PaginationContent>
      </div>
    </Pagination>
  );
}
