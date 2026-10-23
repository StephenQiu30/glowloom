"use client";

import {
  ChevronDown,
  CheckSquare,
  FolderPlus,
  Heart,
  Trash2,
  Upload,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { PageHeading } from "@/components/layout/page-heading";
import { TabsList, TabsTrigger } from "@/components/ui/tabs";
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
  section,
  disabled,
  locked,
  batch,
  canCreateFolder,
  onBatch,
  onFolder,
  onUpload,
}: {
  filter: LibraryFilter;
  onFilter: (filter: LibraryFilter) => void;
  section: string;
  disabled: boolean;
  locked: boolean;
  batch: boolean;
  canCreateFolder: boolean;
  onBatch: () => void;
  onFolder: () => void;
  onUpload: () => void;
}) {
  const inactive = disabled || locked || section !== "library";
  return (
    <>
      <PageHeading
        title="素材库"
        actions={
          section === "library" ? (
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
          ) : null
        }
      />
      <div className="library-filter-row">
        <TabsList variant="navigation" aria-label="素材来源">
          <TabsTrigger value="library" disabled={locked}>
            素材
          </TabsTrigger>
          <TabsTrigger value="history" disabled={locked}>
            生成历史
          </TabsTrigger>
        </TabsList>
        {section === "library" && (
          <>
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
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <Button
                  variant="ghost"
                  size="lg"
                  disabled={inactive}
                  aria-label="筛选素材"
                >
                  筛选
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
                      onFilter({ ...filter, favorite })
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
                  <DropdownMenuItem
                    disabled={!canCreateFolder}
                    onSelect={onFolder}
                  >
                    <FolderPlus />
                    新建文件夹
                  </DropdownMenuItem>
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
            <div className="library-secondary-actions">
              <Button
                variant="ghost"
                size="lg"
                disabled={inactive}
                aria-pressed={filter.favorite}
                onClick={() =>
                  onFilter({ ...filter, favorite: !filter.favorite })
                }
              >
                <Heart data-icon="inline-start" />
                收藏
              </Button>
              <Button
                variant="ghost"
                size="lg"
                disabled={inactive}
                aria-pressed={filter.trash}
                onClick={() =>
                  onFilter({
                    ...filter,
                    trash: !filter.trash,
                    favorite: false,
                    folder: "",
                  })
                }
              >
                <Trash2 data-icon="inline-start" />
                {filter.trash ? "返回素材" : "回收站"}
              </Button>
            </div>
          </>
        )}
      </div>
    </>
  );
}
