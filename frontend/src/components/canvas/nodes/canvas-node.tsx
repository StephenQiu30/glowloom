import { CanvasVideoPlayer } from "./canvas-video-player";
import { useBoundReferences } from "../hooks/use-bound-references";
import { cn } from "@/lib/utils";
import { isImeComposing } from "@/components/canvas/runtime/lib/keyboard-event";
import type { CanvasMediaDimensions } from "../hooks/use-canvas-media-size";
import { PreviewImage } from "@/components/canvas/runtime/components/media/preview-image";
import { Button } from "@/components/ui/button";
import { GenerationResultEmpty } from "@/components/canvas/runtime/components/generation/workspace/generation-result-empty";
import { Spinner } from "@/components/ui/spinner";
import { Input } from "@/components/ui/input";
import {
  Empty,
  EmptyHeader,
  EmptyMedia,
  EmptyDescription,
} from "@/components/ui/empty";
import {
  Popover,
  PopoverAnchor,
  PopoverContent,
} from "@/components/ui/popover";
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetTitle,
} from "@/components/ui/sheet";
import {
  ChevronRight,
  Copy,
  Download,
  FileText,
  Type,
  Play,
  Group,
  Image as ImageIcon,
  Music2,
  AudioLines,
  Puzzle,
  Plus,
  RefreshCw,
  Star,
  Trash2,
  Video,
} from "lucide-react";
import type { ReactNode } from "react";
import React, {
  useCallback,
  useEffect,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
  useSyncExternalStore,
} from "react";

import { canvasThemes } from "@/components/canvas/runtime/lib/theme";
import type { CanvasResourceReference } from "@/components/canvas/lib/canvas-resource-references";
import { getNodeDefinition } from "@/components/canvas/lib/node-registry";
import { buildNodeContext } from "@/components/canvas/lib/plugin-node-context";
import { pickImageSource } from "@/components/canvas/runtime/lib/media/image-thumbnail";
import { formatBytes } from "@/components/canvas/runtime/lib/media/image-utils";
import {
  getImagePreviewRevision,
  previewUrlFor,
  subscribeImagePreviews,
} from "@/components/canvas/runtime/lib/media/image-storage";
import { useThemeStore } from "@/components/canvas/runtime/store/use-theme-store";
import {
  CanvasNodeType,
  type CanvasNodeData,
  type CanvasNodeImage,
  type CanvasNodeText,
  type Position,
} from "@/components/canvas/types/canvas";
import type {
  CanvasNodeContext,
  CanvasPluginHost,
} from "@/components/canvas/types/canvas-plugin";
import { RichTextInput } from "@/components/canvas/runtime/components/editor/rich-text/rich-text-input";
import { documentText } from "@/components/canvas/runtime/components/editor/lib/rich-text-document";
import { RichTextContent } from "@/components/canvas/runtime/components/editor/rich-text/rich-text-content";
import type { RichTextDocument } from "@/components/canvas/runtime/components/editor/types/rich-text";

import {
  resizeCanvasNode,
  type CanvasResizeHandle as ResizeCorner,
} from "../lib/canvas-node-resize";
import { getNodeSpec } from "../constants/canvas";
const selectionBlue = "var(--ring)";

type CanvasNodeProps = {
  readOnly?: boolean;
  acceptContent?: (value: string, document: RichTextDocument) => boolean;
  collaboration?: import("@glowloom/canvas-collab/model").NodeTextBinding;
  data: CanvasNodeData;
  scale: number;
  isSelected: boolean;
  isRelated: boolean;
  isFocusRelated: boolean;
  isConnectionTarget: boolean;
  isConnecting: boolean;
  referenceSelectionState?: "target" | "disabled" | "available";
  showPanel: boolean;
  isMobile?: boolean;
  connectionTypes?: { input: boolean; output: boolean };
  showImageInfo: boolean;
  mentionReferences?: CanvasResourceReference[];
  pluginHost?: CanvasPluginHost;
  registryVersion?: number;
  renderPanel?: (node: CanvasNodeData) => ReactNode;
  onPanelClose?: (nodeId: string) => void;
  renderNodeContent?: (node: CanvasNodeData) => ReactNode;
  groupChildCount?: number;
  isGroupDropTarget?: boolean;
  batchExpanded?: boolean;
  onMouseDown: (event: React.MouseEvent, nodeId: string) => void;
  onSelectCapture?: (event: React.MouseEvent, nodeId: string) => void;
  onHoverStart: (nodeId: string) => void;
  onHoverEnd: (nodeId: string) => void;
  onConnectStart: (
    event: React.MouseEvent,
    nodeId: string,
    handleType: "source" | "target",
  ) => void;
  onResizeStart: (nodeId: string) => void;
  onResize: (
    nodeId: string,
    width: number,
    height: number,
    position?: Position,
  ) => void;
  onResizeEnd: (nodeId: string) => void;
  onContentChange: (
    nodeId: string,
    content: string,
    document?: RichTextDocument,
  ) => void;
  onTitleChange: (nodeId: string, title: string) => void;
  onToggleBatch?: (nodeId: string) => void;
  onSetBatchPrimary?: (nodeId: string, itemId: string) => void;
  onDuplicateBatchImage?: (node: CanvasNodeData, imageId: string) => void;
  onDownloadBatchImage?: (node: CanvasNodeData, imageId: string) => void;
  onRetryBatchImage?: (node: CanvasNodeData, imageId: string) => void;
  onDeleteBatchImage?: (nodeId: string, imageId: string) => void;
  onRetry?: (node: CanvasNodeData) => void;
  onPreviewLoaded?: (
    nodeId: string,
    source: string,
    dimensions?: CanvasMediaDimensions,
  ) => void;
  onPreviewError?: (nodeId: string, source: string) => void;
  onViewImage?: (node: CanvasNodeData, imageId?: string) => void;
  onSelectReference?: (nodeId: string) => void;
  onContextMenu: (event: React.MouseEvent, nodeId: string) => void;
};

type NodeContentRendererProps = {
  readOnly?: boolean;
  acceptContent?: (value: string, document: RichTextDocument) => boolean;
  collaboration?: import("@glowloom/canvas-collab/model").NodeTextBinding;
  node: CanvasNodeData;
  theme: (typeof canvasThemes)[keyof typeof canvasThemes];
  isEditingContent: boolean;
  textareaRef: React.RefObject<HTMLDivElement | null>;
  isBatchRoot: boolean;
  batchCount: number;
  batchExpanded: boolean;
  scale: number;
  renderNodeContent?: (node: CanvasNodeData) => ReactNode;
  pluginContext?: CanvasNodeContext | null;
  onContentChange: (
    nodeId: string,
    content: string,
    document?: RichTextDocument,
  ) => void;
  onStopEditing: () => void;
  mentionReferences: CanvasResourceReference[];
  onRetry?: (node: CanvasNodeData) => void;
  onToggleBatch?: () => void;
  onSetBatchPrimary?: (itemId: string) => void;
  onDuplicateBatchImage?: (imageId: string) => void;
  onDownloadBatchImage?: (imageId: string) => void;
  onRetryBatchImage?: (imageId: string) => void;
  onDeleteBatchImage?: (imageId: string) => void;
  onViewBatchImage?: (imageId: string) => void;
  groupChildCount: number;
};

