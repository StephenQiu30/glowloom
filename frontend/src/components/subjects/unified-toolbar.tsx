"use client";
import type { Ref } from "react";
import { ChevronDown } from "lucide-react";
import { Button } from "@/components/ui/button";
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
import type { SubjectOrder } from "./unified-contracts";
import { SubjectDateRange } from "./subject-date-range";
export type SubjectFilter = {
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
}: {
  batchTriggerRef?: Ref<HTMLButtonElement>;
  searching?: boolean;
  showBatchAction?: boolean;
  value: SubjectFilter;
  disabled: boolean;
  onChange: (filter: SubjectFilter) => void;
  onBatch: () => void;
}) {
  return (
    <div className="unified-subject-toolbar flex min-w-0 flex-1 flex-col gap-2">
      <div className="flex flex-wrap items-center gap-2">
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button
              variant={value.favorite ? "secondary" : "ghost"}
              disabled={disabled}
            >
              筛选
              <ChevronDown />
            </Button>
          </DropdownMenuTrigger>
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
            <Button variant="ghost" disabled={disabled || searching}>
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
        {showBatchAction ? (
          <Button
            variant="ghost"
            ref={batchTriggerRef}
            className="ml-auto"
            disabled={disabled}
            onClick={onBatch}
          >
            批量操作
          </Button>
        ) : null}
      </div>
    </div>
  );
}
