"use client";

import type { ReactNode } from "react";
import Link from "next/link";

import { ChevronDown, CheckSquare, FolderPlus, Upload } from "lucide-react";
import { Separator } from "@/components/ui/separator";
import { Button } from "@/components/ui/button";
import { PageHeading } from "@/components/layout/page-heading";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import {
  DropdownMenu,
  DropdownMenuTrigger,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuCheckboxItem,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
} from "@/components/ui/dropdown-menu";

export const mediaKinds = [
  { id: "image", label: "图片" },
  { id: "video", label: "视频" },
  { id: "audio", label: "音频" },
  { id: "document", label: "文档" },
  { id: "model", label: "3D" },
];
export type LibraryFilter = {
  kind: string;
  search: string;
  favorite: boolean;
  trash: boolean;
  folder: string;
  order: string;
  recent: boolean;
};
const orders = [
  { value: "updated_desc", label: "最近修改" },
  { value: "updated_asc", label: "最早修改" },
  { value: "name_asc", label: "名称顺序" },
];

export function LibraryToolbar({
  filter,
  onFilter,
  disabled,
  locked,
  batch,
  batchActions,
  canCreateFolder,
  onBatch,
  onFolder,
  onUpload,
  onBack,
}: {
  filter: LibraryFilter;
  onFilter: (filter: LibraryFilter) => void;
  disabled: boolean;
  locked: boolean;
  batch: boolean;
  batchActions: ReactNode;
  canCreateFolder: boolean;
  onBatch: () => void;
  onFolder: () => void;
  onUpload: () => void;
  onBack?: () => void;
}) {
  const inactive = disabled || locked;
  return (
    <>
      <PageHeading
        title="参考素材"
        actions={
          batch ? (
            batchActions
          ) : (
            <>
              <Button
                variant="ghost"
                size="lg"
                className="library-desktop-action"
                disabled={inactive}
                onClick={onBatch}
                aria-pressed={batch}
              >
                <CheckSquare data-icon="inline-start" />
                {batch ? "取消选择" : "批量操作"}
              </Button>
              <Button
                variant="ghost"
                size="lg"
                className="library-folder-action"
                disabled={inactive || !canCreateFolder}
                onClick={onFolder}
              >
                <FolderPlus data-icon="inline-start" />
                新建文件夹
              </Button>
              <Button
                size="lg"
                aria-label="上传素材"
                disabled={inactive}
                onClick={onUpload}
              >
                <Upload data-icon="inline-start" />
                <span className="library-upload-label">上传素材</span>
                <span className="library-upload-short-label">上传</span>
              </Button>
            </>
          )
        }
      />
      <div className="library-controls">
        {onBack ? (
          <Button variant="ghost" disabled={locked} onClick={onBack}>
            返回资产
          </Button>
        ) : (
          <Button variant="ghost" asChild>
            <Link href="/assets">返回资产</Link>
          </Button>
        )}
        <div className="library-filter-row">
          <ToggleGroup
            type="single"
            value={filter.kind}
            onValueChange={(kind) => {
              if (kind) onFilter({ ...filter, kind });
            }}
            aria-label="素材类型"
          >
            {mediaKinds.map((kind) => (
              <ToggleGroupItem
                key={kind.id}
                value={kind.id}
                disabled={inactive}
              >
                {kind.label}
              </ToggleGroupItem>
            ))}
          </ToggleGroup>
          <Separator orientation="vertical" className="toolbar-separator" />
          <div className="toolbar-filter-group">
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <Button
                  variant={
                    filter.favorite || filter.trash || filter.recent
                      ? "secondary"
                      : "ghost"
                  }
                  size="lg"
                  disabled={inactive}
                  aria-label={
                    filter.favorite
                      ? "筛选素材：收藏"
                      : filter.trash
                        ? "筛选素材：回收站"
                        : "筛选素材"
                  }
                  data-active={
                    filter.favorite ||
                    filter.trash ||
                    filter.recent ||
                    undefined
                  }
                >
                  {filter.favorite ? "收藏" : filter.trash ? "回收站" : "筛选"}
                  <ChevronDown data-icon="inline-end" />
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent
                align="start"
                className="library-filter-menu w-56"
              >
                <DropdownMenuGroup>
                  <DropdownMenuLabel>显示范围</DropdownMenuLabel>
                  <DropdownMenuCheckboxItem
                    checked={filter.favorite}
                    onCheckedChange={(favorite) =>
                      onFilter({
                        ...filter,
                        favorite,
                        trash: favorite ? false : filter.trash,
                      })
                    }
                  >
                    仅显示收藏
                  </DropdownMenuCheckboxItem>
                  <DropdownMenuCheckboxItem
                    checked={filter.recent}
                    onCheckedChange={(recent) =>
                      onFilter({ ...filter, recent })
                    }
                  >
                    最近 30 天
                  </DropdownMenuCheckboxItem>
                  <DropdownMenuCheckboxItem
                    checked={filter.trash}
                    onCheckedChange={(trash) =>
                      onFilter({
                        ...filter,
                        trash,
                        favorite: false,
                        folder: "",
                      })
                    }
                  >
                    回收站
                  </DropdownMenuCheckboxItem>
                </DropdownMenuGroup>
                <DropdownMenuGroup>
                  <DropdownMenuLabel>排序</DropdownMenuLabel>
                  <DropdownMenuRadioGroup
                    value={filter.order}
                    onValueChange={(order) => onFilter({ ...filter, order })}
                  >
                    {orders.map((order) => (
                      <DropdownMenuRadioItem
                        key={order.value}
                        value={order.value}
                      >
                        {order.label}
                      </DropdownMenuRadioItem>
                    ))}
                  </DropdownMenuRadioGroup>
                </DropdownMenuGroup>
                <DropdownMenuGroup className="library-mobile-menu-actions">
                  <DropdownMenuLabel>管理素材</DropdownMenuLabel>
                  <DropdownMenuItem onSelect={onBatch}>
                    <CheckSquare />
                    {batch ? "取消选择" : "批量操作"}
                  </DropdownMenuItem>
                  {!batch && (
                    <DropdownMenuItem
                      disabled={!canCreateFolder}
                      onSelect={onFolder}
                    >
                      <FolderPlus />
                      新建文件夹
                    </DropdownMenuItem>
                  )}
                </DropdownMenuGroup>
              </DropdownMenuContent>
            </DropdownMenu>
            <Button
              variant="ghost"
              size="lg"
              className="library-desktop-filter"
              disabled={inactive}
              aria-pressed={filter.recent}
              onClick={() => onFilter({ ...filter, recent: !filter.recent })}
            >
              最近30天
            </Button>
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <Button
                  variant="ghost"
                  size="lg"
                  className="library-desktop-filter"
                  disabled={inactive}
                  aria-label="素材排序"
                >
                  {orders.find((o) => o.value === filter.order)?.label}
                  <ChevronDown data-icon="inline-end" />
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent>
                <DropdownMenuGroup>
                  <DropdownMenuRadioGroup
                    value={filter.order}
                    onValueChange={(order) => onFilter({ ...filter, order })}
                  >
                    {orders.map((order) => (
                      <DropdownMenuRadioItem
                        key={order.value}
                        value={order.value}
                      >
                        {order.label}
                      </DropdownMenuRadioItem>
                    ))}
                  </DropdownMenuRadioGroup>
                </DropdownMenuGroup>
              </DropdownMenuContent>
            </DropdownMenu>
          </div>
        </div>
      </div>
    </>
  );
}