export const CanvasNode = React.memo(function CanvasNode({
  readOnly = false,
  acceptContent,
  collaboration,
  data,
  scale,
  isSelected,
  isFocusRelated,
  isConnectionTarget,
  isConnecting,
  referenceSelectionState,
  showPanel,
  isMobile = false,
  connectionTypes,
  showImageInfo,
  mentionReferences = [],
  pluginHost,
  renderPanel,
  onPanelClose,
  renderNodeContent,
  groupChildCount = 0,
  isGroupDropTarget = false,
  batchExpanded = false,
  onMouseDown,
  onSelectCapture,
  onHoverStart,
  onHoverEnd,
  onConnectStart,
  onResizeStart,
  onResize,
  onResizeEnd,
  onContentChange,
  onTitleChange,
  onToggleBatch,
  onSetBatchPrimary,
  onDuplicateBatchImage,
  onDownloadBatchImage,
  onRetryBatchImage,
  onDeleteBatchImage,
  onRetry,
  onPreviewLoaded,
  onPreviewError,
  onViewImage,
  onSelectReference,
  onContextMenu,
}: CanvasNodeProps) {
  const nodeElement = useRef<HTMLDivElement>(null);
  const restorePanelFocus = useRef(true);
  const selectedForFocus = useRef(isSelected);
  useLayoutEffect(() => {
    selectedForFocus.current = isSelected;
  }, [isSelected]);
  const theme = canvasThemes[useThemeStore((state) => state.theme)];
  const [hovered, setHovered] = useState(false);
  const definition = getNodeDefinition(data.type);
  const pluginContext = useMemo<CanvasNodeContext | null>(
    () =>
      pluginHost
        ? buildNodeContext(pluginHost, data, theme, scale, isSelected)
        : null,
    [pluginHost, data, theme, scale, isSelected],
  );
  const [isEditingContent, setIsEditingContent] = useState(false);
  const [isEditingTitle, setIsEditingTitle] = useState(false);
  const [titleDraft, setTitleDraft] = useState(data.title || "");
  const hasImageContent =
    data.type === CanvasNodeType.Image && Boolean(data.metadata?.content);
  const hasVideoContent =
    data.type === CanvasNodeType.Video && Boolean(data.metadata?.content);
  const isGroup = data.type === CanvasNodeType.Group;
  const batchCount =
    data.type === CanvasNodeType.Image
      ? data.metadata?.images?.length || 0
      : data.type === CanvasNodeType.Text
        ? data.metadata?.texts?.length || 0
        : 0;
  const isBatchRoot = batchCount > 1;
  // Nodes with the interaction/move toggle ignore content pointer events in move mode and allow interaction in interactive mode.
  // forceInteractive states such as editing stay interactive, as do empty nodes so their upload and generation actions remain usable.
  const supportsInteractionToggle = Boolean(definition?.interactionToggle);
  const forceInteractive = supportsInteractionToggle
    ? Boolean(definition?.forceInteractive?.(data))
    : false;
  const contentInteractive =
    !supportsInteractionToggle || forceInteractive || !data.metadata?.content
      ? true
      : Boolean(data.metadata?.interactive);
  // Media selection stays neutral; blue is reserved for connection targets.
  const transparentBg = Boolean(definition?.transparentBackground);
  const isActive = isConnectionTarget || isSelected || isFocusRelated;
  const imageBorderColor = isConnectionTarget
    ? selectionBlue
    : isSelected || isFocusRelated
      ? theme.node.muted
      : "transparent";
  const textareaRef = useRef<HTMLDivElement>(null);
  const titleInputRef = useRef<HTMLInputElement>(null);
  const resizeRef = useRef({
    isResizing: false,
    corner: "bottom-right" as ResizeCorner,
    startX: 0,
    startY: 0,
    startLeft: 0,
    startTop: 0,
    startWidth: 0,
    startHeight: 0,
    keepRatio: false,
    ratio: 1,
  });

  useEffect(() => {
    setTitleDraft(data.title || "");
  }, [data.title]);

  useEffect(() => {
    if (!isEditingTitle) return;
    titleInputRef.current?.focus();
    titleInputRef.current?.select();
  }, [isEditingTitle]);

  const finishTitleEditing = useCallback(() => {
    const title = titleDraft.trim() || data.title || "未命名节点";
    setTitleDraft(title);
    setIsEditingTitle(false);
    if (title !== data.title) onTitleChange(data.id, title);
  }, [
    data.id,
    data.title,
    onTitleChange,
    titleDraft,
    setTitleDraft,
    setIsEditingTitle,
  ]);

  useEffect(() => {
    if (!isEditingTitle) return;
    const handleOutsidePointerDown = (event: PointerEvent) => {
      const target = event.target;
      if (target instanceof Node && titleInputRef.current?.contains(target))
        return;
      finishTitleEditing();
    };
    window.addEventListener("pointerdown", handleOutsidePointerDown, true);
    return () =>
      window.removeEventListener("pointerdown", handleOutsidePointerDown, true);
  }, [finishTitleEditing, isEditingTitle]);

  useEffect(() => {
    const textarea = textareaRef.current;
    if (!textarea) return;

    const handleWheel = (event: WheelEvent) => event.stopPropagation();
    textarea.addEventListener("wheel", handleWheel, { passive: false });
    return () => textarea.removeEventListener("wheel", handleWheel);
  }, [data.type, isEditingContent]);

  useEffect(() => {
    if (!isEditingContent) return;
    const textarea = textareaRef.current;
    textarea?.querySelector<HTMLElement>('[contenteditable="true"]')?.focus();
  }, [isEditingContent]);

  useEffect(() => {
    if (!isEditingContent) return;

    const handleOutsidePointerDown = (event: PointerEvent) => {
      const target = event.target;
      if (!(target instanceof Node)) return;
      if (
        isEditingContent &&
        (textareaRef.current?.contains(target) ||
          (target instanceof Element &&
            target.closest(
              "[data-rich-text-floating], [data-slot='dialog-content']",
            )))
      )
        return;

      setIsEditingContent(false);
    };

    window.addEventListener("pointerdown", handleOutsidePointerDown, true);
    return () =>
      window.removeEventListener("pointerdown", handleOutsidePointerDown, true);
  }, [isEditingContent]);

  const handleResizeMove = useCallback(
    (event: MouseEvent) => {
      if (!resizeRef.current.isResizing) return;

      const dx = (event.clientX - resizeRef.current.startX) / scale;
      const dy = (event.clientY - resizeRef.current.startY) / scale;
      const current = resizeRef.current;
      const next = resizeCanvasNode(
        {
          x: current.startLeft,
          y: current.startTop,
          width: current.startWidth,
          height: current.startHeight,
        },
        current.corner,
        dx,
        dy,
        current.keepRatio ? current.ratio : undefined,
      );
      onResize(data.id, next.width, next.height, { x: next.x, y: next.y });
    },
    [data.id, onResize, scale],
  );

  const handleResizeUp = useCallback(
    function finishResize() {
      resizeRef.current.isResizing = false;
      window.removeEventListener("mousemove", handleResizeMove);
      window.removeEventListener("mouseup", finishResize);
      onResizeEnd(data.id);
    },
    [data.id, handleResizeMove, onResizeEnd],
  );

  const handleResizeMouseDown = (
    event: React.MouseEvent,
    corner: ResizeCorner,
  ) => {
    event.stopPropagation();
    event.preventDefault();
    onResizeStart(data.id);
    resizeRef.current = {
      isResizing: true,
      corner,
      startX: event.clientX,
      startY: event.clientY,
      startLeft: data.position.x,
      startTop: data.position.y,
      startWidth: data.width,
      startHeight: data.height,
      keepRatio:
        (data.type === CanvasNodeType.Image && !data.metadata?.freeResize) ||
        data.type === CanvasNodeType.Video ||
        Boolean(definition?.keepAspectRatio?.(data)),
      ratio:
        (data.metadata?.naturalWidth || data.width) /
        (data.metadata?.naturalHeight || data.height || 1),
    };
    window.addEventListener("mousemove", handleResizeMove);
    window.addEventListener("mouseup", handleResizeUp);
  };

  useEffect(() => {
    return () => {
      window.removeEventListener("mousemove", handleResizeMove);
      window.removeEventListener("mouseup", handleResizeUp);
    };
  }, [handleResizeMove, handleResizeUp]);

  const panelOpen = showPanel && !referenceSelectionState && !isGroup;
  return (
    <>
      <Popover
        open={panelOpen && !isMobile}
        onOpenChange={(open) => {
          if (!open) onPanelClose?.(data.id);
        }}
      >
        <PopoverAnchor asChild>
          <div
            ref={nodeElement}
            onLoadCapture={(event) => {
              const target = event.target;
              if (target instanceof HTMLImageElement)
                onPreviewLoaded?.(data.id, target.currentSrc || target.src, {
                  width: target.naturalWidth,
                  height: target.naturalHeight,
                });
            }}
            onLoadedDataCapture={(event) => {
              const target = event.target;
              if (target instanceof HTMLMediaElement)
                onPreviewLoaded?.(
                  data.id,
                  target.currentSrc || target.src,
                  target instanceof HTMLVideoElement
                    ? { width: target.videoWidth, height: target.videoHeight }
                    : undefined,
                );
            }}
            onErrorCapture={(event) => {
              const media = event.target;
              if (
                media instanceof HTMLImageElement ||
                media instanceof HTMLMediaElement
              ) {
                const source = media.getAttribute("src");
                if (source) onPreviewError?.(data.id, source);
              }
            }}
            tabIndex={-1}
            data-node-id={data.id}
            data-node-type={data.type}
            data-node-selected={isSelected}
            data-node-editing={isEditingContent}
            className={cn(
              "node-element group/node absolute flex flex-col transition-shadow duration-200 select-none",
              isGroup ? "z-[5]" : isSelected ? "z-50" : "z-10",
              referenceSelectionState === "available"
                ? "cursor-pointer"
                : referenceSelectionState
                  ? "cursor-not-allowed"
                  : "",
            )}
            style={{
              transform: `translate(${data.position.x}px, ${data.position.y}px)`,
              width: data.width,
              height: data.height,
              transition: "box-shadow 200ms ease",
              contain: "layout style",
            }}
            onMouseEnter={() => {
              setHovered(true);
              onHoverStart(data.id);
            }}
            onMouseLeave={() => {
              setHovered(false);
              onHoverEnd(data.id);
            }}
            onMouseDownCapture={(event) => {
              if (!referenceSelectionState) onSelectCapture?.(event, data.id);
            }}
            onContextMenu={(event) => {
              if (referenceSelectionState) event.preventDefault();
              else onContextMenu(event, data.id);
            }}
          >
            {!referenceSelectionState && (
              <div
                className="canvas-node-heading absolute top-[-28px] left-0 z-[65] max-w-canvas-dialog"
                style={{
                  top: -28 / scale,
                  transform: `scale(${1 / scale})`,
                  transformOrigin: "top left",
                }}
                onMouseDown={(event) => {
                  event.stopPropagation();
                  if (!isEditingTitle) onMouseDown(event, data.id);
                }}
                onPointerDown={(event) => event.stopPropagation()}
              >
                {isEditingTitle ? (
                  <Input
                    aria-label="节点名称"
                    ref={titleInputRef}
                    value={titleDraft}
                    disabled={readOnly}
                    maxLength={64}
                    className="h-6 max-w-full border-0 border-b border-dashed px-0 outline-none"
                    onChange={(event) => {
                      setTitleDraft(event.target.value);
                      onTitleChange(data.id, event.target.value);
                    }}
                    onBlur={finishTitleEditing}
                    onKeyDown={(event) => {
                      if (event.key === "Enter") finishTitleEditing();
                      if (event.key === "Escape") {
                        setTitleDraft(data.title || "");
                        setIsEditingTitle(false);
                      }
                    }}
                  />
                ) : (
                  <Button
                    variant="ghost"
                    type="button"
                    className="canvas-node-title max-w-full justify-start px-0"
                    title="双击或按 Enter 修改节点名称"
                    onKeyDown={(event) => {
                      if (
                        !readOnly &&
                        (event.key === "Enter" || event.key === "F2")
                      ) {
                        event.preventDefault();
                        setIsEditingTitle(true);
                      }
                    }}
                    onDoubleClick={(event) => {
                      event.stopPropagation();
                      if (!readOnly) setIsEditingTitle(true);
                    }}
                  >
                    {data.type === CanvasNodeType.Image ? (
                      <ImageIcon />
                    ) : data.type === CanvasNodeType.Video ? (
                      <Video />
                    ) : data.type === CanvasNodeType.Audio ? (
                      <AudioLines />
                    ) : data.type === CanvasNodeType.Text ? (
                      <Type />
                    ) : (
                      <FileText />
                    )}
                    <span className="truncate">
                      {data.title || "未命名节点"}
                    </span>
                  </Button>
                )}
              </div>
            )}

            <div
              className="canvas-node-body relative h-full w-full overflow-visible rounded-xl border"
              style={{
                background: isGroup
                  ? "transparent"
                  : hasImageContent || hasVideoContent || transparentBg
                    ? "transparent"
                    : theme.node.fill,
                borderColor: isGroup
                  ? isGroupDropTarget || isActive
                    ? selectionBlue
                    : theme.node.stroke
                  : hasImageContent
                    ? imageBorderColor
                    : isConnectionTarget
                      ? selectionBlue
                      : isSelected || isFocusRelated
                        ? theme.node.muted
                        : transparentBg
                          ? "transparent"
                          : theme.node.stroke,
                borderStyle: isGroup ? "dashed" : "solid",
                borderWidth: data.type === CanvasNodeType.Text ? 0 : undefined,
                outline:
                  data.type === CanvasNodeType.Text && isSelected
                    ? `${1 / scale}px solid color-mix(in srgb, var(--canvas-reference-foreground) 50%, transparent)`
                    : undefined,
                outlineOffset:
                  data.type === CanvasNodeType.Text ? 3 / scale : undefined,
                boxShadow: isGroupDropTarget
                  ? `0 0 0 2px color-mix(in srgb, ${selectionBlue} 40%, transparent), inset 0 0 0 999px color-mix(in srgb, ${selectionBlue} 6%, transparent)`
                  : isConnectionTarget
                    ? `0 0 0 1px color-mix(in srgb, ${selectionBlue} 33%, transparent)`
                    : undefined,
              }}
              onMouseDown={(event) => {
                if (!referenceSelectionState) onMouseDown(event, data.id);
                else if (
                  event.button === 0 &&
                  referenceSelectionState === "available"
                ) {
                  event.stopPropagation();
                  onSelectReference?.(data.id);
                }
              }}
              onDoubleClick={(event) => {
                if (referenceSelectionState) {
                  event.stopPropagation();
                  return;
                }
                if (!readOnly && definition?.onDoubleClick && pluginContext) {
                  if (definition.onDoubleClick(pluginContext))
                    event.stopPropagation();
                  return;
                }
                if (data.type === CanvasNodeType.Image && hasImageContent) {
                  event.stopPropagation();
                  onViewImage?.(data);
                  return;
                }
                if (readOnly || data.type !== CanvasNodeType.Text) return;
                event.stopPropagation();
                onPanelClose?.(data.id);
                setIsEditingContent(true);
              }}
              role={
                data.type === CanvasNodeType.Text && !isEditingContent
                  ? "button"
                  : undefined
              }
              tabIndex={
                data.type === CanvasNodeType.Text && !isEditingContent
                  ? 0
                  : undefined
              }
              aria-label={
                data.type === CanvasNodeType.Text && !isEditingContent
                  ? "编辑文本节点内容"
                  : undefined
              }
              onKeyDown={(event) => {
                if (
                  !readOnly &&
                  data.type === CanvasNodeType.Text &&
                  !isEditingContent &&
                  (event.key === "Enter" || event.key === " ")
                ) {
                  event.preventDefault();
                  event.stopPropagation();
                  onPanelClose?.(data.id);
                  setIsEditingContent(true);
                }
              }}
            >
              <div
                className={cn(
                  "relative flex h-full w-full items-center justify-center rounded-[inherit]",
                  isBatchRoot ? "overflow-visible" : "overflow-hidden",
                )}
                style={
                  {
                    background: isGroup
                      ? "transparent"
                      : hasImageContent || hasVideoContent || transparentBg
                        ? "transparent"
                        : data.type === CanvasNodeType.Text
                          ? "linear-gradient(to bottom right, var(--canvas-reference-text-start), var(--canvas-reference-text-end))"
                          : theme.node.fill,
                    pointerEvents: contentInteractive ? undefined : "none",
                  } as React.CSSProperties
                }
              >
                <NodeContent
                  readOnly={readOnly}
                  acceptContent={acceptContent}
                  collaboration={collaboration}
                  node={data}
                  theme={theme}
                  isEditingContent={isEditingContent}
                  textareaRef={textareaRef}
                  isBatchRoot={isBatchRoot}
                  batchCount={batchCount}
                  batchExpanded={batchExpanded}
                  scale={scale}
                  renderNodeContent={renderNodeContent}
                  pluginContext={pluginContext}
                  mentionReferences={mentionReferences}
                  onContentChange={onContentChange}
                  onStopEditing={() => setIsEditingContent(false)}
                  onRetry={onRetry}
                  onToggleBatch={() => onToggleBatch?.(data.id)}
                  onSetBatchPrimary={(itemId) =>
                    onSetBatchPrimary?.(data.id, itemId)
                  }
                  onDuplicateBatchImage={(imageId) =>
                    onDuplicateBatchImage?.(data, imageId)
                  }
                  onDownloadBatchImage={(imageId) =>
                    onDownloadBatchImage?.(data, imageId)
                  }
                  onRetryBatchImage={(imageId) =>
                    onRetryBatchImage?.(data, imageId)
                  }
                  onDeleteBatchImage={(imageId) =>
                    onDeleteBatchImage?.(data.id, imageId)
                  }
                  onViewBatchImage={(imageId) => onViewImage?.(data, imageId)}
                  groupChildCount={groupChildCount}
                />
              </div>

              {showImageInfo && hasImageContent ? (
                <ImageInfoBar node={data} />
              ) : null}

              {referenceSelectionState &&
              (referenceSelectionState !== "available" || hovered) ? (
                <div
                  className="pointer-events-none absolute inset-0 z-[60] grid place-items-center rounded-[inherit]"
                  style={{
                    background: `color-mix(in srgb, ${theme.canvas.background} ${referenceSelectionState === "target" ? 78 : referenceSelectionState === "disabled" ? 60 : 34}%, transparent)`,
                    boxShadow:
                      referenceSelectionState === "available"
                        ? `inset 0 0 0 2px ${selectionBlue}`
                        : undefined,
                  }}
                >
                  {referenceSelectionState !== "disabled" ? (
                    <span
                      className="rounded-lg px-3 py-2 text-sm font-medium shadow-sm"
                      style={{
                        background: theme.toolbar.panel,
                        color: theme.node.text,
                      }}
                    >
                      {referenceSelectionState === "target"
                        ? "正在添加参考"
                        : "选择"}
                    </span>
                  ) : null}
                </div>
              ) : null}

              {!readOnly &&
              !referenceSelectionState &&
              (isSelected || isEditingContent) ? (
                <ResizeHandles
                  scale={scale}
                  onMouseDown={handleResizeMouseDown}
                  onKeyDown={(event, handle) => {
                    if (isImeComposing(event)) return;
                    const horizontal =
                      event.key === "ArrowLeft" || event.key === "ArrowRight";
                    const vertical =
                      event.key === "ArrowUp" || event.key === "ArrowDown";
                    const reset =
                      event.key === "Enter" && !handle.includes("-");
                    if (!horizontal && !vertical && !reset) return;
                    event.preventDefault();
                    event.stopPropagation();
                    const step = event.shiftKey ? 10 : 1;
                    const defaults = getNodeSpec(data.type);
                    let dx = horizontal
                      ? event.key === "ArrowLeft"
                        ? -step
                        : step
                      : 0;
                    let dy = vertical
                      ? event.key === "ArrowUp"
                        ? -step
                        : step
                      : 0;
                    if (reset) {
                      dx =
                        (defaults.width - data.width) *
                        (handle === "left" ? -1 : 1);
                      dy =
                        (defaults.height - data.height) *
                        (handle === "top" ? -1 : 1);
                    }
                    const keepRatio =
                      (data.type === CanvasNodeType.Image &&
                        !data.metadata?.freeResize) ||
                      data.type === CanvasNodeType.Video ||
                      Boolean(definition?.keepAspectRatio?.(data));
                    const ratio =
                      (data.metadata?.naturalWidth || data.width) /
                      (data.metadata?.naturalHeight || data.height || 1);
                    const next = resizeCanvasNode(
                      {
                        ...data.position,
                        width: data.width,
                        height: data.height,
                      },
                      handle,
                      dx,
                      dy,
                      keepRatio ? ratio : undefined,
                    );
                    if (
                      next.width === data.width &&
                      next.height === data.height
                    )
                      return;
                    onResizeStart(data.id);
                    onResize(data.id, next.width, next.height, {
                      x: next.x,
                      y: next.y,
                    });
                    onResizeEnd(data.id);
                  }}
                />
              ) : null}
            </div>

            {!referenceSelectionState &&
            !isGroup &&
            (connectionTypes?.input ?? true) ? (
              <ConnectionHandleDot
                scale={scale}
                side="left"
                visible={hovered || isSelected || isConnecting}
                onMouseDown={(event) =>
                  onConnectStart(event, data.id, "target")
                }
              />
            ) : null}
            {!referenceSelectionState &&
            (connectionTypes?.output ??
              ((definition?.hasSourceHandle ?? true) &&
                data.type !== CanvasNodeType.Config)) ? (
              <ConnectionHandleDot
                scale={scale}
                side="right"
                visible={hovered || isSelected || isConnecting}
                onMouseDown={(event) =>
                  onConnectStart(event, data.id, "source")
                }
              />
            ) : null}
          </div>
        </PopoverAnchor>
        {renderPanel && !isGroup && !isMobile ? (
          <PopoverContent
            className="canvas-node-composer"
            style={{
              width: "min(680px, calc(100dvw - 32px))",
            }}
            side="bottom"
            align="center"
            sideOffset={16}
            collisionPadding={{ top: 64, right: 16, bottom: 16, left: 16 }}
            updatePositionStrategy="always"
            onEscapeKeyDown={(event) => {
              if (isImeComposing(event)) event.preventDefault();
              event.stopPropagation();
            }}
            onOpenAutoFocus={(event) => {
              restorePanelFocus.current = true;
              event.preventDefault();
            }}
            onInteractOutside={(event) => {
              if (
                event.target instanceof Element &&
                event.target.closest("[data-canvas-view-control]")
              ) {
                event.preventDefault();
                return;
              }
              restorePanelFocus.current = false;
            }}
            onCloseAutoFocus={(event) => {
              event.preventDefault();
              if (restorePanelFocus.current && selectedForFocus.current)
                nodeElement.current?.focus({ preventScroll: true });
            }}
            data-canvas-no-zoom
          >
            {renderPanel(data)}
          </PopoverContent>
        ) : null}
      </Popover>
      {renderPanel && !isGroup && isMobile ? (
        <Sheet
          open={panelOpen}
          onOpenChange={(open) => {
            if (!open) onPanelClose?.(data.id);
          }}
        >
          <SheetContent
            side="bottom"
            showCloseButton={false}
            className="canvas-node-sheet"
            onEscapeKeyDown={(event) => {
              if (isImeComposing(event)) event.preventDefault();
              event.stopPropagation();
            }}
            onOverlayClick={() => onPanelClose?.(data.id)}
            onPointerDownOutside={(event) => event.preventDefault()}
            onCloseAutoFocus={(event) => {
              event.preventDefault();
              nodeElement.current?.focus({ preventScroll: true });
            }}
            data-canvas-no-zoom
          >
            <SheetTitle className="sr-only">节点参数</SheetTitle>
            <SheetDescription className="sr-only">
              编辑选中节点的提示词、引用和生成参数。
            </SheetDescription>
            {renderPanel(data)}
          </SheetContent>
        </Sheet>
      ) : null}
    </>
  );
});

