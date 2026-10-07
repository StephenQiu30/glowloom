"use client";
import { useState } from "react";
import { Button } from "@/components/ui/button";
import { Search, SlidersHorizontal } from "lucide-react";
import {
  InputGroup,
  InputGroupAddon,
  InputGroupInput,
} from "@/components/ui/input-group";
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible";
import { Checkbox } from "@/components/ui/checkbox";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import { Field, FieldGroup, FieldLabel } from "@/components/ui/field";
import { LibrarySelect } from "./library-select";
import {
  folderAncestry,
  categoryLabels,
  kindLabels,
  libraryKinds,
  libraryCategories,
  type LibraryFilter,
  type LibraryScope,
  type LibraryPage,
  type LibraryFolder,
} from "./library-model";
export function LibraryFilters({
  scope,
  filter,
  view,
  page,
  folders,
  disabled,
  onChange,
}: {
  scope: LibraryScope;
  filter: LibraryFilter;
  view: string;
  page?: LibraryPage;
  folders: LibraryFolder[];
  disabled: boolean;
  onChange: (
    changes: Record<string, string | undefined>,
    reset?: boolean,
  ) => void;
}) {
  const [search, setSearch] = useState(filter.search);
  return (
    <div className="flex flex-col gap-3">
      <form
        onSubmit={(event) => {
          event.preventDefault();
          onChange({ search });
        }}
      >
        <FieldGroup>
          <Field orientation="horizontal">
            <FieldLabel htmlFor="library-search" className="sr-only">
              搜索完整素材库
            </FieldLabel>
            <InputGroup className="min-w-0 flex-1">
              <InputGroupAddon>
                <Search aria-hidden />
              </InputGroupAddon>
              <InputGroupInput
                id="library-search"
                value={search}
                disabled={disabled}
                onChange={(event) => setSearch(event.target.value)}
                placeholder="标题、标签、来源、备注、正文或MIME"
              />
            </InputGroup>
            <Button type="submit" variant="outline" disabled={disabled}>
              搜索
            </Button>
          </Field>
        </FieldGroup>
      </form>
      <div className="flex min-w-0 flex-wrap items-center justify-between gap-3">
        <ToggleGroup
          type="single"
          value={filter.kind || "all"}
          disabled={disabled}
          aria-label="素材类型"
          className="flex-wrap"
          onValueChange={(value) => {
            if (value) onChange({ kind: value === "all" ? undefined : value });
          }}
        >
          <ToggleGroupItem value="all">全部</ToggleGroupItem>
          {libraryKinds.map((kind) => (
            <ToggleGroupItem key={kind} value={kind}>
              {kindLabels[kind]}
            </ToggleGroupItem>
          ))}
        </ToggleGroup>
      </div>
      <Collapsible className="flex flex-col gap-3">
        <CollapsibleTrigger asChild>
          <Button variant="ghost" className="self-start">
            <SlidersHorizontal data-icon="inline-start" />
            筛选与视图
          </Button>
        </CollapsibleTrigger>
        <CollapsibleContent className="flex flex-col gap-3">
          <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-3">
            <LibrarySelect
              label="业务分类筛选"
              value={filter.category || "all"}
              options={[
                { value: "all", label: "全部业务分类" },
                ...libraryCategories.map((value) => ({
                  value,
                  label: `${categoryLabels[value]} · ${page?.category_counts[value] ?? 0}`,
                })),
              ]}
              disabled={disabled}
              onChange={(value) =>
                onChange({ category: value === "all" ? undefined : value })
              }
            />
            <LibrarySelect
              label="目录筛选"
              value={filter.folder}
              options={[
                { value: "all", label: "全部目录" },
                {
                  value: "root",
                  label: `未分类 · ${page?.folder_counts.root ?? 0}`,
                },
                ...folders.map((folder) => ({
                  value: folder.id,
                  label: `${folderAncestry(folders, folder.id)
                    .map((ancestor) => ancestor.name)
                    .join(" / ")} · ${page?.folder_counts[folder.id] ?? 0}`,
                })),
              ]}
              disabled={disabled}
              onChange={(value) => onChange({ folder: value })}
            />
            <LibrarySelect
              label="全库排序"
              value={filter.order}
              options={[
                { value: "updated_desc", label: "最新更新优先" },
                { value: "updated_asc", label: "最早更新优先" },
                { value: "name_asc", label: "名称顺序" },
              ]}
              disabled={disabled}
              onChange={(value) => onChange({ order: value })}
            />
          </div>
          <div className="flex flex-wrap items-center gap-4">
            <label className="flex items-center gap-2">
              <Checkbox
                checked={filter.recent_only}
                disabled={disabled}
                onCheckedChange={(value) =>
                  onChange({ recent: value === true ? "true" : undefined })
                }
              />
              最近30日
            </label>
            {scope.kind === "personal" && (
              <label className="flex items-center gap-2">
                <Checkbox
                  checked={filter.favorite_only}
                  disabled={disabled}
                  onCheckedChange={(value) =>
                    onChange({ favorite: value === true ? "true" : undefined })
                  }
                />
                只看收藏
              </label>
            )}
            <ToggleGroup
              type="single"
              value={filter.catalog_state}
              disabled={disabled}
              onValueChange={(value) => {
                if (value) onChange({ state: value });
              }}
              aria-label="正常库与回收站"
            >
              <ToggleGroupItem value="active">素材库</ToggleGroupItem>
              <ToggleGroupItem value="trashed">回收站</ToggleGroupItem>
            </ToggleGroup>
            <ToggleGroup
              type="single"
              value={view}
              disabled={disabled}
              onValueChange={(value) => {
                if (value) onChange({ view: value }, false);
              }}
              aria-label="素材视图"
            >
              <ToggleGroupItem value="grid">网格</ToggleGroupItem>
              <ToggleGroupItem value="list">列表</ToggleGroupItem>
            </ToggleGroup>
          </div>
        </CollapsibleContent>
      </Collapsible>
      <p className="text-xs text-muted-foreground">
        分类与目录数字是当前库状态的完整计数；筛选结果共 {page?.total ?? "…"}{" "}
        项。
      </p>
    </div>
  );
}
