import { PreviewImage } from "@/components/canvas/runtime/components/media/preview-image";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Empty, EmptyDescription } from "@/components/ui/empty";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import {
  FileText,
  Image as ImageIcon,
  Music2,
  Plus,
  Puzzle,
  Video,
  X,
} from "lucide-react";
import { useSyncExternalStore, type ReactNode } from "react";
import { useCanvasReferencePreviews } from "../hooks/use-canvas-reference-previews";
import type { CanvasResourceReference } from "../lib/canvas-resource-references";

import {
  buildCanvasResourceReferences,
  getGroupResourceNodes,
} from "@/components/canvas/lib/canvas-resource-references";
import { getNodeDefinition } from "@/components/canvas/lib/node-registry";
import {
  getImagePreviewRevision,
  previewUrlFor,
  subscribeImagePreviews,
} from "@/components/canvas/runtime/lib/media/image-storage";
import {
  CanvasNodeType,
  type CanvasNodeData,
} from "@/components/canvas/types/canvas";

export function CanvasNodeReferenceBar({
  nodeId,
  nodes,
  connectedNodes,
  onDisconnect,
  onStartSelection,
  tools,
}: {
  nodeId: string;
  nodes: CanvasNodeData[];
  connectedNodes: CanvasNodeData[];
  onDisconnect?: (fromNodeId: string, toNodeId: string) => void;
  onStartSelection?: (nodeId: string) => void;
  tools?: ReactNode;
}) {
  const references = connectedNodes.flatMap((sourceNode) =>
    (sourceNode.type === CanvasNodeType.Group
      ? getGroupResourceNodes(sourceNode.id, nodes)
      : [sourceNode]
    ).map((node) => ({ node, sourceNodeId: sourceNode.id })),
  );
  const previews = useCanvasReferencePreviews(
    buildCanvasResourceReferences(references.map(({ node }) => node)),
  );
  const byNode = new Map(
    previews.references.map((reference) => [reference.nodeId, reference]),
  );
  return (
    <div className="canvas-reference-bar">
      <div className="flex min-w-0 flex-wrap items-center gap-2">
        <Button
          variant="secondary"
          size="xs"
          className="canvas-reference-add"
          type="button"
          aria-label="添加参考"
          title="从画布选择参考节点"
          disabled={!onStartSelection}
          onClick={() => onStartSelection?.(nodeId)}
        >
          <Plus aria-hidden="true" />
          <span className="sr-only">参考</span>
        </Button>
        {tools}
      </div>
      {references.length > 0 ? (
        <div
          className="canvas-reference-list"
          role="list"
          aria-label="已连接参考素材"
        >
          {references.map(({ node, sourceNodeId }, index) => (
            <ReferenceItem
              key={`${sourceNodeId}:${node.id}`}
              node={node}
              reference={byNode.get(node.id)}
              label={byNode.get(node.id)?.label ?? `参考${index + 1}`}
              onPreview={previews.refresh}
              onRemove={
                onDisconnect
                  ? () => onDisconnect(sourceNodeId, nodeId)
                  : undefined
              }
            />
          ))}
        </div>
      ) : null}
    </div>
  );
}

function ReferenceItem({
  node,
  label,
  reference,
  onPreview,
  onRemove,
}: {
  node: CanvasNodeData;
  label: string;
  reference?: CanvasResourceReference;
  onPreview: () => void;
  onRemove?: () => void;
}) {
  useSyncExternalStore(
    subscribeImagePreviews,
    getImagePreviewRevision,
    () => 0,
  );
  const resource = getNodeDefinition(node.type)?.resource?.(node);
  const content = reference?.previewUrl;
  const kind = reference?.kind ?? resource?.kind;
  const thumbnail =
    node.referenceProjection || reference?.file
      ? content
      : previewUrlFor(node.metadata?.storageKey) || content;
  const Icon =
    kind === "image"
      ? ImageIcon
      : kind === "video"
        ? Video
        : kind === "audio"
          ? Music2
          : kind === "text"
            ? FileText
            : Puzzle;
  return (
    <div className="canvas-reference-item" role="listitem">
      <Popover
        onOpenChange={(open) => {
          if (open && reference?.file) onPreview();
        }}
      >
        <Tooltip>
          <TooltipTrigger asChild>
            <PopoverTrigger asChild>
              <Button
                variant="secondary"
                aria-label={`预览参考 ${node.title}`}
                className="canvas-reference-thumbnail relative size-14 overflow-hidden p-0"
              >
                <span className="canvas-reference-preview grid size-full place-items-center overflow-hidden rounded-lg">
                  {kind === "image" && thumbnail ? (
                    <PreviewImage
                      src={thumbnail}
                      alt=""
                      className="size-full object-cover"
                    />
                  ) : kind === "video" && content ? (
                    <video
                      src={content}
                      className="size-full object-cover"
                      muted
                    />
                  ) : (
                    <Icon aria-hidden="true" />
                  )}
                </span>
                <Badge
                  variant="secondary"
                  className="pointer-events-none absolute top-0.5 left-0.5 size-4 p-0"
                >
                  {label.match(/\d+$/)?.[0]}
                </Badge>
              </Button>
            </PopoverTrigger>
          </TooltipTrigger>
          <TooltipContent>
            {label} · {node.title}
          </TooltipContent>
        </Tooltip>
        <PopoverContent side="top" align="start">
          {<ReferencePreview node={node} kind={kind} content={content} />}
        </PopoverContent>
      </Popover>
      <Button
        variant="secondary"
        size="icon-xs"
        className="canvas-reference-remove"
        type="button"
        disabled={!onRemove}
        aria-label={`移除参考 ${node.title}`}
        title={`移除参考 ${node.title}`}
        onMouseDown={(event) => event.stopPropagation()}
        onClick={(event) => {
          event.stopPropagation();
          onRemove?.();
        }}
      >
        <X />
      </Button>
    </div>
  );
}

function ReferencePreview({
  node,
  content,
  kind,
}: {
  node: CanvasNodeData;
  content?: string;
  kind?: string;
}) {
  const resource = getNodeDefinition(node.type)?.resource?.(node);
  if (kind === "image" && content)
    return (
      <PreviewImage
        src={content}
        alt={node.title}
        className="max-h-media-preview w-full rounded-lg object-contain"
      />
    );
  if (kind === "video" && content)
    return (
      <video
        src={content}
        className="max-h-media-preview w-full rounded-lg"
        muted
        controls
      />
    );
  if (kind === "audio" && content)
    return <audio src={content} className="w-full" controls />;
  if (kind && kind !== "text")
    return (
      <Empty>
        <EmptyDescription>暂无可用预览</EmptyDescription>
      </Empty>
    );
  return (
    <div className="max-h-media-preview w-full overflow-auto text-sm whitespace-pre-wrap">
      {resource?.text ||
        node.metadata?.content ||
        node.metadata?.prompt ||
        node.title ||
        "暂无内容"}
    </div>
  );
}
