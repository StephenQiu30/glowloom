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
import type { ReactNode } from "react";

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
  onCopy?: () => void;
  onDelete?: () => void;
}) {
  const theme = canvasThemes[useThemeStore((state) => state.theme)];
  if (!nodes.length || (nodes.length < 2 && !onRun)) return null;

  const bounds = nodeBounds(nodes);
  const left = viewport.x + bounds.left * viewport.k - SELECTION_PAD;
  const top = viewport.y + bounds.top * viewport.k - SELECTION_PAD;
  const width = (bounds.right - bounds.left) * viewport.k + SELECTION_PAD * 2;
  const height = (bounds.bottom - bounds.top) * viewport.k + SELECTION_PAD * 2;
  const showActions = showToolbar && !readOnly;

  return (
    <>
      <svg
        data-canvas-drag-selection
        className="pointer-events-none absolute z-[65] overflow-visible"
        style={{ left, top, width, height }}
      >
        <rect
          x={1}
          y={1}
          width={Math.max(width - 2, 0)}
          height={Math.max(height - 2, 0)}
          rx={16}
          ry={16}
          fill={theme.canvas.selectionFill}
          stroke={theme.canvas.selectionStroke}
          strokeOpacity={0.55}
          strokeWidth={1.5}
          strokeDasharray="7 5"
          strokeLinecap="round"
        />
      </svg>
      {showActions ? (
        <div
          className="shadow-overlay absolute z-[70] flex min-h-12 max-w-[calc(100%-1.5rem)] flex-wrap justify-center rounded-xl border text-sm"
          style={{
            left: "50%",
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
