"use client";
import { Folder, MoreHorizontal } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Card, CardHeader, CardTitle, CardFooter } from "@/components/ui/card";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import type { LibraryFolder } from "./library-model";
const styleClass: Record<string, string> = {
  glass: "rounded-2xl backdrop-blur-sm",
  stacked: "rounded-xl border-b-4",
  midnight: "rounded-xl border-2",
  paper: "rounded-none border-dashed",
  cinema: "rounded-lg border-x-4",
  compact: "rounded-md",
};
const themeClass: Record<string, string> = {
  aurora: "bg-secondary",
  obsidian: "bg-muted",
  ember: "bg-accent",
  pearl: "bg-background",
};
export function LibraryFolderCard({
  folder,
  count,
  childrenCount,
  disabled,
  readOnly,
  onOpen,
  onEdit,
  onDelete,
}: {
  folder: LibraryFolder;
  count: number;
  childrenCount: number;
  disabled: boolean;
  readOnly: boolean;
  onOpen: () => void;
  onEdit: () => void;
  onDelete: () => void;
}) {
  return (
    <div className={themeClass[folder.theme] ?? "bg-background"}>
      <Card
        variant="folder"
        className={styleClass[folder.style] ?? ""}
        data-folder-style={folder.style}
        data-folder-theme={folder.theme}
      >
        <CardHeader className="min-w-0 flex-1 p-0">
          <CardTitle>
            <Button
              variant="ghost"
              disabled={disabled}
              onClick={onOpen}
              className="max-w-full justify-start"
            >
              <Folder aria-hidden data-icon="inline-start" />
              <span className="truncate">{folder.name}</span>
            </Button>
          </CardTitle>
          <p className="px-2 text-xs text-muted-foreground">
            {count} 项素材 · {childrenCount} 个子目录
          </p>
        </CardHeader>
        <CardFooter className="p-0">
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button
                variant="ghost"
                size="icon-sm"
                disabled={disabled || readOnly}
                aria-label={`${folder.name} 目录操作`}
              >
                <MoreHorizontal aria-hidden data-icon="inline-start" />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent>
              <DropdownMenuGroup>
                <DropdownMenuItem onSelect={onEdit}>
                  重命名、移动与外观
                </DropdownMenuItem>
                <DropdownMenuItem variant="destructive" onSelect={onDelete}>
                  删除目录
                </DropdownMenuItem>
              </DropdownMenuGroup>
            </DropdownMenuContent>
          </DropdownMenu>
        </CardFooter>
      </Card>
    </div>
  );
}