function NodeContent(props: NodeContentRendererProps) {
  if (props.renderNodeContent) {
    const content = props.renderNodeContent(props.node);
    if (content != null) return content;
  }
  if (props.isBatchRoot && props.node.type === CanvasNodeType.Image)
    return <ImageNodeContent {...props} />;
  if (
    props.node.type === CanvasNodeType.Text &&
    props.node.metadata?.texts?.length &&
    (props.node.metadata.status !== "error" ||
      props.node.metadata.texts.some((text) => text.content))
  )
    return <TextContent {...props} />;
  if (props.node.metadata?.status === "loading")
    return <LoadingContent theme={props.theme} />;
  if (props.node.metadata?.status === "error")
    return (
      <ErrorContent
        node={props.node}
        theme={props.theme}
        onRetry={props.onRetry}
      />
    );

  const Renderer = nodeContentRenderers[props.node.type as CanvasNodeType];
  if (Renderer) return <Renderer {...props} />;

  // Render plugin nodes with their registered renderer, or show the missing-plugin placeholder.
  const definition = getNodeDefinition(props.node.type);
  if (definition?.Content && props.pluginContext) {
    const PluginContent = definition.Content;
    return <PluginContent ctx={props.pluginContext} />;
  }
  return <MissingPluginContent theme={props.theme} type={props.node.type} />;
}

