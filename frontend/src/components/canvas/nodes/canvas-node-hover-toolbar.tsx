import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { isFixedCanvasMedia } from "../lib/canvas-media-source";
import { useAppFeedback } from "@/components/canvas/runtime/components/feedback/app-feedback-provider";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { cn } from "@/lib/utils";
import {
  Download,
  Ellipsis,
  FolderPlus,
  Image as ImageIcon,
  Info,
  MessageSquare,
  Maximize2,
  Minus,
  Music2,
  Plus,
  RefreshCw,
  Settings2,
  Trash2,
  X,
  Ungroup,
  Upload,
  Video,
} from "lucide-react";
import {
  useCallback,
  useEffect,
  useMemo,
  useState,
  type ReactNode,
} from "react";

import { useCopyText } from "@/components/canvas/runtime/hooks/use-copy-text";
import { canvasThemes } from "@/components/canvas/runtime/lib/theme";
import { getNodeDefinition } from "@/components/canvas/lib/node-registry";
import {
  formatBytes,
  getDataUrlByteSize,
} from "@/components/canvas/runtime/lib/media/image-utils";
import { useThemeStore } from "@/components/canvas/runtime/store/use-theme-store";
import {
  CanvasNodeType,
  type CanvasNodeData,
  type ViewportTransform,
} from "@/components/canvas/types/canvas";
import type { CanvasNodeToolbarItem } from "@/components/canvas/types/canvas-plugin";
import {
  ImageToolSettingsModal,
  type ImageToolbarSettingsTool,
} from "../parameters/canvas-image-toolbar-settings-modal";
import {
  buildImageToolbarTools,
  type ImageQuickToolId,
} from "../lib/canvas-image-toolbar-tools";

type CanvasNodeHoverToolbarProps = {
  node: CanvasNodeData | null;
  viewport: ViewportTransform;
  canvasSize: { width: number; height: number };
  onKeep: (nodeId: string) => void;
  onLeave: () => void;
  onInfo: (node: CanvasNodeData) => void;
  onDecreaseFont: (node: CanvasNodeData) => void;
  onIncreaseFont: (node: CanvasNodeData) => void;
  onToggleDialog: (node: CanvasNodeData) => void;
  onGenerateImage: (node: CanvasNodeData) => void;
  onUpload: (node: CanvasNodeData) => void;
  onRemoveAsset?: (node: CanvasNodeData) => void;
  onDownload: (node: CanvasNodeData) => void;
  onSaveAsset: (node: CanvasNodeData) => void;
  onMaskEdit: (node: CanvasNodeData) => void;
  onCrop: (node: CanvasNodeData) => void;
  onSplit: (node: CanvasNodeData) => void;
  onUpscale: (node: CanvasNodeData) => void;
  onSuperResolve: (node: CanvasNodeData) => void;
  onAngle: (node: CanvasNodeData) => void;
  onViewImage: (node: CanvasNodeData) => void;
  onReversePrompt: (node: CanvasNodeData) => void;
  onRetry: (node: CanvasNodeData) => void;
  onToggleFreeResize: (node: CanvasNodeData) => void;
  onDelete: (node: CanvasNodeData) => void;
  onUngroup?: (node: CanvasNodeData) => void;
  extraTools?: CanvasNodeToolbarItem[];
};

type ToolbarTool = {
  id: string;
  title: string;
  label: string;
  icon: ReactNode;
  onClick: () => void;
  active?: boolean;
  danger?: boolean;
};

