"use client";

import { useLayoutEffect, useRef, type RefObject, type ReactNode } from "react";
import { Copy, Group, Link2, Unlink, Trash2, Ungroup } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import {
  CANVAS_NODE_DRAG_EVENT,
  CANVAS_VIEWPORT_EVENT,
} from "../viewport/transform";
import type { Node } from "../document";

export function CanvasSelectionToolbar({
  nodes,
  containerRef,
  canEdit,
  canRemove,
  onGroup,
  onUngroup,
  onCopy,
  onRemove,
  onConnect,
  onDisconnect,
  children,
  rightInset = 0,
}: {
  rightInset?: number;
  children?: ReactNode;
  nodes: Node[];
  containerRef: RefObject<HTMLDivElement | null>;
  canEdit: boolean;
  canRemove: boolean;
  onGroup: () => void;
  onUngroup: () => void;
  onCopy: () => void;
  onRemove: () => void;
  onConnect: () => void;
  onDisconnect: () => void;
}) {
  const ref = useRef<HTMLDivElement>(null);
  useLayoutEffect(() => {
    const container = containerRef.current,
      toolbar = ref.current;
    if (!container || !toolbar) return;
    const position = () => {
      const boxes = nodes.flatMap((node) => {
        const element = container.querySelector<HTMLElement>(
          `[data-node-id="${node.id}"] .canvas-node-shell`,
        );
        return element ? [element.getBoundingClientRect()] : [];
      });
      if (!boxes.length) return;
      const bounds = container.getBoundingClientRect();
      const center =
        (Math.min(...boxes.map((box) => box.left)) +
          Math.max(...boxes.map((box) => box.right))) /
          2 -
        bounds.left;
      const top = Math.min(...boxes.map((box) => box.top)) - bounds.top;
      const available = Math.max(80, container.clientWidth - rightInset);
      toolbar.style.maxWidth = `${Math.max(56, available - 24)}px`;
      toolbar.style.left = `${Math.max(12, Math.min(available - toolbar.offsetWidth - 12, center - toolbar.offsetWidth / 2))}px`;
      toolbar.style.top = `${Math.max(64, Math.min(container.clientHeight - toolbar.offsetHeight - 70, top - toolbar.offsetHeight - 36))}px`;
    };
    position();
    const observer = new ResizeObserver(position);
    observer.observe(container);
    observer.observe(toolbar);
    container.addEventListener(CANVAS_VIEWPORT_EVENT, position);
    container.addEventListener(CANVAS_NODE_DRAG_EVENT, position);
    return () => {
      observer.disconnect();
      container.removeEventListener(CANVAS_VIEWPORT_EVENT, position);
      container.removeEventListener(CANVAS_NODE_DRAG_EVENT, position);
    };
  }, [containerRef, nodes, rightInset]);
  const actions = [
    {
      label: "组合所选节点",
      Icon: Group,
      run: onGroup,
      disabled: !canEdit || nodes.length < 2,
    },
    {
      label: "解除所选分组",
      Icon: Ungroup,
      run: onUngroup,
      disabled:
        !canEdit ||
        !nodes.some((node) => node.parent_id || node.node_type === "group"),
    },
    { label: "复制所选节点", Icon: Copy, run: onCopy, disabled: !canEdit },
    {
      label: "连接所选节点",
      Icon: Link2,
      run: onConnect,
      disabled: !canEdit || nodes.length !== 2,
    },
    {
      label: "移除所选标注",
      Icon: Unlink,
      run: onDisconnect,
      disabled: !canEdit,
    },
    {
      label: "移除所选节点",
      Icon: Trash2,
      run: onRemove,
      disabled: !canRemove,
    },
  ];
  return (
    <TooltipProvider>
      <div
        ref={ref}
        className="canvas-selection-toolbar"
        role="toolbar"
        aria-label="所选节点操作"
        data-canvas-no-zoom
      >
        {children ?? (
          <>
            <span className="px-2 text-xs whitespace-nowrap text-muted-foreground">
              已选 {nodes.length} 项
            </span>
            {actions.map(({ label, Icon, run, disabled }) => (
              <Tooltip key={label}>
                <TooltipTrigger asChild>
                  <Button
                    size="icon-sm"
                    variant="ghost"
                    aria-label={label}
                    disabled={disabled}
                    onClick={run}
                  >
                    <Icon data-icon="inline-start" />
                  </Button>
                </TooltipTrigger>
                <TooltipContent>{label}</TooltipContent>
              </Tooltip>
            ))}
          </>
        )}
      </div>
    </TooltipProvider>
  );
}
