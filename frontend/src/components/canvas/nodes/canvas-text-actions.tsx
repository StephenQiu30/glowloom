"use client";

import { useRef, useState } from "react";
import { Download, Maximize2 } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { RichTextInput } from "../runtime/components/editor/rich-text/rich-text-input";
import {
  textToDocument,
  documentText,
} from "../runtime/components/editor/lib/rich-text-document";
import { canvasTextBackgrounds } from "../lib/canvas-text-background";
import type {
  CanvasNodeData,
  CanvasNodeMetadata,
  ViewportTransform,
} from "../types/canvas";
import type { RichTextDocument } from "../runtime/components/editor/types/rich-text";
import type { NodeTextBinding } from "@glowloom/canvas-collab/model";
import { useBoundReferences } from "../hooks/use-bound-references";

export function CanvasTextActions({
  node,
  viewport,
  canvasSize,
  readOnly,
  collaboration,
  onBackgroundChange,
  onContentChange,
  acceptContent,
  onKeep,
  onLeave,
}: {
  node: CanvasNodeData;
  viewport: ViewportTransform;
  canvasSize: { width: number; height: number };
  readOnly?: boolean;
  collaboration?: NodeTextBinding;
  onBackgroundChange: (
    color: NonNullable<CanvasNodeMetadata["textBackground"]>,
  ) => void;
  onContentChange: (value: string, document: RichTextDocument) => void;
  acceptContent: (value: string, document: RichTextDocument) => boolean;
  onKeep: () => void;
  onLeave: () => void;
}) {
  const [expanded, setExpanded] = useState(false);
  const [paletteOpen, setPaletteOpen] = useState(false);
  const expandRef = useRef<HTMLButtonElement>(null);
  const references = useBoundReferences(node);
  const color =
    canvasTextBackgrounds.find(
      (item) => item.value === node.metadata?.textBackground,
    ) ?? canvasTextBackgrounds[0];
  const document = node.metadata?.richText ?? {
    schemaVersion: 1 as const,
    content: textToDocument(node.metadata?.content ?? ""),
  };
  const left = Math.max(
    88,
    Math.min(
      viewport.x + (node.position.x + node.width / 2) * viewport.k,
      canvasSize.width - 88,
    ),
  );
  // Screen-space controls stay the same size at every world-space zoom.
  const top = Math.max(
    64,
    Math.min(
      viewport.y + node.position.y * viewport.k - 76,
      canvasSize.height - 48,
    ),
  );

  return (
    <>
      <div
        role="toolbar"
        aria-label="文本选中操作"
        className="canvas-text-actions"
        style={{ left, top }}
        data-canvas-no-zoom
        data-canvas-text-action
        onPointerDown={(event) => event.stopPropagation()}
        onMouseDown={(event) => event.stopPropagation()}
        onMouseEnter={onKeep}
        onMouseLeave={() => {
          if (!expanded && !paletteOpen) onLeave();
        }}
      >
        <DropdownMenu
          open={paletteOpen}
          onOpenChange={(open) => {
            setPaletteOpen(open);
            if (open) onKeep();
          }}
        >
          <DropdownMenuTrigger asChild>
            <Button
              variant="ghost"
              className="canvas-text-background-trigger"
              aria-label="背景色"
              disabled={readOnly}
            >
              <span
                aria-hidden="true"
                className="canvas-text-background-chip"
                style={{ background: color.color }}
              />
              背景色
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent
            aria-label="背景色"
            align="center"
            className="canvas-text-color-menu z-[120] h-10 w-[214px] min-w-[214px] overflow-visible rounded-xl bg-(--canvas-text-menu) px-2 py-1.5 text-(--canvas-reference-foreground)"
            data-canvas-no-zoom
            data-canvas-text-action
            onPointerDown={(event) => event.stopPropagation()}
          >
            <DropdownMenuRadioGroup
              value={color.value}
              className="canvas-text-color-options"
              onValueChange={(value) => {
                const choice = canvasTextBackgrounds.find(
                  (item) => item.value === value,
                );
                if (choice && !readOnly) onBackgroundChange(choice.value);
              }}
            >
              {canvasTextBackgrounds.map((choice) => (
                <DropdownMenuRadioItem
                  key={choice.value}
                  value={choice.value}
                  aria-label={choice.label}
                  className="canvas-text-color-option size-7 min-w-7 gap-0 p-[5px] focus:bg-transparent"
                >
                  <span
                    className="canvas-text-color-swatch"
                    aria-hidden="true"
                    style={{ background: choice.color }}
                  />
                </DropdownMenuRadioItem>
              ))}
            </DropdownMenuRadioGroup>
          </DropdownMenuContent>
        </DropdownMenu>
        <span className="canvas-text-action-divider" aria-hidden="true" />
        <Button
          variant="ghost"
          size="icon"
          ref={expandRef}
          aria-label="全屏"
          onClick={() => {
            onKeep();
            setExpanded(true);
          }}
        >
          <Maximize2 />
        </Button>
        <Button
          variant="ghost"
          size="icon"
          aria-label="下载文本"
          onClick={() => {
            void import("../runtime/components/editor/lib/rich-text-download")
              .then(({ downloadRichTextMarkdown }) =>
                downloadRichTextMarkdown(document, node.title, references),
              )
              .catch(() => toast.error("文本下载失败，请重试"));
          }}
        >
          <Download />
        </Button>
      </div>
      {expanded && (
        <div className="hidden">
          <RichTextInput
            collaboration={collaboration}
            documentMode
            documentExport="markdown"
            documentTitle={node.title}
            value={node.metadata?.content ?? ""}
            document={document}
            references={references}
            disabled={readOnly}
            aria-label="文本节点内容"
            onChange={onContentChange}
            acceptDocument={(next) =>
              !readOnly && acceptContent(documentText(next.content), next)
            }
            expandedView={{
              open: expanded,
              onOpenChange: setExpanded,
              onCloseAutoFocus: () =>
                expandRef.current?.focus({ preventScroll: true }),
            }}
          />
        </div>
      )}
    </>
  );
}