const nodeContentRenderers = {
  [CanvasNodeType.Text]: TextContent,
  [CanvasNodeType.Image]: ImageNodeContent,
  [CanvasNodeType.Config]: EmptyImageContent,
  [CanvasNodeType.Video]: VideoNodeContent,
  [CanvasNodeType.Audio]: AudioNodeContent,
  [CanvasNodeType.Group]: GroupNodeContent,
} satisfies Record<
  CanvasNodeType,
  (props: NodeContentRendererProps) => ReactNode
>;

function GroupNodeContent({
  node,
  theme,
  groupChildCount,
}: NodeContentRendererProps) {
  return (
    <div className="pointer-events-none flex h-full w-full p-3">
      <div
        className="flex h-7 max-w-full items-center gap-2 px-1 text-xs font-medium"
        style={{ color: theme.node.text }}
      >
        <Group
          className="size-3.5 shrink-0"
          style={{ color: theme.node.muted }}
        />
        <span className="truncate">{node.title || "组"}</span>
        <span
          className="shrink-0 text-xs font-normal"
          style={{ color: theme.node.muted }}
        >
          {`${groupChildCount} 个节点`}
        </span>
      </div>
    </div>
  );
}

function LoadingContent({ theme }: Pick<NodeContentRendererProps, "theme">) {
  return (
    <div
      className="flex h-full w-full flex-col items-center justify-center gap-3"
      style={{ color: theme.node.activeStroke }}
    >
      <Spinner className="size-10" />
      <span className="text-xs tracking-[0.2em]">{"生成中"}</span>
    </div>
  );
}

