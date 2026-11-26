import { PreviewImage } from "@/components/canvas/runtime/components/media/preview-image";
import { Button } from "@/components/ui/button";
import { ContentState } from "@/components/layout/content-state";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { useIsMobile } from "@/hooks/use-mobile";
import {
  ChevronRight,
  Download,
  Filter,
  Folder,
  Eye,
  FileText,
  Image as ImageIcon,
  ListChecks,
  ListX,
  PanelLeft,
  SquareCheck,
  UserRound,
  Music2,
  Search,
  Settings2,
  Square,
  Type,
  Video,
  X,
} from "lucide-react";
import { motion } from "motion/react";
import {
  useMemo,
  useState,
  useSyncExternalStore,
  type PointerEvent as ReactPointerEvent,
  type ReactNode,
  type Ref,
} from "react";

import { getNodeDefinition } from "../lib/node-registry.ts";
import {
  CanvasAssetLayoutButton,
  CanvasAssetSearch,
  type CanvasAssetLayout,
} from "../controls/canvas-asset-toolbar.tsx";
import {
  CanvasToolbarButton,
  CanvasToolbarGroup,
} from "../controls/canvas-toolbar-primitives.tsx";
import {
  CanvasNodeThumbnail,
  CanvasNodeThumbnailProvider,
} from "../search/canvas-node-search.tsx";
import {
  canvasNodeName,
  canvasNodeTypeLabel,
  canvasSearchType,
  canvasSearchTypes,
  type CanvasSearchType,
} from "../search/canvas-node-search-state.ts";
import { formatDuration } from "@/components/canvas/runtime/lib/media/image-utils";
import {
  getImagePreviewRevision,
  previewUrlFor,
  subscribeImagePreviews,
} from "@/components/canvas/runtime/lib/media/image-storage";
import {
  CANVAS_ASSET_PANEL_ID,
  CANVAS_SIDE_PANEL_MAX_WIDTH,
  CANVAS_SIDE_PANEL_MIN_WIDTH,
  CANVAS_SIDE_PANEL_MOTION_MS,
  useCanvasSidePanelStore,
} from "../store/use-canvas-side-panel-store.ts";
import { CanvasNodeType, type CanvasNodeData } from "../types/canvas.ts";

import { ZipDownloadDialog } from "@/components/canvas/runtime/components/assets/transfer/zip-download-dialog";
import { canvasZipSelection } from "../lib/canvas-zip-selection.ts";
import { notifyError } from "@/components/canvas/runtime/components/feedback/error-notification";

const PANEL_MOTION_SECONDS = CANVAS_SIDE_PANEL_MOTION_MS / 1000;
const PANEL_EASE = [0.22, 1, 0.36, 1] as const;

type PanelTab = "canvas" | "assets";

type Props = {
  referenceAssets?: ReactNode;
  navigationRef: Ref<HTMLDivElement>;
  nodes: CanvasNodeData[];
  selectedNodeIds: Set<string>;
  onFocusNode: (nodeId: string) => void;
  onPreviewNode: (nodeId: string) => void;
};

const NODE_TYPE_ICON: Record<string, typeof Square> = {
  [CanvasNodeType.Image]: ImageIcon,
  [CanvasNodeType.Video]: Video,
  [CanvasNodeType.Audio]: Music2,
  [CanvasNodeType.Text]: Type,
  [CanvasNodeType.Config]: Settings2,
  [CanvasNodeType.Group]: Folder,
  subject: UserRound,
};

const STATUS_LABEL: Record<string, string> = {
  success: "已完成",
  loading: "生成中",
  error: "未完成",
};

function restoreAssetTriggerFocus() {
  requestAnimationFrame(() => {
    document
      .querySelector<HTMLButtonElement>(
        '[data-canvas-side-panel-trigger][aria-expanded="false"]',
      )
      ?.focus();
  });
}

