import { Button } from "@/components/ui/button";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import {
  Group,
  Ungroup,
  Play,
  Grid2X2,
  LayoutDashboard,
  Copy,
  Trash2,
} from "lucide-react";
import type { MouseEvent, ReactNode } from "react";
import { CanvasConnectionHandle } from "../nodes/canvas-connection-handle";

import { canvasThemes } from "@/components/canvas/runtime/lib/theme";
import { nodeBounds } from "@/components/canvas/lib/canvas-node-geometry";
import { useThemeStore } from "@/components/canvas/runtime/store/use-theme-store";
import type {
  CanvasNodeData,
  ViewportTransform,
} from "@/components/canvas/types/canvas";

const SELECTION_PAD = 14;

export function CanvasSelectionToolbar({
  nodes,
  viewport,
  showToolbar,
  canGroup,
  canUngroup,
  onGroup,
  onUngroup,
  onRun,
  runDisabled = false,
  readOnly = false,
  onLayout,
  onConnectAfter,
  onConnectStart,
  onCopy,
  onDelete,
}: {
  nodes: CanvasNodeData[];
  viewport: ViewportTransform;
  showToolbar: boolean;
  canGroup: boolean;
  canUngroup: boolean;
  onGroup: () => void;
  onUngroup: () => void;
  onRun?: () => void;
  runDisabled?: boolean;
  readOnly?: boolean;
  onLayout?: (mode: "grid" | "smart") => void;
  onConnectAfter?: () => void;
  onConnectStart?: (event: MouseEvent) => void;
  onCopy?: () => void;
  onDelete?: () => void;
}) {
  const theme = canvasThemes[useThemeStore((state) => state.theme)];
  if (!nodes.length || (nodes.length < 2 && !onRun)) return null;

  const bounds = nodeBounds(nodes);
  const left = viewport.x + bounds.left * viewport.k - SELECTION_PAD;
  const top = viewport.y + bounds.top * viewport.k - SELECTION_PAD;
  const width = (bounds.right - bounds.left) * viewport.k + SELECTION_PAD * 2;
  const showActions = showToolbar && !readOnly;

  return (
    <>
      <div
        data-canvas-drag-selection
        aria-hidden="true"
        className="pointer-events-none absolute z-[65]"
        style={{
          left: viewport.x + bounds.left * viewport.k,
          top: viewport.y + bounds.top * viewport.k,
          width: 0,
          height: 0,
        }}
      >
        <div
          data-canvas-selection-frame
          style={{
            position: "absolute",
            left: -40 * viewport.k,
            top: -40 * viewport.k,
            width: bounds.right - bounds.left,
            height: bounds.bottom - bounds.top,
            boxSizing: "content-box",
            padding: 40,
            borderRadius: 40,
            borderStyle: "dashed",
            borderWidth: 1 / viewport.k,
            borderColor:
              "color-mix(in srgb, var(--canvas-reference-foreground) 20%, transparent)",
            background:
              "color-mix(in srgb, var(--canvas-reference-foreground) 3.9215686274509802%, transparent)",
            transform: `scale(${viewport.k})`,
            transformOrigin: "0 0",
          }}
        />
      </div>
      {showActions ? (
        <div
          role="toolbar"
          aria-label="多选节点操作"
          className="shadow-overlay absolute z-[70] flex min-h-12 max-w-[calc(100%-1.5rem)] flex-wrap justify-center rounded-xl border text-sm"
          style={{
            left: left + width / 2,
            transform: "translateX(-50%)",
            top: Math.max(116, top - 60),
            background: theme.toolbar.panel,
            borderColor: theme.toolbar.border,
            color: theme.node.text,
          }}
          onMouseDown={(event) => event.stopPropagation()}
          onPointerDown={(event) => event.stopPropagation()}
        >
          {canGroup ? (
            <SelectionAction
              title={"用组节点包住选中节点"}
              label={"编组"}
              icon={<Group className="size-4" />}
              onClick={onGroup}
            />
          ) : null}
          {onLayout && (
            <>
              <SelectionAction
                title="宫格布局"
                label="宫格"
                icon={<Grid2X2 className="size-4" />}
                onClick={() => onLayout("grid")}
              />
              <SelectionAction
                title="智能布局"
                label="智能布局"
                icon={<LayoutDashboard className="size-4" />}
                onClick={() => onLayout("smart")}
              />
            </>
          )}
          {onCopy && (
            <SelectionAction
              title="复制选中节点"
              label="复制"
              icon={<Copy className="size-4" />}
              onClick={onCopy}
            />
          )}
          {onDelete && (
            <SelectionAction
              title="删除选中节点"
              label="删除"
              icon={<Trash2 className="size-4" />}
              onClick={onDelete}
            />
          )}
          {onRun && (
            <SelectionAction
              title="运行选中节点或分组"
              label="运行流程"
              icon={<Play className="size-4" />}
              onClick={onRun}
              disabled={runDisabled}
            />
          )}
          {canUngroup ? (
            <SelectionAction
              title={"取消节点分组"}
              label={"解散组"}
              icon={<Ungroup className="size-4" />}
              onClick={onUngroup}
            />
          ) : null}
        </div>
      ) : null}
      {showActions && (onConnectStart || onConnectAfter) && (
        <div
          data-canvas-selection-connection
          className="absolute z-[70]"
          style={{
            left: viewport.x + bounds.right * viewport.k + 40 * viewport.k + 16,
            top: viewport.y + ((bounds.top + bounds.bottom) / 2) * viewport.k,
            width: 0,
            height: 0,
          }}
        >
          <CanvasConnectionHandle
            variant="aggregate"
            scale={viewport.k}
            side="right"
            visible
            onMouseDown={(event) => {
              event.stopPropagation();
              if (onConnectStart) onConnectStart(event);
              else onConnectAfter?.();
            }}
          />
        </div>
      )}
    </>
  );
}

function SelectionAction({
  title,
  label,
  icon,
  onClick,
  disabled,
}: {
  title: string;
  label: string;
  icon: ReactNode;
  onClick: () => void;
  disabled?: boolean;
}) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          variant="ghost"
          type="button"
          className="group relative flex h-12 items-center px-1.5 whitespace-nowrap"
          onClick={() => onClick()}
          disabled={disabled}
          aria-label={title}
        >
          <span className="group-hover:bg-media-backdrop/5 dark:group-hover:bg-media-foreground/10 flex h-9 items-center gap-2 rounded-lg px-2.5 transition">
            {icon}
            <span>{label}</span>
          </span>
        </Button>
      </TooltipTrigger>
      <TooltipContent side="top">{title}</TooltipContent>
    </Tooltip>
  );
}
