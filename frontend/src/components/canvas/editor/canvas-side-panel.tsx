import { PreviewImage } from "@/components/canvas/runtime/components/media/preview-image";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Badge } from "@/components/ui/badge";
import { Empty, EmptyHeader, EmptyTitle } from "@/components/ui/empty";
import {
  InputGroup,
  InputGroupAddon,
  InputGroupButton,
  InputGroupInput,
} from "@/components/ui/input-group";
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { useIsMobile } from "@/hooks/use-mobile";
import {
  ChevronRight,
  Download,
  Eye,
  FileText,
  Image as ImageIcon,
  ListChecks,
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
} from "react";

import {
  canvasThemes,
  type CanvasTheme,
} from "@/components/canvas/runtime/lib/theme";
import { getNodeDefinition } from "@/components/canvas/lib/node-registry";
import { cn } from "@/lib/utils";
import {
  getImagePreviewRevision,
  previewUrlFor,
  subscribeImagePreviews,
} from "@/components/canvas/runtime/lib/media/image-storage";
import {
  CANVAS_SIDE_PANEL_MAX_WIDTH,
  CANVAS_SIDE_PANEL_MIN_WIDTH,
  CANVAS_SIDE_PANEL_MOTION_MS,
  useCanvasSidePanelStore,
} from "@/components/canvas/store/use-canvas-side-panel-store";
import { useThemeStore } from "@/components/canvas/runtime/store/use-theme-store";
import {
  CanvasNodeType,
  type CanvasNodeData,
} from "@/components/canvas/types/canvas";

import { LibraryPickerContent } from "@/components/canvas/runtime/components/assets/picker/library-picker-content";
import type { InsertAssetPayload } from "@/components/canvas/runtime/components/assets/picker/asset-picker-modal";
import { ZipDownloadDialog } from "@/components/canvas/runtime/components/assets/transfer/zip-download-dialog";
import { canvasZipSelection } from "../lib/canvas-zip-selection";
import { notifyError } from "@/components/canvas/runtime/components/feedback/error-notification";

const PANEL_MOTION_SECONDS = CANVAS_SIDE_PANEL_MOTION_MS / 1000;
const PANEL_EASE = [0.22, 1, 0.36, 1] as const;

type PanelTab = "canvas" | "assets";

type Props = {
  assetsRevision?: number;
  savedAssetScope?: string;
  projectId?: string;
  nodes: CanvasNodeData[];
  selectedNodeIds: Set<string>;
  onFocusNode: (nodeId: string) => void;
  onPreviewNode: (nodeId: string) => void;
  onInsertAsset: (payload: InsertAssetPayload) => void;
};

const NODE_TYPE_ICON: Record<string, typeof Square> = {
  [CanvasNodeType.Image]: ImageIcon,
  [CanvasNodeType.Video]: Video,
  [CanvasNodeType.Audio]: Music2,
  [CanvasNodeType.Text]: Type,
  [CanvasNodeType.Config]: Settings2,
  [CanvasNodeType.Group]: Square,
};

const STATUS_LABEL: Record<string, string> = {
  success: "已完成",
  loading: "生成中",
  error: "未完成",
};

