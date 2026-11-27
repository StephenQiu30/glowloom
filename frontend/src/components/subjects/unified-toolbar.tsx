"use client";
import { Toolbar, ToolbarGroup } from "@/components/ui/toolbar";
import { CollectionFilterBar } from "@/components/layout/collection-filter-bar";
import type { Ref } from "react";
import { ChevronDown, X } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Separator } from "@/components/ui/separator";
import {
  DropdownMenu,
  DropdownMenuTrigger,
  DropdownMenuContent,
  DropdownMenuCheckboxItem,
  DropdownMenuLabel,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuSeparator,
} from "@/components/ui/dropdown-menu";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import {
  subjectTypes,
  type SubjectType,
  type SubjectOrder,
} from "./unified-contracts";
import { SubjectDateRange } from "./subject-date-range";
export type SubjectFilter = {
  type?: SubjectType | "";
  favorite: boolean;
  order: SubjectOrder;
  from: string;
  through: string;
};
export const defaultSubjectFilter: SubjectFilter = {
  favorite: false,
  order: "updated_desc",
  from: "",
  through: "",
};
export function UnifiedSubjectToolbar({
  value,
  disabled,
  onChange,
  onBatch,
  batchTriggerRef,
  searching = false,
  showBatchAction = true,
  showTypes = false,
}: {
  batchTriggerRef?: Ref<HTMLButtonElement>;
  searching?: boolean;
  showBatchAction?: boolean;
  showTypes?: boolean;
  value: SubjectFilter;
  disabled: boolean;
  onChange: (filter: SubjectFilter) => void;
  onBatch: () => void;
}) {
  const FilterBar = showTypes ? CollectionFilterBar : Toolbar;
  return (
    <FilterBar role="group" aria-label="主体筛选" className="flex-wrap">
      {showTypes && (
        <ToggleGroup
          variant="text"
          size="toolbar"
          spacing={1}
          type="single"
          value={value.type || "all"}
          className="max-w-full justify-start gap-0 overflow-x-auto"
          aria-label="主体类型"
          onValueChange={(type) => {
            if (type)
              onChange({
                ...value,
                type: type === "all" ? "" : (type as SubjectType),
              });
          }}
        >
          <ToggleGroupItem value="all" disabled={disabled}>
            全部
          </ToggleGroupItem>
          {subjectTypes.map((type) => (
            <ToggleGroupItem
              key={type.value}
              value={type.value}
              disabled={disabled}
            >
              {type.label}
            </ToggleGroupItem>
          ))}
        </ToggleGroup>
      )}
      {showTypes && (
        <Separator orientation="vertical" className="max-md:hidden" />
      )}
      <ToolbarGroup
        appearance="plain"
        className="flex-wrap"
        aria-label="筛选与排序"
      >
        <DropdownMenu>
          <div className="group relative inline-flex min-w-0">
            <DropdownMenuTrigger asChild>
              <Button
                size="toolbar"
                variant="filter"
                data-active={value.favorite || undefined}
                disabled={disabled}
                className={
                  value.favorite
                    ? "pr-[var(--toolbar-control-size)]"
                    : undefined
                }
              >
                {value.favorite ? "收藏" : "筛选"}
                <ChevronDown
                  className={
                    value.favorite
                      ? "pointer-events-none absolute right-3 group-focus-within:opacity-0 group-hover:opacity-0 max-md:opacity-0"
                      : undefined
                  }
                />
              </Button>
            </DropdownMenuTrigger>
            {value.favorite ? (
              <Button
                variant="ghost"
                size="toolbar-icon"
                disabled={disabled}
                aria-label="清空筛选"
                className="absolute top-0 right-0 h-full opacity-0 group-focus-within:opacity-100 group-hover:opacity-100 max-md:opacity-100"
                onClick={() => onChange({ ...value, favorite: false })}
              >
                <X />
              </Button>
            ) : null}
          </div>
          <DropdownMenuContent align="start" className="unified-subject-menu">
            <DropdownMenuCheckboxItem
              checked={value.favorite}
              onCheckedChange={(favorite) => onChange({ ...value, favorite })}
            >
              收藏
            </DropdownMenuCheckboxItem>
          </DropdownMenuContent>
        </DropdownMenu>
        <SubjectDateRange
          from={value.from}
          through={value.through}
          disabled={disabled}
          onChange={(from, through) => onChange({ ...value, from, through })}
        />
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button
              size="toolbar"
              variant="filter"
              disabled={disabled || searching}
            >
              排序
              <ChevronDown />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="start" className="unified-subject-menu">
            <DropdownMenuLabel>顺序</DropdownMenuLabel>
            <DropdownMenuRadioGroup
              value={value.order.endsWith("desc") ? "desc" : "asc"}
              onValueChange={(direction) =>
                onChange({
                  ...value,
                  order:
                    `${value.order.startsWith("created") ? "created" : "updated"}_${direction}` as SubjectOrder,
                })
              }
            >
              <DropdownMenuRadioItem value="desc">近-远</DropdownMenuRadioItem>
              <DropdownMenuRadioItem value="asc">远-近</DropdownMenuRadioItem>
            </DropdownMenuRadioGroup>
            <DropdownMenuSeparator />
            <DropdownMenuLabel>排序方式</DropdownMenuLabel>
            <DropdownMenuRadioGroup
              value={value.order.startsWith("created") ? "created" : "updated"}
              onValueChange={(field) =>
                onChange({
                  ...value,
                  order:
                    `${field}_${value.order.endsWith("desc") ? "desc" : "asc"}` as SubjectOrder,
                })
              }
            >
              <DropdownMenuRadioItem value="updated">
                修改时间
              </DropdownMenuRadioItem>
              <DropdownMenuRadioItem value="created">
                创建时间
              </DropdownMenuRadioItem>
            </DropdownMenuRadioGroup>
          </DropdownMenuContent>
        </DropdownMenu>
      </ToolbarGroup>
      {showBatchAction ? (
        <Button
          size="toolbar"
          variant="ghost"
          ref={batchTriggerRef}
          className="ml-auto"
          disabled={disabled}
          onClick={onBatch}
        >
          批量操作
        </Button>
      ) : null}
    </FilterBar>
  );
}