export function CanvasSidePanel({
  referenceAssets,
  navigationRef,
  nodes,
  selectedNodeIds,
  onFocusNode,
  onPreviewNode,
}: Props) {
  const tab = useCanvasSidePanelStore((state) => state.tab);
  const setTab = useCanvasSidePanelStore((state) => state.setTab);
  const [assetsVisited, setAssetsVisited] = useState(false);
  const isMobile = useIsMobile();
  const closePanel = useCanvasSidePanelStore((state) => state.closePanel);
  const width = useCanvasSidePanelStore((state) => state.width);
  const panelOpen = useCanvasSidePanelStore((state) => state.panelOpen);
  const panelMounted = useCanvasSidePanelStore((state) => state.panelMounted);
  const panelClosing = useCanvasSidePanelStore((state) => state.panelClosing);
  const setWidth = useCanvasSidePanelStore((state) => state.setWidth);
  const [resizing, setResizing] = useState(false);

  const startResize = (event: ReactPointerEvent<HTMLButtonElement>) => {
    event.preventDefault();
    const startX = event.clientX;
    const startWidth = width;
    const onMove = (moveEvent: PointerEvent) => {
      setWidth(startWidth + moveEvent.clientX - startX);
    };
    const onUp = () => {
      window.removeEventListener("pointermove", onMove);
      window.removeEventListener("pointerup", onUp);
      window.removeEventListener("pointercancel", onUp);
      setResizing(false);
    };
    setResizing(true);
    window.addEventListener("pointermove", onMove);
    window.addEventListener("pointerup", onUp);
    window.addEventListener("pointercancel", onUp);
  };

  const panelContent = (
    <Tabs
      value={tab}
      onValueChange={(value) => {
        setTab(value as PanelTab);
        if (value === "assets" || tab === "assets") setAssetsVisited(true);
      }}
      className="canvas-asset-panel flex min-h-0 flex-1 flex-col"
    >
      <div className="canvas-panel-header">
        <div ref={navigationRef} className="min-w-0 flex-1" />
        <CanvasToolbarButton
          aria-label="收起资产管理"
          title="收起资产管理"
          className="text-muted-foreground"
          data-canvas-side-panel-close
          onClick={() => {
            closePanel();
            if (!isMobile) restoreAssetTriggerFocus();
          }}
        >
          {isMobile ? (
            <X aria-hidden="true" />
          ) : (
            <PanelLeft aria-hidden="true" />
          )}
        </CanvasToolbarButton>
      </div>
      <TabsList
        variant="pill"
        className="canvas-asset-tabs"
        aria-label="资产来源"
      >
        <TabsTrigger value="canvas">画布资产</TabsTrigger>
        <TabsTrigger value="assets">参考素材</TabsTrigger>
      </TabsList>
      <TabsContent
        forceMount
        value="canvas"
        className="min-h-0 flex-1 overflow-hidden data-[state=inactive]:hidden"
      >
        <CanvasNodesTab
          nodes={nodes}
          selectedNodeIds={selectedNodeIds}
          onFocusNode={(id) => {
            onFocusNode(id);
            if (isMobile) closePanel();
          }}
          onPreviewNode={onPreviewNode}
        />
      </TabsContent>
      <TabsContent
        forceMount
        value="assets"
        className="min-h-0 flex-1 overflow-hidden data-[state=inactive]:hidden"
      >
        {(assetsVisited || tab === "assets") &&
          (referenceAssets ?? <ContentState title="参考素材暂不可用" />)}
      </TabsContent>
    </Tabs>
  );
  if (isMobile)
    return (
      <Sheet
        open={panelOpen}
        onOpenChange={(open) => {
          if (!open) closePanel();
        }}
      >
        <SheetContent
          id={CANVAS_ASSET_PANEL_ID}
          aria-describedby={undefined}
          side="left"
          className="gap-0 p-0"
          showCloseButton={false}
          onCloseAutoFocus={(event) => {
            event.preventDefault();
            restoreAssetTriggerFocus();
          }}
        >
          <SheetHeader className="sr-only">
            <SheetTitle>资产管理</SheetTitle>
          </SheetHeader>
          {panelContent}
        </SheetContent>
      </Sheet>
    );
  if (!panelMounted) return null;

  return (
    <motion.div
      className="relative z-40 flex h-full shrink-0"
      initial={{ width: 0, opacity: 0 }}
      animate={{ width: panelOpen ? width + 1 : 0, opacity: panelOpen ? 1 : 0 }}
      transition={{
        duration: resizing ? 0 : PANEL_MOTION_SECONDS,
        ease: PANEL_EASE,
      }}
      style={{
        overflow: "clip",
        pointerEvents: panelClosing ? "none" : undefined,
      }}
    >
      <motion.aside
        id={CANVAS_ASSET_PANEL_ID}
        aria-label="资产管理"
        inert={panelClosing}
        className="relative flex h-full shrink-0 flex-col overflow-hidden border-r bg-background text-foreground"
        initial={{ x: -48 }}
        animate={{ x: panelClosing ? -28 : 0 }}
        transition={{
          duration: resizing ? 0 : PANEL_MOTION_SECONDS,
          ease: PANEL_EASE,
        }}
        style={{ width }}
        data-canvas-no-zoom
      >
        {panelContent}
        <Button
          variant="ghost"
          type="button"
          role="separator"
          aria-orientation="vertical"
          aria-controls={CANVAS_ASSET_PANEL_ID}
          aria-valuemin={CANVAS_SIDE_PANEL_MIN_WIDTH}
          aria-valuemax={CANVAS_SIDE_PANEL_MAX_WIDTH}
          aria-valuenow={width}
          className="absolute inset-y-0 right-0 z-40 h-auto w-4 translate-x-1/2 cursor-col-resize touch-none p-0"
          onPointerDown={startResize}
          onKeyDown={(event) => {
            if (event.key !== "ArrowLeft" && event.key !== "ArrowRight") return;
            event.preventDefault();
            setWidth(width + (event.key === "ArrowLeft" ? -16 : 16));
          }}
          aria-label={"调整左侧面板宽度"}
        />
      </motion.aside>
    </motion.div>
  );
}