function ErrorContent({
  node,
  onRetry,
}: Pick<NodeContentRendererProps, "node" | "theme" | "onRetry">) {
  return (
    <GenerationResultEmpty
      kind={
        node.type === CanvasNodeType.Text
          ? "text"
          : node.type === CanvasNodeType.Video
            ? "video"
            : node.type === CanvasNodeType.Audio
              ? "audio"
              : "image"
      }
      onRetry={onRetry ? () => onRetry(node) : undefined}
    />
  );
}

function MissingPluginContent({
  theme,
  type,
}: Pick<NodeContentRendererProps, "theme"> & { type: string }) {
  return (
    <div
      className="flex h-full w-full flex-col items-center justify-center gap-2 px-4 text-center"
      style={{ color: theme.node.placeholder }}
    >
      <Puzzle className="size-7 opacity-40" />
      <span className="text-sm">{"缺少插件"}</span>
      <span className="text-xs opacity-70">{`节点类型“${type}”的插件未安装或未启用`}</span>
    </div>
  );
}

function TextContent({
  readOnly,
  acceptContent,
  collaboration,
  node,
  theme,
  isEditingContent,
  textareaRef,
  mentionReferences,
  batchExpanded,
  onContentChange,
  onStopEditing,
  onToggleBatch,
  onSetBatchPrimary,
}: NodeContentRendererProps) {
  const fontSize = node.metadata?.fontSize || 14;
  const textStyle = {
    fontSize: `${fontSize}px`,
    lineHeight: `${Math.round(fontSize * (22 / 14))}px`,
    color: theme.node.text,
    boxSizing: "border-box",
  } as React.CSSProperties;
  const texts = node.metadata?.texts || [];
  const batchCount = texts.length;
  const isBatchRoot = batchCount > 1;
  const primaryTextId = node.metadata?.primaryTextId || texts[0]?.id;
  const primaryText = texts.find((text) => text.id === primaryTextId);
  const content = primaryText?.content || node.metadata?.content || "";
  const paddingClass = isBatchRoot ? "px-6 pb-5 pt-14" : "px-6 py-5";

  const boundReferences = useBoundReferences(node);
  return (
    <BatchFrame batchCount={batchCount} batchExpanded={batchExpanded}>
      {batchExpanded
        ? texts
            .filter((text) => text.id !== primaryTextId)
            .map((text, index) => (
              <ExpandedTextCard
                key={text.id}
                node={node}
                text={text}
                index={index}
                onSetPrimary={() => onSetBatchPrimary?.(text.id)}
              />
            ))
        : null}
      <div className="flex h-full w-full flex-col overflow-hidden rounded-xl">
        {isEditingContent ? (
          <RichTextInput
            acceptDocument={
              acceptContent
                ? (document) =>
                    acceptContent(documentText(document.content), document)
                : undefined
            }
            disabled={readOnly}
            collaboration={collaboration}
            rootRef={textareaRef}
            aria-label="文本节点内容"
            className="h-full w-full"
            style={textStyle}
            value={content}
            document={primaryText?.richText ?? node.metadata?.richText}
            documentMode
            floatingToolbar
            documentTitle={node.title}
            autoFocus
            references={[
              ...mentionReferences.filter((reference) => reference.active),
              ...boundReferences,
            ]}
            onChange={(value, document) =>
              onContentChange(node.id, value, document)
            }
            onEscape={onStopEditing}
          />
        ) : content ? (
          <div
            className={cn(
              "thin-scrollbar block h-full w-full overflow-y-auto bg-transparent font-sans break-words whitespace-pre-wrap",
              paddingClass,
            )}
            data-canvas-wheel-scroll
            style={textStyle}
            onWheel={(event) => event.stopPropagation()}
          >
            <RichTextContent
              value={content}
              document={primaryText?.richText ?? node.metadata?.richText}
              references={[...mentionReferences, ...boundReferences]}
            />
          </div>
        ) : primaryText ? (
          <TextSlotStatus text={primaryText} />
        ) : (
          <EmptyNodeContent type={CanvasNodeType.Text} />
        )}
      </div>
      {isBatchRoot ? (
        <Button
          variant="outline"
          type="button"
          className="absolute top-2.5 right-2.5 z-30 flex h-8 items-center justify-center gap-1.5 border px-3 backdrop-blur-md transition hover:scale-[1.02]"
          aria-label={batchExpanded ? "备选文本已展开" : "备选文本已收起"}
          onClick={(event) => {
            event.stopPropagation();
            onToggleBatch?.();
          }}
          onMouseDown={(event) => event.stopPropagation()}
          onPointerDown={(event) => event.stopPropagation()}
        >
          <span className="leading-none">{`${batchCount} 条`}</span>
          <ChevronRight
            className={cn(
              "size-3.5 opacity-80 transition-transform",
              batchExpanded ? "rotate-90" : "",
            )}
          />
        </Button>
      ) : null}
    </BatchFrame>
  );
}