export function CanvasNodeHoverToolbar({
  node,
  viewport,
  canvasSize,
  onKeep,
  onLeave,
  onInfo,
  onDecreaseFont,
  onIncreaseFont,
  onToggleDialog,
  onGenerateImage,
  onUpload,
  onRemoveAsset,
  onDownload,
  onSaveAsset,
  onMaskEdit,
  onCrop,
  onSplit,
  onUpscale,
  onSuperResolve,
  onAngle,
  onViewImage,
  onReversePrompt,
  onRetry,
  onToggleFreeResize,
  onDelete,
  onUngroup,
  extraTools = [],
}: CanvasNodeHoverToolbarProps) {
  const theme = canvasThemes[useThemeStore((state) => state.theme)];
  const [quickImageToolIds, setQuickImageToolIds] = useState<
    ImageQuickToolId[]
  >(["view", "download"]);
  const [showImageToolLabels, setShowImageToolLabels] = useState(false);
  const [draftImageToolIds, setDraftImageToolIds] = useState<
    ImageQuickToolId[]
  >(["view", "download"]);
  const [draftShowImageToolLabels, setDraftShowImageToolLabels] =
    useState(false);
  const [imageToolSettingsOpen, setImageToolSettingsOpen] = useState(false);
  const [moreOpen, setMoreOpen] = useState(false);
  const { message } = useAppFeedback();
  const copyText = useCopyText();
  const [toolbarSize, setToolbarSize] = useState({ width: 0, height: 48 });
  const measureToolbar = useCallback((element: HTMLDivElement | null) => {
    if (!element) return;
    const measure = () => {
      const width = element.offsetWidth;
      const height = element.offsetHeight;
      setToolbarSize((current) =>
        current.width === width && current.height === height
          ? current
          : { width, height },
      );
    };
    measure();
    const observer = new ResizeObserver(measure);
    observer.observe(element);
    return () => observer.disconnect();
  }, []);

  useEffect(() => {
    // 选中的节点或编辑弹窗改变时重置本地草稿，避免沿用上一张素材的输入。
    // eslint-disable-next-line react-hooks/set-state-in-effect
    setImageToolSettingsOpen(false);
  }, [node?.id]);

  if (!node) return null;

  const activeNode = node;
  const left = Math.max(
    8 + toolbarSize.width / 2,
    Math.min(
      viewport.x + (node.position.x + node.width / 2) * viewport.k,
      canvasSize.width - 8 - toolbarSize.width / 2,
    ),
  );
  const top = Math.max(
    toolbarSize.height + 8,
    Math.min(
      viewport.y + node.position.y * viewport.k - 44,
      canvasSize.height - 8,
    ),
  );
  const isImage = node.type === CanvasNodeType.Image;
  const isVideo = node.type === CanvasNodeType.Video;
  const isAudio = node.type === CanvasNodeType.Audio;
  const hasImage = isImage && Boolean(node.metadata?.content);
  const hasVideo = isVideo && Boolean(node.metadata?.content);
  const hasAudio = isAudio && Boolean(node.metadata?.content);
  const isText = node.type === CanvasNodeType.Text;
  const isConfig = node.type === CanvasNodeType.Config;
  const fixedMedia = isFixedCanvasMedia(node);
  const canRetry =
    !fixedMedia &&
    node.metadata?.status === "error" &&
    !(isVideo && Boolean(node.metadata?.videoTaskId) && !hasVideo);
  const canQueryVideoTask =
    isVideo &&
    Boolean(node.metadata?.videoTaskId) &&
    !hasVideo &&
    node.metadata?.status !== "loading";
  const copyImagePrompt = (target: CanvasNodeData) => {
    const prompt = target.metadata?.prompt?.trim();
    if (!prompt) {
      message.warning("暂无可复制的提示词");
      return;
    }
    copyText(prompt, "提示词已复制");
  };
  const imageTools = buildImageToolbarTools(node, {
    onUpload,
    onToggleFreeResize,
    onMaskEdit,
    onCrop,
    onSplit,
    onUpscale,
    onSuperResolve,
    onAngle,
    onViewImage,
    onCopyPrompt: copyImagePrompt,
    onReversePrompt,
  });

  function openImageToolSettings() {
    onKeep(activeNode.id);
    setDraftImageToolIds(quickImageToolIds);
    setDraftShowImageToolLabels(showImageToolLabels);
    setImageToolSettingsOpen(true);
  }

  const baseToolbarTools: ToolbarTool[] = [
    {
      id: "info",
      title: "查看节点信息",
      label: "信息",
      icon: <Info className="size-4" />,
      onClick: () => onInfo(node),
    },
    ...(node.type === CanvasNodeType.Group && onUngroup
      ? [
          {
            id: "ungroup",
            title: "取消节点分组",
            label: "解散组",
            icon: <Ungroup className="size-4" />,
            onClick: () => onUngroup(node),
          },
        ]
      : []),
    {
      id: "delete",
      title: "移除节点",
      label: "删除",
      icon: <Trash2 className="size-4" />,
      onClick: () => onDelete(node),
      danger: true,
    },
  ];
  const nodeToolbarTools: ToolbarTool[] = [
    ...(canQueryVideoTask
      ? [
          {
            id: "queryVideoTask",
            title: "使用任务 ID 查询视频生成状态",
            label: "获取任务状态",
            icon: <RefreshCw className="size-4" />,
            onClick: () => onRetry(node),
          },
        ]
      : []),
    ...(canRetry
      ? [
          {
            id: "retry",
            title: "重新生成",
            label: "重试",
            icon: <RefreshCw className="size-4" />,
            onClick: () => onRetry(node),
          },
        ]
      : []),
    ...(hasImage || hasVideo || isText
      ? [
          {
            id: "saveAsset",
            title: "加入我的资产",
            label: "存资产",
            icon: <FolderPlus className="size-4" />,
            onClick: () => onSaveAsset(node),
          },
        ]
      : []),
    ...(hasImage || hasVideo || hasAudio
      ? [
          {
            id: "download",
            title: hasAudio ? "下载音频" : hasVideo ? "下载视频" : "下载图片",
            label: "下载",
            icon: <Download className="size-4" />,
            onClick: () => onDownload(node),
          },
        ]
      : []),
    ...(isVideo && !fixedMedia
      ? [
          {
            id: "edit",
            title: "编辑",
            label: "编辑",
            icon: <MessageSquare className="size-4" />,
            onClick: () => onToggleDialog(node),
          },
        ]
      : []),
    ...(isText
      ? [
          {
            id: "generateImage",
            title: "用文本生图",
            label: "生图",
            icon: <ImageIcon className="size-4" />,
            onClick: () => onGenerateImage(node),
          },
        ]
      : []),
    ...(isConfig
      ? [
          {
            id: "config",
            title: "生成配置",
            label: "生成配置",
            icon: <Settings2 className="size-4" />,
            onClick: () => onToggleDialog(node),
          },
        ]
      : []),
    ...(isText
      ? [
          {
            id: "decreaseFont",
            title: "减小字号",
            label: "缩小",
            icon: <Minus className="size-4" />,
            onClick: () => onDecreaseFont(node),
          },
        ]
      : []),
    ...(isText
      ? [
          {
            id: "increaseFont",
            title: "增大字号",
            label: "放大",
            icon: <Plus className="size-4" />,
            onClick: () => onIncreaseFont(node),
          },
        ]
      : []),
    ...(isImage && !hasImage
      ? [
          {
            id: "uploadImage",
            title: "上传图片",
            label: "上传图片",
            icon: <Upload className="size-4" />,
            onClick: () => onUpload(node),
          },
        ]
      : []),
    ...(isVideo
      ? [
          {
            id: "uploadVideo",
            title: hasVideo ? "替换视频" : "上传视频",
            label: hasVideo ? "替换视频" : "上传视频",
            icon: <Video className="size-4" />,
            onClick: () => onUpload(node),
          },
        ]
      : []),
    ...(isAudio
      ? [
          {
            id: "uploadAudio",
            title: hasAudio ? "替换音频" : "上传音频",
            label: hasAudio ? "替换音频" : "上传音频",
            icon: <Music2 className="size-4" />,
            onClick: () => onUpload(node),
          },
        ]
      : []),
    ...(hasImage
      ? imageTools.map((tool) => ({
          id: tool.id,
          title: tool.title,
          label: tool.label,
          icon: tool.icon,
          active: tool.active,
          onClick: tool.onClick,
        }))
      : []),
  ];
  const toolbarTools = hasImage
    ? quickImageToolIds.flatMap((id) =>
        [...baseToolbarTools, ...nodeToolbarTools].filter(
          (tool) => tool.id === id,
        ),
      )
    : [...baseToolbarTools, ...nodeToolbarTools, ...extraTools];
  const primaryTools = hasImage
    ? toolbarTools
    : hasVideo
      ? [
          {
            id: "preview",
            title: hasVideo ? "预览视频" : "预览图片",
            label: "预览",
            icon: <Maximize2 className="size-4" />,
            onClick: () => onViewImage(node),
          },
          ...toolbarTools.filter((tool) => tool.id === "download"),
        ]
      : toolbarTools.filter(
          (tool) => !["info", "delete", "saveAsset"].includes(tool.id),
        );
  const moreTools = [
    ...baseToolbarTools,
    ...nodeToolbarTools,
    ...extraTools,
  ].filter(
    (tool) =>
      !primaryTools.some((primary) => primary.id === tool.id) &&
      tool.id !== "view",
  );
  const selectableImageToolbarTools = [
    ...baseToolbarTools,
    ...nodeToolbarTools,
  ].filter((tool) => tool.id !== "retry") as ImageToolbarSettingsTool[];

  const closeImageToolSettings = () => {
    setImageToolSettingsOpen(false);
    onLeave();
  };

  const setDraftImageToolVisible = (id: ImageQuickToolId, visible: boolean) => {
    setDraftImageToolIds((current) => {
      const selected = new Set(current);
      if (visible) selected.add(id);
      else selected.delete(id);
      return selectableImageToolbarTools
        .filter((tool) => selected.has(tool.id))
        .map((tool) => tool.id);
    });
  };

  const saveImageToolSettings = () => {
    const config = {
      ids: draftImageToolIds,
      showLabels: draftShowImageToolLabels,
    };
    setQuickImageToolIds(config.ids);
    setShowImageToolLabels(config.showLabels);
    closeImageToolSettings();
  };

  return (
    <>
      <div
        ref={measureToolbar}
        role="toolbar"
        aria-label="节点工具栏"
        className="shadow-overlay absolute z-[70] flex min-h-11 w-max -translate-x-1/2 -translate-y-full flex-wrap items-center overflow-visible rounded-xl border text-sm"
        style={{
          left,
          top,
          maxWidth: Math.max(0, canvasSize.width - 16),
          background: theme.toolbar.panel,
          borderColor: theme.toolbar.border,
          color: theme.node.text,
        }}
        onMouseEnter={() => onKeep(node.id)}
        onMouseLeave={() => {
          if (!imageToolSettingsOpen && !moreOpen) onLeave();
        }}
        onMouseDown={(event) => event.stopPropagation()}
        onPointerDown={(event) => event.stopPropagation()}
      >
        {primaryTools.map((tool) => (
          <ToolbarAction
            key={tool.id}
            {...tool}
            showLabel={isImage && showImageToolLabels}
          />
        ))}
        <DropdownMenu
          open={moreOpen}
          onOpenChange={(open) => {
            setMoreOpen(open);
            if (open) onKeep(node.id);
          }}
        >
          <DropdownMenuTrigger asChild>
            <Button variant="ghost" size="icon" aria-label="更多节点操作">
              <Ellipsis />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent
            className="z-[80] w-52"
            align="end"
            data-canvas-no-zoom
            onPointerDown={(event) => event.stopPropagation()}
          >
            {moreTools.map((tool) => (
              <DropdownMenuItem
                key={tool.id}
                variant={tool.danger ? "destructive" : undefined}
                onSelect={tool.onClick}
              >
                {tool.icon}
                {tool.title}
              </DropdownMenuItem>
            ))}
            {fixedMedia && onRemoveAsset && (
              <DropdownMenuItem onSelect={() => onRemoveAsset(node)}>
                <X />
                移除素材
              </DropdownMenuItem>
            )}
            {hasImage && (
              <DropdownMenuItem onSelect={openImageToolSettings}>
                <Settings2 />
                配置快捷工具
              </DropdownMenuItem>
            )}
          </DropdownMenuContent>
        </DropdownMenu>
      </div>
      {hasImage ? (
        <ImageToolSettingsModal
          open={imageToolSettingsOpen}
          tools={selectableImageToolbarTools}
          selectedIds={draftImageToolIds}
          showLabels={draftShowImageToolLabels}
          onToggle={setDraftImageToolVisible}
          onShowLabelsChange={setDraftShowImageToolLabels}
          onCancel={closeImageToolSettings}
          onSave={saveImageToolSettings}
        />
      ) : null}
    </>
  );
}