export function CanvasSidePanel({
  assetsRevision,
  savedAssetScope,
  projectId,
  nodes,
  selectedNodeIds,
  onFocusNode,
  onPreviewNode,
  onInsertAsset,
}: Props) {
  const theme = canvasThemes[useThemeStore((state) => state.theme)];
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
    let nextWidth = startWidth;
    const onMove = (moveEvent: PointerEvent) => {
      nextWidth = Math.min(
        CANVAS_SIDE_PANEL_MAX_WIDTH,
        Math.max(
          CANVAS_SIDE_PANEL_MIN_WIDTH,
          startWidth + moveEvent.clientX - startX,
        ),
      );
      setWidth(nextWidth);
    };
    const onUp = () => {
      window.removeEventListener("pointermove", onMove);
      window.removeEventListener("pointerup", onUp);
      setResizing(false);
    };
    setResizing(true);
    window.addEventListener("pointermove", onMove);
    window.addEventListener("pointerup", onUp);
  };

  const panelContent = (
    <Tabs
      value={tab}
      onValueChange={(value) => {
        setTab(value as PanelTab);
        if (value === "assets" || tab === "assets") setAssetsVisited(true);
      }}
      className="flex min-h-0 flex-1 flex-col gap-2"
    >
      <div className="canvas-panel-header flex min-h-17 shrink-0 items-center gap-2 px-3 py-3">
        <TabsList className="flex-1">
          <TabsTrigger value="canvas">画布</TabsTrigger>
          <TabsTrigger value="assets">资产</TabsTrigger>
        </TabsList>
        {!isMobile ? (
          <Button
            variant="ghost"
            size="icon-lg"
            aria-label="收起资产管理"
            onClick={closePanel}
          >
            <X />
          </Button>
        ) : null}
      </div>
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
          theme={theme}
        />
      </TabsContent>
      <TabsContent
        forceMount
        value="assets"
        className="min-h-0 flex-1 overflow-hidden data-[state=inactive]:hidden"
      >
        {(assetsVisited || tab === "assets") && (
          <CanvasAssetsTab
            key={`${projectId}:${assetsRevision}`}
            projectId={projectId}
            initialScope={savedAssetScope}
            onInsert={onInsertAsset}
          />
        )}
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
          side="left"
          className="gap-0 p-0"
          onCloseAutoFocus={(event) => {
            event.preventDefault();
            document
              .querySelector<HTMLButtonElement>(
                "[data-canvas-side-panel-trigger]",
              )
              ?.focus();
          }}
        >
          <SheetHeader>
            <SheetTitle>资产管理</SheetTitle>
            <SheetDescription>
              查找画布节点，选择并插入账户素材。
            </SheetDescription>
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
        className="relative flex h-full shrink-0 flex-col overflow-hidden border-r"
        initial={{ x: -48 }}
        animate={{ x: panelClosing ? -28 : 0 }}
        transition={{
          duration: resizing ? 0 : PANEL_MOTION_SECONDS,
          ease: PANEL_EASE,
        }}
        style={{
          width,
          background: theme.toolbar.panel,
          borderColor: theme.toolbar.border,
          color: theme.node.text,
        }}
        data-canvas-no-zoom
      >
        {panelContent}
        <Button
          variant="ghost"
          type="button"
          className="absolute inset-y-0 right-0 z-40 w-4 translate-x-1/2 cursor-col-resize"
          onPointerDown={startResize}
          aria-label={"调整左侧面板宽度"}
        />
      </motion.aside>
    </motion.div>
  );
}

// ---------------------------------------------------------------------------
// Canvas tab: list nodes and center, zoom, and select the clicked node.
// ---------------------------------------------------------------------------

const NODE_FILTER_VALUES = [
  "all",
  CanvasNodeType.Image,
  CanvasNodeType.Video,
  CanvasNodeType.Text,
  CanvasNodeType.Audio,
  CanvasNodeType.Config,
  CanvasNodeType.Group,
];

function nodePreviewText(node: CanvasNodeData) {
  if (node.type === CanvasNodeType.Text)
    return node.metadata?.content || node.metadata?.prompt || "";
  return getNodeDefinition(node.type)?.title || node.type;
}