function ExpandedTextCard({
  node,
  text,
  index,
  onSetPrimary,
}: {
  node: CanvasNodeData;
  text: CanvasNodeText;
  index: number;
  onSetPrimary: () => void;
}) {
  const theme = canvasThemes[useThemeStore((state) => state.theme)];
  const count = node.metadata?.texts?.length || 0;
  const columns = Math.min(count, 4);
  const rows = Math.ceil(count / columns);
  const rootSlot = (rows - 1) * columns;
  const slot = index >= rootSlot ? index + 1 : index;
  const x = (slot % columns) * (node.width + 18);
  const y = (Math.floor(slot / columns) - rows + 1) * (node.height + 18);

  return (
    <div
      className="absolute z-20 overflow-hidden rounded-xl border shadow-[0_18px_50px_var(--canvas-shadow-soft)]"
      style={
        {
          left: x,
          top: y,
          width: node.width,
          height: node.height,
          background: theme.node.panel,
          borderColor: theme.node.stroke,
          "--batch-from-x": `${-x}px`,
          "--batch-from-y": `${-y}px`,
          "--batch-from-rotate": `${4 + index * 2}deg`,
          animation: `canvas-batch-child-in 320ms ${index * 35}ms cubic-bezier(.2,.85,.18,1) both`,
        } as React.CSSProperties
      }
      onMouseDown={(event) => event.stopPropagation()}
      onPointerDown={(event) => event.stopPropagation()}
    >
      {text.content ? (
        <>
          <div
            className="thin-scrollbar h-full overflow-y-auto px-4 pt-14 pb-4 font-sans text-sm leading-6 break-words whitespace-pre-wrap"
            style={{ color: theme.node.text }}
            onWheel={(event) => event.stopPropagation()}
          >
            {text.content}
          </div>
          <Button
            variant="ghost"
            type="button"
            className="pointer-events-none absolute top-2.5 right-2.5 z-30 flex h-8 items-center gap-1.5 px-2.5 opacity-0 transition duration-150 group-hover/node:pointer-events-auto group-hover/node:opacity-100"
            onClick={(event) => (event.stopPropagation(), onSetPrimary())}
          >
            <Star style={{ color: selectionBlue }} />
            {"设为主文本"}
          </Button>
        </>
      ) : (
        <TextSlotStatus text={text} />
      )}
    </div>
  );
}

function TextSlotStatus({ text }: { text: CanvasNodeText }) {
  const theme = canvasThemes[useThemeStore((state) => state.theme)];
  const loading = text.status === "loading";
  if (!loading) return <GenerationResultEmpty kind="text" />;
  return (
    <div
      className="flex h-full w-full flex-col items-center justify-center gap-3 px-6 text-center"
      style={{
        background: theme.node.fill,
        color: theme.node.activeStroke,
      }}
    >
      <Spinner className="size-10" />
      <span className="text-xs tracking-[0.2em]">{"生成中"}</span>
    </div>
  );
}

function ImageNodeContent(props: NodeContentRendererProps) {
  if (!props.node.metadata?.content && !props.isBatchRoot)
    return <EmptyImageContent />;

  return (
    <ImageContent
      node={props.node}
      batchExpanded={props.batchExpanded}
      scale={props.scale}
      onToggleBatch={props.onToggleBatch}
      onSetBatchPrimary={props.onSetBatchPrimary}
      onDuplicateBatchImage={props.onDuplicateBatchImage}
      onDownloadBatchImage={props.onDownloadBatchImage}
      onRetryBatchImage={props.onRetryBatchImage}
      onDeleteBatchImage={props.onDeleteBatchImage}
      onViewBatchImage={props.onViewBatchImage}
    />
  );
}

function EmptyNodeContent({ type }: { type: string }) {
  if (type === CanvasNodeType.Text)
    return (
      <div className="canvas-text-placeholder h-full w-full px-6 py-5">
        双击编辑文本
      </div>
    );
  const Icon =
    type === CanvasNodeType.Video
      ? Play
      : type === CanvasNodeType.Audio
        ? AudioLines
        : type === CanvasNodeType.Text
          ? FileText
          : ImageIcon;
  return (
    <Empty className="canvas-node-placeholder" data-node-type={type}>
      <EmptyHeader>
        <EmptyMedia variant="icon">
          <Icon />
        </EmptyMedia>
        <EmptyDescription
          className={type === CanvasNodeType.Audio ? undefined : "sr-only"}
        >
          {type === CanvasNodeType.Text
            ? "双击输入故事大纲，或选中节点生成分镜脚本"
            : type === CanvasNodeType.Audio
              ? "选中节点，添加已上传的音频素材"
              : type === CanvasNodeType.Image
                ? "输入画面描述，或添加参考图片开始创作"
                : "输入镜头描述，或添加参考素材开始创作"}
        </EmptyDescription>
      </EmptyHeader>
    </Empty>
  );
}

function EmptyImageContent() {
  return <EmptyNodeContent type={CanvasNodeType.Image} />;
}

function VideoNodeContent({ node }: NodeContentRendererProps) {
  if (!node.metadata?.content)
    return <EmptyNodeContent type={CanvasNodeType.Video} />;
  return <CanvasVideoPlayer node={node} />;
}

function AudioNodeContent({ node, theme }: NodeContentRendererProps) {
  if (!node.metadata?.content)
    return <EmptyNodeContent type={CanvasNodeType.Audio} />;
  return (
    <div
      className="flex h-full w-full flex-col justify-center gap-3 px-4"
      style={{ background: theme.node.fill, color: theme.node.text }}
    >
      <div className="flex min-w-0 items-center gap-2 text-sm opacity-70">
        <Music2 className="size-4 shrink-0" />
        <span className="truncate">{"音频"}</span>
      </div>
      <audio
        src={node.metadata.content}
        controls
        className="w-full"
        data-canvas-no-zoom
      />
    </div>
  );
}