export function CanvasNodeInfoModal({
  node,
  open,
  onClose,
}: {
  node: CanvasNodeData | null;
  open: boolean;
  onClose: () => void;
}) {
  const theme = canvasThemes[useThemeStore((state) => state.theme)];
  const [view, setView] = useState<"info" | "json">("info");
  const imageBytes =
    node?.type === CanvasNodeType.Image && node.metadata?.content
      ? getDataUrlByteSize(node.metadata.content)
      : 0;
  const batchCount =
    node?.type === CanvasNodeType.Image
      ? node.metadata?.images?.length || 0
      : 0;
  const json = useMemo(() => {
    if (!node) return "";
    return JSON.stringify(
      node,
      (key, value) => {
        if (
          key === "content" &&
          typeof value === "string" &&
          value.startsWith("data:image/")
        ) {
          return "[base64 image]";
        }
        return value;
      },
      2,
    );
  }, [node]);

  useEffect(() => {
    // 选中的节点或编辑弹窗改变时重置本地草稿，避免沿用上一张素材的输入。
    // eslint-disable-next-line react-hooks/set-state-in-effect
    if (open) setView("info");
  }, [node?.id, open]);

  const title = (
    <div className="flex items-center justify-between gap-4 pr-12">
      <span>{"节点信息"}</span>
      <ToggleGroup
        type="single"
        variant="outline"
        value={String(view)}
        size="sm"
        onValueChange={(value) => {
          if (value) ((value) => setView(value as "info" | "json"))(value);
        }}
      >
        {[
          { label: "信息", value: "info" },
          { label: "JSON", value: "json" },
        ].map((item) => {
          const option =
            typeof item === "object" ? item : { value: item, label: item };
          return (
            <ToggleGroupItem
              key={String(option.value)}
              value={String(option.value)}
            >
              {option.label}
            </ToggleGroupItem>
          );
        })}
      </ToggleGroup>
    </div>
  );

  return (
    <Dialog
      open={open && Boolean(node)}
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
    >
      <DialogContent
        className={cn("max-h-dialog overflow-y-auto", "canvas-node-info-modal")}
      >
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
          <DialogDescription className="sr-only">
            查看当前节点的属性与媒体信息。
          </DialogDescription>
        </DialogHeader>
        <div>
          {node ? (
            <div
              className="h-media-editor min-h-0 text-sm select-text"
              data-canvas-shortcuts-ignore
            >
              {view === "info" ? (
                <div className="thin-scrollbar flex h-full flex-col gap-3 overflow-auto pr-1">
                  <InfoRow label="ID" value={node.id} />
                  <InfoRow label={"名称"} value={node.title || "未命名节点"} />
                  <InfoRow
                    label={"类型"}
                    value={
                      node.type === CanvasNodeType.Group
                        ? "组"
                        : node.type === CanvasNodeType.Config
                          ? "生成配置"
                          : [
                                CanvasNodeType.Image,
                                CanvasNodeType.Video,
                                CanvasNodeType.Audio,
                                CanvasNodeType.Text,
                              ].includes(node.type as CanvasNodeType)
                            ? (
                                {
                                  text: "文本",
                                  image: "图片",
                                  video: "视频",
                                  audio: "音频",
                                } as Record<string, string>
                              )[String(node.type)] || String(node.type)
                            : getNodeDefinition(node.type)?.title || node.type
                    }
                  />
                  <InfoRow
                    label={"尺寸"}
                    value={`${Math.round(node.width)} x ${Math.round(node.height)}`}
                  />
                  <InfoRow
                    label={"位置"}
                    value={`${Math.round(node.position.x)}, ${Math.round(node.position.y)}`}
                  />
                  <InfoRow
                    label={"状态"}
                    value={node.metadata?.status || "idle"}
                  />
                  {batchCount > 1 ? (
                    <InfoRow label={"图片组"} value={`${batchCount} 张`} />
                  ) : null}
                  {node.metadata?.prompt ? (
                    <InfoRow label={"提示词"} value={node.metadata.prompt} />
                  ) : null}
                  {node.metadata?.videoTaskId ? (
                    <InfoRow
                      label={"任务 ID"}
                      value={node.metadata.videoTaskId}
                    />
                  ) : null}
                  {imageBytes ? (
                    <InfoRow
                      label={"图片大小"}
                      value={formatBytes(imageBytes)}
                    />
                  ) : null}
                </div>
              ) : (
                <pre
                  className="thin-scrollbar h-full overflow-auto rounded-lg border p-3 text-xs leading-5"
                  style={{
                    background: theme.node.fill,
                    borderColor: theme.node.stroke,
                    color: theme.node.text,
                  }}
                >
                  {json}
                </pre>
              )}
            </div>
          ) : null}
        </div>
      </DialogContent>
    </Dialog>
  );
}

function ToolbarAction({
  title,
  label,
  icon,
  onClick,
  showLabel,
  active = false,
  danger = false,
}: ToolbarTool & { showLabel: boolean }) {
  const hasText = showLabel && Boolean(label);
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          variant={danger ? "destructive" : active ? "secondary" : "ghost"}
          size={hasText ? "sm" : "icon-sm"}
          type="button"
          onClick={onClick}
          aria-label={title}
        >
          {icon}
          {hasText ? <span>{label}</span> : null}
        </Button>
      </TooltipTrigger>
      <TooltipContent side="top">{title}</TooltipContent>
    </Tooltip>
  );
}

function InfoRow({ label, value }: { label: string; value: ReactNode }) {
  return (
    <div className="grid-cols-info-field grid gap-3">
      <span className="opacity-50">{label}</span>
      <span className="min-w-0 break-words whitespace-pre-wrap">{value}</span>
    </div>
  );
}