function CanvasNodesTab({
  nodes,
  selectedNodeIds,
  onFocusNode,
  onPreviewNode,
  theme,
}: {
  nodes: CanvasNodeData[];
  selectedNodeIds: Set<string>;
  onFocusNode: (nodeId: string) => void;
  onPreviewNode: (nodeId: string) => void;
  theme: CanvasTheme;
}) {
  useSyncExternalStore(
    subscribeImagePreviews,
    getImagePreviewRevision,
    () => 0,
  );
  const [keyword, setKeyword] = useState("");
  const [typeFilter, setTypeFilter] = useState<string>("all");
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
        (typeFilter === "all" || node.type === typeFilter) &&
        (!query ||
          [node.title, node.metadata?.content, node.metadata?.prompt]
            .filter(Boolean)
            .join(" ")
            .toLowerCase()
            .includes(query)),
    );
  }, [nodes, keyword, typeFilter]);
  const treeRows = useMemo(() => {
    const filteredIds = new Set(filtered.map((node) => node.id));
    const groups = new Set(
      nodes
        .filter((node) => node.type === CanvasNodeType.Group)
        .map((node) => node.id),
    );
    const children = new Map<string, CanvasNodeData[]>();
    filtered.forEach((node) => {
      const groupId = node.metadata?.groupId;
      if (groupId && groups.has(groupId))
        children.set(groupId, [...(children.get(groupId) || []), node]);
    });
    return nodes.flatMap((node) => {
      if (node.metadata?.groupId && groups.has(node.metadata.groupId))
        return [];
      if (node.type !== CanvasNodeType.Group)
        return filteredIds.has(node.id)
          ? [{ node, depth: 0, hasChildren: false }]
          : [];
      const groupChildren = children.get(node.id) || [];
      if (!filteredIds.has(node.id) && !groupChildren.length) return [];
      return [
        { node, depth: 0, hasChildren: groupChildren.length > 0 },
        ...(collapsedGroups.has(node.id)
          ? []
          : groupChildren.map((child) => ({
              node: child,
              depth: 1,
              hasChildren: false,
            }))),
      ];
    });
  }, [collapsedGroups, filtered, nodes]);

  const exitSelect = () => {
    setSelectMode(false);
    setChecked(new Set());
  };
  const toggleChecked = (id: string) =>
    setChecked((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  const allChecked =
    filtered.length > 0 && filtered.every((node) => checked.has(node.id));
  const toggleAll = () =>
    setChecked(
      allChecked ? new Set() : new Set(filtered.map((node) => node.id)),
    );

  const handleExport = () => {
    const targets = nodes.filter((node) => checked.has(node.id));
    if (!targets.length) return;
    try {
      setDownload(canvasZipSelection(targets, nodes));
    } catch (error) {
      notifyError(error, "素材ZIP尚不可打包");
    }
  };

  return (
    <div className="flex h-full flex-col">
      <div className="flex items-center gap-2 px-3 pt-1 pb-2.5">
        <span className="text-xs font-medium opacity-60">{"画布元素"}</span>
        {filtered.length ? (
          <span className="text-xs opacity-35">{filtered.length}</span>
        ) : null}
        <Button
          variant="ghost"
          type="button"
          onClick={() => (selectMode ? exitSelect() : setSelectMode(true))}
          className="ml-auto flex items-center gap-1 px-1.5 py-1 opacity-70 transition hover:opacity-100"
          style={
            selectMode
              ? { color: theme.toolbar.activeText, opacity: 1 }
              : undefined
          }
        >
          <ListChecks />
          {selectMode ? "取消" : "选择"}
        </Button>
        {selectMode ? null : (
          <Select
            value={String(typeFilter ?? "")}
            onValueChange={(value) => {
              const option = NODE_FILTER_VALUES.map((value) => ({
                value,
                label:
                  value === "all"
                    ? "全部"
                    : (
                        {
                          image: "图片",
                          video: "视频",
                          text: "文本",
                          audio: "音频",
                          config: "配置",
                          group: "分组",
                        } as Record<string, string>
                      )[String(value)] || String(value),
              })).find((item) => String(item.value) === value);
              if (option) setTypeFilter(option.value);
            }}
          >
            <SelectTrigger className="w-20" aria-label="节点类型筛选">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectGroup>
                {NODE_FILTER_VALUES.map((value) => ({
                  value,
                  label:
                    value === "all"
                      ? "全部"
                      : (
                          {
                            image: "图片",
                            video: "视频",
                            text: "文本",
                            audio: "音频",
                            config: "配置",
                            group: "分组",
                          } as Record<string, string>
                        )[String(value)] || String(value),
                })).map((option) => (
                  <SelectItem
                    key={String(option.value)}
                    value={String(option.value)}
                  >
                    {option.label}
                  </SelectItem>
                ))}
              </SelectGroup>
            </SelectContent>
          </Select>
        )}
      </div>
      <div className="px-3 pb-2.5">
        <InputGroup>
          <InputGroupInput
            aria-label="搜索节点"
            placeholder={"搜索节点"}
            value={keyword}
            onChange={(e) => setKeyword(e.target.value)}
          />
          <InputGroupAddon>
            {<Search className="text-muted-foreground" />}
          </InputGroupAddon>

          <InputGroupAddon align="inline-end">
            <InputGroupButton aria-label="清空" onClick={() => setKeyword("")}>
              <X />
            </InputGroupButton>
          </InputGroupAddon>
        </InputGroup>
      </div>
      <div className="min-h-0 flex-1 [scrollbar-gutter:stable] overflow-y-auto px-2 pb-3">
        {treeRows.length ? (
          <div className="flex flex-col gap-1.5">
            {treeRows.map(({ node, depth, hasChildren }) => {
              const Icon = NODE_TYPE_ICON[node.type] || FileText;
              const isImage =
                node.type === CanvasNodeType.Image && node.metadata?.content;
              const isChecked = checked.has(node.id);
              const active = selectMode
                ? isChecked
                : selectedNodeIds.has(node.id);
              return (
                <div
                  key={node.id}
                  className={cn(
                    "group relative flex items-center rounded-lg transition",
                    depth && "ml-5",
                    active
                      ? ""
                      : "hover:bg-media-backdrop/5 dark:hover:bg-media-foreground/5",
                  )}
                  style={
                    active ? { background: theme.toolbar.activeBg } : undefined
                  }
                >
                  {depth ? (
                    <span
                      className="pointer-events-none absolute top-[calc(-50%-0.4rem)] -left-3 h-[calc(100%+0.4rem)] w-3 rounded-bl-md border-b border-l opacity-45"
                      style={{ borderColor: theme.node.stroke }}
                    />
                  ) : null}
                  {node.type === CanvasNodeType.Group && hasChildren ? (
                    <Button
                      variant="ghost"
                      type="button"
                      onClick={() =>
                        setCollapsedGroups((prev) =>
                          prev.has(node.id)
                            ? new Set([...prev].filter((id) => id !== node.id))
                            : new Set(prev).add(node.id),
                        )
                      }
                      className="ml-1 grid size-6 shrink-0 place-items-center opacity-55 transition hover:opacity-100"
                      aria-label={node.title}
                    >
                      <ChevronRight
                        className={cn(
                          "size-3.5 transition-transform",
                          !collapsedGroups.has(node.id) && "rotate-90",
                        )}
                      />
                    </Button>
                  ) : null}
                  {selectMode && (
                    <Checkbox
                      className="ml-2"
                      checked={isChecked}
                      onCheckedChange={() => toggleChecked(node.id)}
                      aria-label={`选择节点：${node.title || "未命名节点"}`}
                    />
                  )}
                  <Button
                    variant="ghost"
                    type="button"
                    onClick={() =>
                      selectMode ? toggleChecked(node.id) : onFocusNode(node.id)
                    }
                    className={cn(
                      "flex h-auto min-w-0 flex-1 items-center gap-3 py-2 pr-2 text-left",
                      node.type === CanvasNodeType.Group && hasChildren
                        ? "pl-0"
                        : "pl-2",
                    )}
                    title={selectMode ? undefined : "定位到节点"}
                    aria-pressed={selectMode ? isChecked : undefined}
                  >
                    <span className="grid size-10 shrink-0 place-items-center overflow-hidden rounded-md">
                      {isImage ? (
                        <PreviewImage
                          src={
                            previewUrlFor(node.metadata?.storageKey) ||
                            node.metadata?.content
                          }
                          alt={node.title}
                          className="size-full object-cover"
                        />
                      ) : (
                        <Icon className="size-5 opacity-60" />
                      )}
                    </span>
                    <span className="flex min-w-0 flex-1 flex-col gap-0.5">
                      <span className="block truncate text-sm leading-snug font-medium">
                        {node.title ||
                          getNodeDefinition(node.type)?.title ||
                          "未命名节点"}
                      </span>
                      <span className="block truncate text-xs leading-snug opacity-50">
                        {nodePreviewText(node)}
                      </span>
                    </span>
                    {node.metadata?.status &&
                    node.metadata.status !== "idle" ? (
                      <Badge variant="secondary">
                        {STATUS_LABEL[node.metadata.status] ||
                          node.metadata.status}
                      </Badge>
                    ) : null}
                  </Button>
                  {selectMode || !isImage ? null : (
                    <div className="flex shrink-0 flex-col items-center gap-0.5 pr-1.5">
                      <Button
                        variant="ghost"
                        type="button"
                        onClick={() => onPreviewNode(node.id)}
                        className="grid size-7 place-items-center opacity-55 transition hover:opacity-100"
                        aria-label={"放大预览"}
                        title={"放大预览"}
                      >
                        <Eye />
                      </Button>
                    </div>
                  )}
                </div>
              );
            })}
          </div>
        ) : (
          <Empty>
            <EmptyHeader>
              <EmptyTitle>画布暂无匹配节点</EmptyTitle>
            </EmptyHeader>
          </Empty>
        )}
      </div>
      {selectMode ? (
        <div
          className="flex items-center gap-2 border-t px-3 py-2.5"
          style={{ borderColor: theme.toolbar.border }}
        >
          <Button
            variant="ghost"
            type="button"
            onClick={toggleAll}
            className="px-2 py-1 opacity-70 transition hover:opacity-100"
          >
            {allChecked ? "取消全选" : "全选"}
          </Button>
          <span className="text-xs opacity-45">{`已选 ${checked.size}`}</span>
          <Button
            variant="ghost"
            type="button"
            onClick={() => void handleExport()}
            disabled={!checked.size}
            className="ml-auto flex items-center gap-1.5 px-3 py-1.5 transition disabled:cursor-not-allowed disabled:opacity-40"
          >
            <Download />
            {"打包下载"}
          </Button>
        </div>
      ) : null}
      {download && (
        <ZipDownloadDialog
          request={download}
          onClose={() => setDownload(undefined)}
        />
      )}
    </div>
  );
}

// ---------------------------------------------------------------------------
// Assets tab: library scope, type filtering, and click-to-insert.
// ---------------------------------------------------------------------------

function CanvasAssetsTab({
  projectId,
  initialScope,
  onInsert,
}: {
  projectId?: string;
  initialScope?: string;
  onInsert: (payload: InsertAssetPayload) => void;
}) {
  return (
    <div className="h-full min-h-0 px-3 py-2">
      <LibraryPickerContent
        compact
        insertOnUpload={false}
        projectId={projectId}
        initialScope={initialScope}
        onInsert={onInsert}
      />
    </div>
  );
}
