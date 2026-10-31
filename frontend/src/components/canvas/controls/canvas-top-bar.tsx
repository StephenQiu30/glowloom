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
  TooltipProvider,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { ChevronDown, Pencil, FolderOpen, History } from "lucide-react";
import { CanvasNodeSearch } from "../search/canvas-node-search";
import { CanvasHistoryDialog } from "../history/canvas-history-dialog";
import { toast } from "sonner";
import { Search, UserRound, Link2 } from "lucide-react";
import Link from "next/link";
import { BrandMark } from "@/components/canvas/runtime/components/layout/shell/brand-mark";
import { CanvasNodeSummary } from "./canvas-node-summary";
import type { CanvasNodeData } from "../types/canvas";

export function CanvasTopBar({
  title,
  nodes,
  canvasID,
  projectID,
  onRenameProject,
  onExportProject,
  exportingProject,
  onHome,
  onProjects,
  onCreateProject,
  onDeleteProject,
  saveControl,
  workflowControl,
  participantsControl,
  onFocusNode,
  onCreatePersonalProject,
  onProjectCanvases,
  canSwitchCanvas = true,
  canDeleteCanvas = true,
  canRenameCanvas = true,
  projectCanvas = false,
  projectPath,
}: {
  title: string;
  nodes?: CanvasNodeData[];
  canvasID?: string;
  projectID?: string;
  onFocusNode?: (nodeID: string) => void;
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
  participantsControl?: React.ReactNode;
  onCreatePersonalProject?: () => void;
  onProjectCanvases?: () => void;
  canSwitchCanvas?: boolean;
  canDeleteCanvas?: boolean;
  canRenameCanvas?: boolean;
  projectCanvas?: boolean;
  projectPath?: string;
}) {
  return (
    <TooltipProvider>
      <div
        data-canvas-no-zoom
        className="canvas-topbar flex min-w-0 flex-1 items-center justify-between gap-2"
      >
        <div className="canvas-chrome-panel pointer-events-auto flex min-w-0 items-center gap-1">
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button
                variant="ghost"
                size="default"
                className="canvas-brand"
                aria-label="打开导航菜单"
              >
                <BrandMark />
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
                size="default"
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
                <DropdownMenuItem
                  className="h-9"
                  disabled={!canRenameCanvas}
                  onSelect={onRenameProject}
                >
                  <Pencil />
                  重命名画布
                </DropdownMenuItem>
                <DropdownMenuItem
                  className="h-9"
                  disabled={exportingProject}
                  onSelect={onExportProject}
                >
                  导出画布
                </DropdownMenuItem>
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
          <div className="canvas-status-panel flex items-center gap-1">
            {nodes && (
              <CanvasNodeSummary
                nodes={nodes}
                canvasID={canvasID}
                projectID={projectID}
              />
            )}
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
        <div className="canvas-header-actions pointer-events-auto flex items-center gap-1">
          {participantsControl}
          <CanvasNodeSearch
            nodes={nodes ?? []}
            onLocate={(id) => onFocusNode?.(id)}
          >
            <Button
              variant="ghost"
              size="icon-lg"
              aria-label="搜索画布"
              disabled={!onFocusNode}
            >
              <Search />
            </Button>
          </CanvasNodeSearch>
          <CanvasHistoryDialog>
            <Button variant="ghost" size="icon-lg" aria-label="生成历史">
              <History />
            </Button>
          </CanvasHistoryDialog>
          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                variant="ghost"
                size="default"
                className="canvas-share-trigger"
                aria-label="复制画布链接"
                onClick={() => {
                  void navigator.clipboard
                    .writeText(window.location.href)
                    .then(() => toast.success("画布链接已复制"))
                    .catch(() =>
                      toast.error("复制失败，请从浏览器地址栏复制链接"),
                    );
                }}
              >
                <span className="hidden sm:inline">分享</span>
                <Link2 className="sm:hidden" />
              </Button>
            </TooltipTrigger>
            <TooltipContent>复制画布链接，保留现有访问权限</TooltipContent>
          </Tooltip>
          <Button
            variant="ghost"
            size="icon-lg"
            aria-label="账号设置"
            disabled={!canSwitchCanvas}
            asChild={canSwitchCanvas}
          >
            {canSwitchCanvas ? (
              <Link href="/account">
                <UserRound />
              </Link>
            ) : (
              <UserRound />
            )}
          </Button>
        </div>
      </div>
    </TooltipProvider>
  );
}