function ImageContent({
  node,
  batchExpanded,
  scale,
  onToggleBatch,
  onSetBatchPrimary,
  onDuplicateBatchImage,
  onDownloadBatchImage,
  onRetryBatchImage,
  onDeleteBatchImage,
  onViewBatchImage,
}: {
  node: CanvasNodeData;
  batchExpanded: boolean;
  scale: number;
  onToggleBatch?: () => void;
  onSetBatchPrimary?: (imageId: string) => void;
  onDuplicateBatchImage?: (imageId: string) => void;
  onDownloadBatchImage?: (imageId: string) => void;
  onRetryBatchImage?: (imageId: string) => void;
  onDeleteBatchImage?: (imageId: string) => void;
  onViewBatchImage?: (imageId: string) => void;
}) {
  useSyncExternalStore(
    subscribeImagePreviews,
    getImagePreviewRevision,
    () => 0,
  );
  const images = node.metadata?.images || [];
  const batchCount = images.length;
  const isBatchRoot = batchCount > 1;
  const primaryImageId = node.metadata?.primaryImageId || images[0]?.id;
  const primaryImage = images.find((image) => image.id === primaryImageId);
  const primaryContent = primaryImage?.content || node.metadata?.content;
  const primaryPreviewUrl = previewUrlFor(
    primaryImage?.storageKey || node.metadata?.storageKey,
  );
  const primarySource = primaryContent
    ? pickImageSource({
        previewUrl: primaryPreviewUrl,
        originalUrl: primaryContent,
        naturalWidth: primaryImage?.naturalWidth || node.metadata?.naturalWidth,
        naturalHeight:
          primaryImage?.naturalHeight || node.metadata?.naturalHeight,
        renderedWidth: node.width,
        renderedHeight: node.height,
        scale,
      })
    : undefined;

  return (
    <BatchFrame batchCount={batchCount} batchExpanded={batchExpanded}>
      {batchExpanded
        ? images
            .filter((image) => image.id !== primaryImageId)
            .map((image, index) => (
              <ExpandedImageCard
                key={image.id}
                node={node}
                image={image}
                index={index}
                scale={scale}
                onView={() => onViewBatchImage?.(image.id)}
                onSetPrimary={() => onSetBatchPrimary?.(image.id)}
                onDuplicate={() => onDuplicateBatchImage?.(image.id)}
                onDownload={() => onDownloadBatchImage?.(image.id)}
                onRetry={() => onRetryBatchImage?.(image.id)}
                onDelete={() => onDeleteBatchImage?.(image.id)}
              />
            ))
        : null}
      <div className="h-full w-full overflow-hidden rounded-xl">
        {primaryContent ? (
          <PreviewImage
            src={primarySource}
            alt={node.title}
            draggable={false}
            onDragStart={(event) => event.preventDefault()}
            className="pointer-events-none block h-full w-full object-contain select-none"
          />
        ) : (
          <ImageSlotStatus image={primaryImage} />
        )}
      </div>
      {primaryImage?.status === "error" ? (
        <BatchImageFailureActions
          placement="left"
          onRetry={() => onRetryBatchImage?.(primaryImage.id)}
          onDelete={() => onDeleteBatchImage?.(primaryImage.id)}
        />
      ) : null}
      {primaryImage?.content ? (
        <Button
          variant="outline"
          type="button"
          className="pointer-events-none absolute top-2.5 left-2.5 z-30 flex h-8 items-center gap-1 border px-2 opacity-0 backdrop-blur-md transition group-hover/node:pointer-events-auto group-hover/node:opacity-100 hover:scale-[1.02]"
          title={"下载"}
          onClick={(event) => (
            event.stopPropagation(),
            onDownloadBatchImage?.(primaryImage.id)
          )}
        >
          <Download />
          {"下载"}
        </Button>
      ) : null}
      {isBatchRoot ? (
        <Button
          variant="outline"
          type="button"
          className="absolute top-2.5 right-2.5 z-30 flex h-8 items-center justify-center gap-1.5 border px-3 backdrop-blur-md transition hover:scale-[1.02]"
          aria-label={batchExpanded ? "图片组已展开" : "图片组已收起"}
          onClick={(event) => {
            event.stopPropagation();
            onToggleBatch?.();
          }}
          onMouseDown={(event) => event.stopPropagation()}
          onPointerDown={(event) => event.stopPropagation()}
        >
          <span className="leading-none">{`${batchCount} 张`}</span>
          <ChevronRight
            className={cn(
              "size-3.5 opacity-80 transition-transform",
              batchExpanded ? "rotate-90" : "",
            )}
          />
        </Button>
      ) : null}
    </BatchFrame>
  );
}

function ExpandedImageCard({
  node,
  image,
  index,
  scale,
  onView,
  onSetPrimary,
  onDuplicate,
  onDownload,
  onRetry,
  onDelete,
}: {
  node: CanvasNodeData;
  image: CanvasNodeImage;
  index: number;
  scale: number;
  onView: () => void;
  onSetPrimary: () => void;
  onDuplicate: () => void;
  onDownload: () => void;
  onRetry: () => void;
  onDelete: () => void;
}) {
  const theme = canvasThemes[useThemeStore((state) => state.theme)];
  useSyncExternalStore(
    subscribeImagePreviews,
    getImagePreviewRevision,
    () => 0,
  );
  const count = node.metadata?.images?.length || 0;
  const columns = Math.min(count, 4);
  const rows = Math.ceil(count / columns);
  const rootSlot = (rows - 1) * columns;
  const slot = index >= rootSlot ? index + 1 : index;
  const column = slot % columns;
  const row = Math.floor(slot / columns);
  const x = column * (node.width + 18);
  const y = (row - rows + 1) * (node.height + 18);
  const source = image.content
    ? pickImageSource({
        previewUrl: previewUrlFor(image.storageKey),
        originalUrl: image.content,
        naturalWidth: image.naturalWidth,
        naturalHeight: image.naturalHeight,
        renderedWidth: node.width,
        renderedHeight: node.height,
        scale,
      })
    : undefined;

  return (
    <div
      className={cn(
        "absolute z-20 overflow-hidden rounded-xl",
        image.content
          ? ""
          : "border shadow-[0_18px_50px_var(--canvas-shadow-strong)]",
      )}
      style={
        {
          left: x,
          top: y,
          width: node.width,
          height: node.height,
          background: "transparent",
          borderColor: theme.node.stroke,
          "--batch-from-x": `${-x}px`,
          "--batch-from-y": `${-y}px`,
          "--batch-from-rotate": `${4 + index * 2}deg`,
          animation: `canvas-batch-child-in 320ms ${index * 35}ms cubic-bezier(.2,.85,.18,1) both`,
        } as React.CSSProperties
      }
      onMouseDown={(event) => event.stopPropagation()}
      onPointerDown={(event) => event.stopPropagation()}
      onDoubleClick={(event) => {
        if (
          !image.content ||
          (event.target instanceof Element && event.target.closest("button"))
        )
          return;
        event.stopPropagation();
        onView();
      }}
    >
      {image.content ? (
        <PreviewImage
          src={source}
          alt={node.title}
          draggable={false}
          className="pointer-events-none h-full w-full object-contain select-none"
        />
      ) : (
        <ImageSlotStatus image={image} />
      )}
      {image.content ? (
        <div className="pointer-events-none absolute inset-x-2 top-2 flex items-center gap-1 opacity-0 transition-opacity duration-150 group-hover/node:pointer-events-auto group-hover/node:opacity-100">
          <Button
            variant="outline"
            type="button"
            className="flex h-8 min-w-0 flex-1 items-center justify-center gap-1 border px-1.5 backdrop-blur-md transition hover:scale-[1.02]"
            title={"下载"}
            onClick={(event) => (event.stopPropagation(), onDownload())}
          >
            <Download className="shrink-0" />
            <span className="truncate">{"下载"}</span>
          </Button>
          <Button
            variant="outline"
            type="button"
            className="flex h-8 min-w-0 flex-1 items-center justify-center gap-1 border px-1.5 backdrop-blur-md transition hover:scale-[1.02]"
            title={"创建副本"}
            onClick={(event) => (event.stopPropagation(), onDuplicate())}
          >
            <Copy className="shrink-0" />
            <span className="truncate">{"创建副本"}</span>
          </Button>
          <Button
            variant="outline"
            type="button"
            className="flex h-8 min-w-0 flex-1 items-center justify-center gap-1 border px-1.5 backdrop-blur-md transition hover:scale-[1.02]"
            title={"设为主图"}
            onClick={(event) => (event.stopPropagation(), onSetPrimary())}
          >
            <Star className="shrink-0" style={{ color: selectionBlue }} />
            <span className="truncate">{"设为主图"}</span>
          </Button>
        </div>
      ) : null}
      {image.status === "error" ? (
        <BatchImageFailureActions
          placement="right"
          onRetry={onRetry}
          onDelete={onDelete}
        />
      ) : null}
    </div>
  );
}

