import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { ChevronDown, Pencil, FolderOpen, History } from "lucide-react";
import { productBrand } from "@/components/canvas/runtime/lib/brand";
import { BrandMark } from "@/components/canvas/runtime/components/layout/shell/brand-mark";

export function CanvasTopBar({
  title,
  onRenameProject,
  onHome,
  onProjects,
  onCreateProject,
  onDeleteProject,
  saveControl,
  workflowControl,
  onVersionHistory,
  onCreatePersonalProject,
  onProjectCanvases,
  canSwitchCanvas = true,
  canDeleteCanvas = true,
  projectCanvas = false,
  projectPath,
}: {
  title: string;
  onRenameProject: () => void;
  canUndo: boolean;
  canRedo: boolean;
  onHome: () => void;
  onProjects: () => void;
  onCreateProject: () => void;
  onDeleteProject: () => void;
  onExportProject: () => void;
  exportingProject?: boolean;
  onImportImage: () => void;
  onOpenPlugins?: () => void;
  onUndo: () => void;
  onRedo: () => void;
  saveControl?: React.ReactNode;
  workflowControl?: React.ReactNode;
  onVersionHistory?: () => void;
  onCreatePersonalProject?: () => void;
  onProjectCanvases?: () => void;
  canSwitchCanvas?: boolean;
  canDeleteCanvas?: boolean;
  projectCanvas?: boolean;
  projectPath?: string;
}) {
  return (
    <div
      data-canvas-no-zoom
      className="canvas-topbar pointer-events-none absolute inset-x-2 top-2 z-50 flex items-start justify-between gap-3"
    >
      <div className="canvas-chrome-panel pointer-events-auto flex min-w-0 items-center gap-1">
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button
              variant="ghost"
              size="sm"
              className="canvas-brand"
              aria-label="打开导航菜单"
            >
              <BrandMark />
              <span className="canvas-brand-word">{productBrand.name}</span>
              <ChevronDown data-icon="inline-end" />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent
            align="start"
            sideOffset={8}
            className="w-canvas-menu"
            data-canvas-no-zoom
          >
            <DropdownMenuGroup>
              <DropdownMenuItem className="h-10 px-3" onSelect={onHome}>
                回到主页
              </DropdownMenuItem>
              <DropdownMenuItem className="h-10 px-3" onSelect={onProjects}>
                全部项目
              </DropdownMenuItem>
            </DropdownMenuGroup>
            {(!projectCanvas || canDeleteCanvas) && (
              <>
                <DropdownMenuSeparator />
                <DropdownMenuGroup>
                  {!projectCanvas && (
                    <DropdownMenuItem
                      className="h-10 px-3"
                      onSelect={onCreateProject}
                    >
                      新建画布
                    </DropdownMenuItem>
                  )}
                  {canDeleteCanvas && (
                    <DropdownMenuItem
                      className="h-10 px-3"
                      variant="destructive"
                      onSelect={onDeleteProject}
                    >
                      删除当前画布
                    </DropdownMenuItem>
                  )}
                </DropdownMenuGroup>
              </>
            )}
          </DropdownMenuContent>
        </DropdownMenu>
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button
              variant="ghost"
              size="sm"
              className="canvas-project-trigger"
              aria-label="打开画布菜单"
            >
              <span className="truncate">{title}</span>
              <ChevronDown data-icon="inline-end" />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent
            align="start"
            sideOffset={8}
            className="w-canvas-menu"
            data-canvas-no-zoom
          >
            {projectPath && (
              <DropdownMenuLabel className="text-xs font-normal break-words whitespace-normal">
                所属项目：{projectPath}
              </DropdownMenuLabel>
            )}
            <DropdownMenuGroup>
              {onProjectCanvases && (
                <DropdownMenuItem
                  className="h-9"
                  disabled={!canSwitchCanvas}
                  onSelect={onProjectCanvases}
                >
                  <FolderOpen />
                  返回所属项目
                </DropdownMenuItem>
              )}
              <DropdownMenuItem className="h-9" onSelect={onRenameProject}>
                <Pencil />
                重命名画布
              </DropdownMenuItem>
              {onVersionHistory && (
                <DropdownMenuItem className="h-9" onSelect={onVersionHistory}>
                  <History />
                  版本历史
                </DropdownMenuItem>
              )}
              {onCreatePersonalProject && (
                <DropdownMenuItem
                  className="h-9"
                  onSelect={onCreatePersonalProject}
                >
                  <FolderOpen />
                  建立个人项目
                </DropdownMenuItem>
              )}
            </DropdownMenuGroup>
          </DropdownMenuContent>
        </DropdownMenu>
      </div>
      <div className="canvas-chrome-panel canvas-status-panel pointer-events-auto flex items-center gap-1">
        {workflowControl}
        {saveControl === undefined ? (
          <Tooltip>
            <TooltipTrigger asChild>
              <span
                role="status"
                tabIndex={0}
                className="canvas-save-status shrink-0 px-1.5 text-xs text-muted-foreground"
              >
                节点未保存
              </span>
            </TooltipTrigger>
            <TooltipContent side="bottom">
              节点编辑仅在当前页面预览，离开会丢失。
            </TooltipContent>
          </Tooltip>
        ) : (
          saveControl
        )}
      </div>
    </div>
  );
}