const nodeAssetTypes = [
  ...canvasSearchTypes,
  { type: CanvasNodeType.Config, label: "配置" },
] as const;
type NodeAssetType = CanvasSearchType | CanvasNodeType.Config;

function nodeAssetType(node: CanvasNodeData): NodeAssetType {
  return node.type === CanvasNodeType.Config
    ? CanvasNodeType.Config
    : canvasSearchType(node.type, node);
}

function CanvasNodesTab({
  nodes,
  selectedNodeIds,
  onFocusNode,
  onPreviewNode,
}: Omit<Props, "referenceAssets" | "navigationRef">) {
  useSyncExternalStore(
    subscribeImagePreviews,
    getImagePreviewRevision,
    () => 0,
  );
  const [keyword, setKeyword] = useState("");
  const [typeFilter, setTypeFilter] = useState<NodeAssetType>("all");
  const [layout, setLayout] = useState<CanvasAssetLayout>("grid");
  const [selectMode, setSelectMode] = useState(false);
  const [checked, setChecked] = useState<Set<string>>(new Set());
  const [collapsedGroups, setCollapsedGroups] = useState<Set<string>>(
    new Set(),
  );
  const [download, setDownload] =
    useState<Omit<CanvasContract.AssetZipExportRequest, "commandId">>();
  const filtered = useMemo(() => {
    const query = keyword.trim().toLowerCase();
    return nodes.filter(
      (node) =>
        (typeFilter === "all" || nodeAssetType(node) === typeFilter) &&
        (!query ||
          [
            node.title,
            node.metadata?.prompt,
            node.type === "text" && node.metadata?.content,
          ]
            .filter(Boolean)
            .join(" ")
            .toLowerCase()
            .includes(query)),
    );
  }, [nodes, keyword, typeFilter]);

  const rows = useMemo(() => {
    if (layout === "grid")
      return filtered.map((node) => ({
        node,
        grouped: false,
        expandable: false,
      }));
    const filteredIds = new Set(filtered.map((node) => node.id));
    const groups = new Set(
      nodes
        .filter((node) => node.type === CanvasNodeType.Group)
        .map((node) => node.id),
    );
    const children = new Map<string, CanvasNodeData[]>();
    for (const node of filtered) {
      const groupId = node.metadata?.groupId;
      if (groupId && groups.has(groupId)) {
        const members = children.get(groupId) ?? [];
        members.push(node);
        children.set(groupId, members);
      }
    }
    return nodes.flatMap((node) => {
      if (node.metadata?.groupId && groups.has(node.metadata.groupId))
        return [];
      const members = children.get(node.id) ?? [];
      if (!filteredIds.has(node.id) && !members.length) return [];
      return [
        { node, grouped: false, expandable: members.length > 0 },
        ...(collapsedGroups.has(node.id)
          ? []
          : members.map((member) => ({
              node: member,
              grouped: true,
              expandable: false,
            }))),
      ];
    });
  }, [collapsedGroups, filtered, layout, nodes]);

  const checkedNodes = nodes.filter((node) => checked.has(node.id));
  const allChecked =
    rows.length > 0 && rows.every(({ node }) => checked.has(node.id));
  function handleExport() {
    if (!checkedNodes.length) return;
    try {
      setDownload(canvasZipSelection(checkedNodes, nodes));
    } catch (error) {
      notifyError(error, "素材ZIP尚不可打包");
    }
  }

  return (
    <div className="flex h-full flex-col">
      <div className="canvas-asset-tools">
        <CanvasAssetSearch
          label="搜索节点"
          value={keyword}
          onChange={setKeyword}
          onClear={() => setKeyword("")}
        />
        <CanvasToolbarGroup appearance="plain" aria-label="画布资产工具">
          <CanvasToolbarButton
            aria-label="多选节点"
            title={selectMode ? "退出多选" : "多选节点"}
            aria-pressed={selectMode}
            onClick={() => {
              setSelectMode(!selectMode);
              setChecked(new Set());
            }}
          >
            <ListChecks />
          </CanvasToolbarButton>
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <CanvasToolbarButton
                variant={typeFilter === "all" ? "ghost" : "secondary"}
                aria-label="节点类型筛选"
                title="节点类型筛选"
              >
                <Filter />
              </CanvasToolbarButton>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end" data-canvas-no-zoom>
              <DropdownMenuRadioGroup
                value={typeFilter}
                onValueChange={(value) => {
                  const type = nodeAssetTypes.find(
                    (item) => item.type === value,
                  )?.type;
                  if (type) setTypeFilter(type);
                }}
              >
                {nodeAssetTypes.map(({ type, label }) => (
                  <DropdownMenuRadioItem key={type} value={type}>
                    {label}
                  </DropdownMenuRadioItem>
                ))}
              </DropdownMenuRadioGroup>
            </DropdownMenuContent>
          </DropdownMenu>
          <CanvasAssetLayoutButton layout={layout} onChange={setLayout} />
        </CanvasToolbarGroup>
      </div>
      <div className="canvas-asset-results">
        {rows.length ? (
          <CanvasNodeThumbnailProvider>
            <ul
              aria-label="画布资产"
              className="canvas-asset-items"
              data-layout={layout}
            >
              {rows.map(({ node, grouped, expandable }) => {
                const name = canvasNodeName(node);
                const type = nodeAssetType(node);
                const Icon =
                  NODE_TYPE_ICON[type] ??
                  (type === "other" ? FileText : Square);
                const selected = selectMode
                  ? checked.has(node.id)
                  : selectedNodeIds.has(node.id);
                const collapsed = collapsedGroups.has(node.id);
                const status = node.metadata?.status;
                const canPreview =
                  (type === "image" || type === "video") &&
                  !!node.metadata?.content;
                const typeLabel =
                  getNodeDefinition(node.type)?.title ||
                  canvasNodeTypeLabel(node.type, node);
                const duration = node.metadata?.durationMs;
                const fallback = (
                  <Icon className="text-muted-foreground" size={20} />
                );
                return (
                  <li
                    className="canvas-asset-item"
                    key={node.id}
                    data-selected={selected || undefined}
                    data-grouped={grouped || undefined}
                  >
                    {expandable && (
                      <CanvasToolbarButton
                        aria-expanded={!collapsed}
                        aria-label={`${collapsed ? "展开" : "收起"}组：${name}`}
                        onClick={() =>
                          setCollapsedGroups((previous) => {
                            const next = new Set(previous);
                            if (!next.delete(node.id)) next.add(node.id);
                            return next;
                          })
                        }
                      >
                        <ChevronRight
                          className={collapsed ? undefined : "rotate-90"}
                        />
                      </CanvasToolbarButton>
                    )}
                    <Button
                      type="button"
                      variant="ghost"
                      className="canvas-asset-item-trigger h-auto justify-start p-0"
                      aria-label={`${selectMode ? "选择" : "定位"}节点：${name}，${typeLabel}`}
                      aria-pressed={selectMode ? selected : undefined}
                      aria-current={
                        !selectMode && selected ? "true" : undefined
                      }
                      title={name}
                      onClick={() => {
                        if (!selectMode) {
                          onFocusNode(node.id);
                          return;
                        }
                        setChecked((previous) => {
                          const next = new Set(previous);
                          if (!next.delete(node.id)) next.add(node.id);
                          return next;
                        });
                      }}
                    >
                      <span className="canvas-asset-cover" aria-hidden="true">
                        <CanvasNodeThumbnail
                          node={node}
                          fallback={fallback}
                          mediaFallback={
                            <CanvasAssetMediaFallback
                              node={node}
                              type={type}
                              fallback={fallback}
                            />
                          }
                        />
                        {duration && duration > 0 && type === "video" ? (
                          <span className="canvas-asset-duration">
                            {formatDuration(duration)}
                          </span>
                        ) : null}
                        {selectMode && selected && (
                          <SquareCheck className="canvas-asset-check" />
                        )}
                      </span>
                      <span className="canvas-asset-item-label">
                        <span className="block truncate">{name}</span>
                        <span className="canvas-asset-item-detail">
                          {[typeLabel, status && STATUS_LABEL[status]]
                            .filter(Boolean)
                            .join(" · ")}
                        </span>
                      </span>
                    </Button>
                    {!selectMode && canPreview && (
                      <CanvasToolbarButton
                        className="canvas-asset-preview"
                        aria-label={`预览：${name}`}
                        title="预览"
                        onClick={() => onPreviewNode(node.id)}
                      >
                        <Eye />
                      </CanvasToolbarButton>
                    )}
                  </li>
                );
              })}
            </ul>
          </CanvasNodeThumbnailProvider>
        ) : (
          <ContentState
            className="min-h-24 p-3"
            icon={Search}
            title={nodes.length ? "没有匹配的节点" : "画布暂无资产"}
          />
        )}
      </div>
      {selectMode && (
        <CanvasToolbarGroup
          appearance="plain"
          className="canvas-asset-selection"
          aria-label="批量操作"
        >
          <CanvasToolbarButton
            aria-label={allChecked ? "取消全选" : "全选当前结果"}
            title={allChecked ? "取消全选" : "全选当前结果"}
            disabled={!rows.length}
            onClick={() =>
              setChecked((previous) => {
                const next = new Set(previous);
                for (const { node } of rows) {
                  if (allChecked) next.delete(node.id);
                  else next.add(node.id);
                }
                return next;
              })
            }
          >
            {allChecked ? <ListX /> : <ListChecks />}
          </CanvasToolbarButton>
          <span className="flex-1 text-xs text-muted-foreground" role="status">
            已选 {checkedNodes.length} 项
          </span>
          <CanvasToolbarButton
            aria-label="打包下载"
            title="打包下载"
            onClick={handleExport}
            disabled={!checkedNodes.length}
          >
            <Download />
          </CanvasToolbarButton>
        </CanvasToolbarGroup>
      )}
      {download && (
        <ZipDownloadDialog
          request={download}
          onClose={() => setDownload(undefined)}
        />
      )}
    </div>
  );
}

function CanvasAssetMediaFallback({
  node,
  type,
  fallback,
}: {
  node: CanvasNodeData;
  type: NodeAssetType;
  fallback: React.ReactNode;
}) {
  const [failed, setFailed] = useState<string>();
  const metadata = node.metadata;
  const primary =
    metadata?.images?.find((image) => image.id === metadata.primaryImageId) ??
    metadata?.images?.[0];
  const source =
    type === "image"
      ? previewUrlFor(primary?.storageKey || metadata?.storageKey) ||
        primary?.content ||
        metadata?.content
      : metadata?.content;
  if (!source || failed === source) return fallback;
  if (type === "video") {
    return (
      <video
        src={source}
        preload="metadata"
        muted
        playsInline
        aria-hidden="true"
        className="size-full object-cover"
        onError={() => setFailed(source)}
      />
    );
  }
  if (type === "image") {
    return (
      <PreviewImage
        src={source}
        loading="lazy"
        className="size-full object-cover"
        onError={() => setFailed(source)}
      />
    );
  }
  return fallback;
}