function BatchImageFailureActions({
  placement,
  onRetry,
  onDelete,
}: {
  placement: "left" | "right";
  onRetry: () => void;
  onDelete: () => void;
}) {
  return (
    <div
      className={cn(
        "absolute top-3 z-30 flex items-center gap-1.5",
        placement === "left" ? "left-3" : "right-3",
      )}
    >
      <Button
        variant="outline"
        type="button"
        className="flex h-8 items-center gap-1.5 border px-2.5 transition hover:scale-[1.02]"
        onClick={(event) => (event.stopPropagation(), onRetry())}
      >
        <RefreshCw />
        {"重试"}
      </Button>
      <Button
        variant="destructive"
        size="icon-sm"
        type="button"
        className="grid size-8 place-items-center border transition hover:scale-[1.02]"
        onClick={(event) => (event.stopPropagation(), onDelete())}
        aria-label={"删除"}
        title={"删除"}
      >
        <Trash2 />
      </Button>
    </div>
  );
}

function ImageSlotStatus({ image }: { image?: CanvasNodeImage }) {
  const theme = canvasThemes[useThemeStore((state) => state.theme)];
  if (image?.status === "error") return <GenerationResultEmpty kind="image" />;
  return (
    <div
      className="flex h-full w-full flex-col items-center justify-center gap-3 px-6 text-center"
      style={{
        background: theme.node.fill,
        color: theme.node.activeStroke,
      }}
    >
      <Spinner className="size-10" />
      <span className="text-xs tracking-[0.2em]">{"生成中"}</span>
    </div>
  );
}

function ImageInfoBar({ node }: { node: CanvasNodeData }) {
  const width = Math.round(node.metadata?.naturalWidth || node.width);
  const height = Math.round(node.metadata?.naturalHeight || node.height);
  const size = formatBytes(node.metadata?.bytes || 0);
  return (
    <div className="pointer-events-none absolute right-3 bottom-3 z-40 max-w-canvas-dialog">
      <span className="bg-media-backdrop/55 text-media-foreground max-w-full truncate rounded-md px-2 py-1 text-xs leading-none font-medium backdrop-blur-sm">
        {width} x {height}
        {size ? ` · ${size}` : ""}
      </span>
    </div>
  );
}

function BatchFrame({
  batchCount,
  batchExpanded,
  children,
}: {
  batchCount: number;
  batchExpanded: boolean;
  children: ReactNode;
}) {
  const theme = canvasThemes[useThemeStore((state) => state.theme)];
  const isBatchRoot = batchCount > 1;
  return (
    <div className="group/batch relative h-full w-full overflow-visible">
      {isBatchRoot ? (
        <div className="pointer-events-none absolute inset-0 overflow-visible">
          {Array.from({ length: Math.min(batchCount - 1, 3) }).map(
            (_, index) => (
              <div
                key={index}
                className="absolute rounded-[inherit] border shadow-card transition-all duration-300 group-hover/batch:translate-x-1"
                style={{
                  inset: 0,
                  background: `linear-gradient(135deg, ${theme.node.panel}, ${theme.node.fill})`,
                  borderColor: theme.node.stroke,
                  opacity: batchExpanded ? 0 : 1,
                  transform: `translate(${10 + index * 6}px, ${4 + index * 3}px) rotate(${1.5 + index}deg)`,
                  zIndex: -index - 1,
                }}
              />
            ),
          )}
        </div>
      ) : null}
      {children}
    </div>
  );
}
const resizeHandles: {
  handle: ResizeCorner;
  label: string;
  cursor: string;
}[] = [
  {
    handle: "top-left",
    label: "左上角",
    cursor: "nwse-resize",
  },
  {
    handle: "top",
    label: "上边",
    cursor: "ns-resize",
  },
  {
    handle: "top-right",
    label: "右上角",
    cursor: "nesw-resize",
  },
  {
    handle: "right",
    label: "右边",
    cursor: "ew-resize",
  },
  {
    handle: "bottom-right",
    label: "右下角",
    cursor: "nwse-resize",
  },
  {
    handle: "bottom",
    label: "下边",
    cursor: "ns-resize",
  },
  {
    handle: "bottom-left",
    label: "左下角",
    cursor: "nesw-resize",
  },
  {
    handle: "left",
    label: "左边",
    cursor: "ew-resize",
  },
];
function ResizeHandles({
  scale,
  onMouseDown,
  onKeyDown,
}: {
  scale: number;
  onMouseDown: (event: React.MouseEvent, corner: ResizeCorner) => void;
  onKeyDown: (event: React.KeyboardEvent, corner: ResizeCorner) => void;
}) {
  // Hot zones stay in screen pixels while the node body follows canvas zoom.
  const unit = 1 / Math.max(scale, 0.01);
  return resizeHandles.map(({ handle, label, cursor }) => {
    const corner = handle.includes("-");
    const horizontal = handle === "top" || handle === "bottom";
    const position: React.CSSProperties = corner
      ? {
          [handle.includes("left") ? "left" : "right"]: -16 * unit,
          [handle.includes("top") ? "top" : "bottom"]: -16 * unit,
          width: 24 * unit,
          height: 24 * unit,
        }
      : horizontal
        ? {
            left: 12 * unit,
            right: 12 * unit,
            [handle]: -9 * unit,
            width: "auto",
            height: 10 * unit,
          }
        : {
            top: 12 * unit,
            bottom: 12 * unit,
            [handle]: -9 * unit,
            width: 10 * unit,
            height: "auto",
          };
    return (
      <Button
        key={handle}
        variant="ghost"
        size="icon-lg"
        aria-label={`调整节点大小：${label}`}
        title={`拖动或方向键调整${label}${handle.includes("-") ? "" : "；Enter 重置该方向尺寸"}`}
        data-canvas-no-pan
        className="canvas-node-resize absolute z-50"
        style={{ ...position, cursor }}
        onMouseDown={(event) => {
          if (event.button === 0) onMouseDown(event, handle);
        }}
        onPointerDown={(event) => event.stopPropagation()}
        onClick={(event) => event.stopPropagation()}
        onDoubleClick={(event) => event.stopPropagation()}
        onKeyDown={(event) => onKeyDown(event, handle)}
      >
        {handle === "bottom-right" && (
          <svg
            aria-hidden="true"
            viewBox="0 0 16 16"
            className="canvas-node-resize-mark"
            style={{
              width: 16 * unit,
              height: 16 * unit,
              right: 22 * unit,
              bottom: 22 * unit,
            }}
          >
            <path
              d="M4 12 12 4M8 12 12 8"
              fill="none"
              stroke="currentColor"
              strokeWidth="1.2"
              strokeLinecap="round"
            />
          </svg>
        )}
      </Button>
    );
  });
}

function ConnectionHandleDot({
  scale,
  side,
  visible,
  onMouseDown,
}: {
  scale: number;
  side: "left" | "right";
  visible: boolean;
  onMouseDown: (event: React.MouseEvent) => void;
}) {
  const theme = canvasThemes[useThemeStore((state) => state.theme)];
  const unit = 1 / Math.max(scale, 0.01);

  return (
    <div
      data-canvas-no-pan
      className={cn(
        "absolute top-1/2 z-30 flex -translate-y-1/2 cursor-crosshair items-center justify-center transition-opacity duration-150",
        visible
          ? "pointer-events-auto opacity-100"
          : "pointer-events-none opacity-0",
      )}
      style={{ width: 36 * unit, height: 36 * unit, [side]: -38 * unit }}
      onMouseDown={onMouseDown}
    >
      <Button
        variant="outline"
        size="icon-xs"
        aria-label={side === "left" ? "连接输入节点" : "连接输出节点"}
        className="canvas-node-port"
        style={{
          background: "transparent",
          borderColor: theme.node.muted,
          width: 18 * unit,
          height: 18 * unit,
          borderWidth: unit,
        }}
      >
        <Plus
          aria-hidden="true"
          style={{ width: 12 * unit, height: 12 * unit }}
        />
      </Button>
    </div>
  );
}
