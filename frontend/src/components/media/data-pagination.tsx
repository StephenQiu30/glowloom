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

export const pageSizes = [12, 24, 48] as const;
export type PageSize = (typeof pageSizes)[number];

/** Offset APIs show totals; cursor APIs expose only the known page and next-page availability. */
export function DataPagination({
  label,
  page,
  pageSize,
  total,
  hasNext = false,
  disabled = false,
  onPageChange,
  onPageSizeChange,
}: {
  label: string;
  page: number;
  pageSize: PageSize;
  total?: number;
  hasNext?: boolean;
  disabled?: boolean;
  onPageChange: (page: number) => void;
  onPageSizeChange: (size: PageSize) => void;
}) {
  const id = useId();
  const pages = total === undefined ? undefined : Math.ceil(total / pageSize);
  const current =
    pages === 0 ? 0 : pages === undefined ? page : Math.min(page, pages);
  return (
    <Pagination aria-label={label} className="data-pagination">
      <Field orientation="horizontal" className="w-auto gap-2">
        <FieldLabel htmlFor={id} className="max-sm:sr-only">
          每页
        </FieldLabel>
        <Select
          value={String(pageSize)}
          disabled={disabled}
          onValueChange={(value) => {
            const size = pageSizes.find((size) => String(size) === value);
            if (size) onPageSizeChange(size);
          }}
        >
          <SelectTrigger
            id={id}
            aria-label={`${label}每页数量`}
            className="w-20 text-sm data-[size=default]:h-9 max-md:data-[size=default]:h-11"
          >
            <SelectValue />
          </SelectTrigger>
          <SelectContent align="end">
            <SelectGroup>
              {pageSizes.map((size) => (
                <SelectItem key={size} value={String(size)}>
                  {size}
                </SelectItem>
              ))}
            </SelectGroup>
          </SelectContent>
        </Select>
      </Field>
      <span className="data-pagination-status" role="status">
        <span aria-hidden="true">
          {pages === undefined ? `第 ${current} 页` : `${current} / ${pages}`}
        </span>
        <span className="sr-only">
          {pages === undefined
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
