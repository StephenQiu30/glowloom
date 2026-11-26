import { useEffect, useRef } from "react";
import { createPortal } from "react-dom";
import {
  CanvasToolbarButton,
  CanvasToolbarDivider,
  CanvasToolbarGroup,
} from "./canvas-toolbar-primitives.tsx";
import { Toolbar } from "@/components/ui/toolbar";
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
import {
  ChevronDown,
  Pencil,
  FolderOpen,
  FolderClosed,
  Frame,
  History,
  Search,
  UserRound,
  Link2,
  Copy,
} from "lucide-react";
import { CanvasNodeSearch } from "../search/canvas-node-search.tsx";
import { CanvasHistoryDialog } from "../history/canvas-history-dialog.tsx";
import { toast } from "@/components/feedback/notification";
import Link from "next/link";
import { BrandMark } from "@/components/brand/brand";
import { useIsMobile } from "@/hooks/use-mobile";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
  PopoverTitle,
} from "@/components/ui/popover";
import { CanvasNodeSummary } from "./canvas-node-summary.tsx";
import type { CanvasNodeData } from "../types/canvas.ts";
import { Button } from "@/components/ui/button";
import { isProjectID } from "@/components/projects/project";
import type { CanvasMoreControl } from "./canvas-toolbar.tsx";
import {
  CANVAS_ASSET_PANEL_ID,
  useCanvasSidePanelStore,
} from "../store/use-canvas-side-panel-store.ts";

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
  participantsControl,
  presenceConnected,
  renderAccount,
  renderMore,
  compact,
  assetNavigationSlot,
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
  participantsControl?: React.ReactNode;
  presenceConnected?: boolean;
  renderAccount?: (compact: boolean) => React.ReactNode;
  renderMore?: CanvasMoreControl;
  compact?: boolean;
  assetNavigationSlot?: HTMLElement | null;
  onCreatePersonalProject?: () => void;
  onProjectCanvases?: () => void;
  canSwitchCanvas?: boolean;
  canDeleteCanvas?: boolean;
  canRenameCanvas?: boolean;
  projectCanvas?: boolean;
  projectPath?: string;
}) {
  const isMobile = useIsMobile();
  const compactLayout = compact ?? isMobile;
  const assetPanelOpen = useCanvasSidePanelStore((state) => state.panelOpen);
  const toggleAssetPanel = useCanvasSidePanelStore(
    (state) => state.togglePanel,
  );
  const navigationDocked = assetPanelOpen && !!assetNavigationSlot;
  const focusAfterOpen = useRef(false);
  useEffect(() => {
    if (!navigationDocked || !focusAfterOpen.current) return;
    focusAfterOpen.current = false;
    assetNavigationSlot?.parentElement
      ?.querySelector<HTMLButtonElement>("[data-canvas-side-panel-close]")
      ?.focus();
  }, [assetNavigationSlot, navigationDocked]);
  const nodeSummary = nodes && (
    <CanvasNodeSummary
      nodes={nodes}
      canvasID={canvasID}
      projectID={projectID}
    />
  );
  const history = (
    <CanvasHistoryDialog
      onLocate={
        onFocusNode &&
        ((id) => {
          if (nodes?.some((n) => n.id === id)) onFocusNode(id);
          else toast.info("原节点已删除");
        })
      }
    >
      <CanvasToolbarButton aria-label="生成历史" title="生成历史">
        <History />
      </CanvasToolbarButton>
    </CanvasHistoryDialog>
  );
  const search = (
    <CanvasNodeSearch nodes={nodes ?? []} onLocate={(id) => onFocusNode?.(id)}>
      <CanvasToolbarButton
        aria-label="搜索画布"
        title="搜索画布"
        disabled={!onFocusNode}
      >
        <Search />
      </CanvasToolbarButton>
    </CanvasNodeSearch>
  );
  const sharePath =
    isProjectID(projectID) && isProjectID(canvasID)
      ? "/canvas?" +
        new URLSearchParams({ project_id: projectID, canvas_id: canvasID })
      : "";
  const account = renderAccount ? (
    renderAccount(compactLayout)
  ) : (
    <CanvasToolbarButton
      aria-label="账号设置"
      title="账号设置"
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
    </CanvasToolbarButton>
  );
  const navigation = (
    <CanvasToolbarGroup
      className="canvas-navigation-group"
      appearance={navigationDocked ? "plain" : "surface"}
      aria-label="画布导航"
    >
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <CanvasToolbarButton
            className="canvas-brand"
            aria-label="打开导航菜单"
          >
            <BrandMark monochrome />
          </CanvasToolbarButton>
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
          <CanvasToolbarButton
            iconOnly={false}
            className="canvas-project-trigger"
            aria-label="打开画布菜单"
          >
            <Frame aria-hidden="true" />
            <span className="truncate">{title}</span>
            <ChevronDown data-icon="inline-end" />
          </CanvasToolbarButton>
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
      {!navigationDocked && (
        <>
          <CanvasToolbarDivider />
          <Tooltip>
            <TooltipTrigger asChild>
              <CanvasToolbarButton
                aria-label="资产管理"
                iconOnly={false}
                aria-expanded={assetPanelOpen}
                aria-controls={
                  assetPanelOpen ? CANVAS_ASSET_PANEL_ID : undefined
                }
                data-canvas-side-panel-trigger
                onClick={(event) => {
                  focusAfterOpen.current =
                    !assetPanelOpen && event.detail === 0 && !isMobile;
                  toggleAssetPanel();
                }}
              >
                <FolderClosed aria-hidden="true" />
                <span className="canvas-assets-label">资产管理</span>
              </CanvasToolbarButton>
            </TooltipTrigger>
            <TooltipContent>资产管理</TooltipContent>
          </Tooltip>
        </>
      )}
    </CanvasToolbarGroup>
  );
  return (
    <TooltipProvider>
      <Toolbar
        data-canvas-no-zoom
        className="canvas-topbar flex-1 justify-between"
      >
        <div className="canvas-chrome-panel pointer-events-auto flex min-w-0 items-center">
          {navigationDocked
            ? createPortal(navigation, assetNavigationSlot)
            : navigation}
          <div className="canvas-status-panel flex items-center gap-1">
            {!compactLayout && nodeSummary}
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
        <div className="canvas-header-actions pointer-events-auto flex shrink-0 items-center">
          <CanvasToolbarGroup
            aria-label={compactLayout ? "搜索" : "搜索与生成历史"}
          >
            {search}
            {!compactLayout && history}
          </CanvasToolbarGroup>
          <CanvasToolbarGroup aria-label="协作与分享">
            {!compactLayout && participantsControl}
            <Popover>
              <PopoverTrigger asChild>
                <CanvasToolbarButton
                  iconOnly={compactLayout}
                  aria-label="分享"
                  title="分享"
                  disabled={!sharePath}
                >
                  <Link2 />
                  {!compactLayout && <span>分享</span>}
                </CanvasToolbarButton>
              </PopoverTrigger>
              <PopoverContent
                side="bottom"
                align="end"
                sideOffset={8}
                aria-label="分享画布"
                className="w-72 max-w-[calc(100vw-24px)] gap-3"
                data-canvas-no-zoom
              >
                <PopoverTitle>分享画布</PopoverTitle>
                <div className="grid gap-1">
                  <p className="truncate text-sm font-medium" title={title}>
                    {title}
                  </p>
                  <p className="text-sm text-muted-foreground">
                    拥有项目访问权限的成员可打开链接。
                  </p>
                </div>
                {presenceConnected === false && (
                  <p role="status" className="text-sm text-muted-foreground">
                    在线状态暂不可用
                  </p>
                )}
                {compactLayout && participantsControl}
                <Button
                  variant="outline"
                  className="justify-start"
                  onClick={async () => {
                    const url = new URL(sharePath, window.location.origin);
                    try {
                      await navigator.clipboard.writeText(url.href);
                      toast.success("画布链接已复制");
                    } catch {
                      toast.error("复制失败，请从浏览器地址栏复制链接");
                    }
                  }}
                >
                  <Copy />
                  复制链接
                </Button>
              </PopoverContent>
            </Popover>
          </CanvasToolbarGroup>
          {renderMore?.({
            shortcuts: compactLayout ? (
              <>
                {history}
                {nodeSummary}
              </>
            ) : undefined,
            appearanceInAccount: !!renderAccount,
          })}
          <CanvasToolbarGroup aria-label="账号与服务">
            {account}
          </CanvasToolbarGroup>
        </div>
      </Toolbar>
    </TooltipProvider>
  );
}
