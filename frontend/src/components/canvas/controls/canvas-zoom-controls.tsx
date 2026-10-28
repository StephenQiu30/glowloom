import { CanvasModeMenu } from "./canvas-mode-menu";
import { Button } from "@/components/ui/button";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
  PopoverTitle,
} from "@/components/ui/popover";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { PanelTop, Link2, Focus, Minus, Plus, Check } from "lucide-react";
import { useState } from "react";
import { Input } from "@/components/ui/input";
import { CANVAS_MIN_SCALE, CANVAS_MAX_SCALE } from "../lib/canvas-viewport";

export function CanvasZoomControls({
  canvasTool = "select",
  onCanvasToolChange,
  scale,
  onScaleChange,
  onFit,
  onFitSelection,
  isMiniMapOpen,
  onToggleMiniMap,
  showConnections,
  onToggleConnections,
  snapToGrid,
  onToggleSnap,
}: {
  canvasTool?: "select" | "pan";
  onCanvasToolChange?: (tool: "select" | "pan") => void;
  scale: number;
  onScaleChange: (scale: number) => void;
  onFit: () => void;
  onFitSelection?: () => void;
  isMiniMapOpen: boolean;
  onToggleMiniMap: () => void;
  showConnections: boolean;
  onToggleConnections: () => void;
  snapToGrid: boolean;
  onToggleSnap: () => void;
}) {
  const [zoomOpen, setZoomOpen] = useState(false);
  const [draft, setDraft] = useState("");
  const [error, setError] = useState("");
  const percent = Math.round(scale * 100);
  return (
    <div
      data-canvas-no-zoom
      className="canvas-viewport-controls flex min-w-0 items-center gap-1"
      aria-label="画布视图控制"
    >
      {onCanvasToolChange && (
        <CanvasModeMenu value={canvasTool} onChange={onCanvasToolChange} />
      )}
      <div className="canvas-view-shortcuts flex items-center gap-1">
        {[
          {
            label: "小地图",
            icon: PanelTop,
            onClick: onToggleMiniMap,
            pressed: isMiniMapOpen,
          },
          {
            label: "显示连线",
            icon: Link2,
            onClick: onToggleConnections,
            pressed: showConnections,
          },
        ].map(({ label, icon: Icon, onClick, pressed }) => (
          <Tooltip key={label}>
            <TooltipTrigger asChild>
              <Button
                variant={pressed ? "secondary" : "ghost"}
                size="icon-lg"
                onClick={onClick}
                aria-label={label}
                data-canvas-view-control={label === "适合屏幕" ? "" : undefined}
                aria-pressed={pressed}
              >
                <Icon />
              </Button>
            </TooltipTrigger>
            <TooltipContent side="top">{label}</TooltipContent>
          </Tooltip>
        ))}
      </div>
      <Popover
        open={zoomOpen}
        onOpenChange={(open) => {
          setZoomOpen(open);
          if (open) {
            setDraft(String(percent));
            setError("");
          }
        }}
      >
        <PopoverTrigger asChild>
          <Button
            variant="ghost"
            size="default"
            className="min-w-16"
            aria-label={`缩放选项，当前 ${percent}%`}
            data-canvas-view-control
          >
            {percent}%
          </Button>
        </PopoverTrigger>
        <PopoverContent
          side="top"
          align="start"
          className="canvas-zoom-menu"
          data-canvas-view-control
          data-canvas-no-zoom
        >
          <PopoverTitle className="sr-only">缩放</PopoverTitle>
          <form
            className="mb-2"
            onSubmit={(event) => {
              event.preventDefault();
              const input = draft.trim().replace(/%$/, "").trim();
              const value = Number(input);
              if (!input || !Number.isFinite(value) || value <= 0) {
                setError("请输入有效的缩放比例");
                return;
              }
              onScaleChange(
                Math.min(
                  CANVAS_MAX_SCALE,
                  Math.max(CANVAS_MIN_SCALE, value / 100),
                ),
              );
              setZoomOpen(false);
            }}
          >
            <Input
              aria-label="缩放比例（百分比）"
              inputMode="decimal"
              value={draft}
              aria-invalid={!!error}
              onChange={(event) => {
                setDraft(event.target.value);
                setError("");
              }}
              onKeyDown={(event) => {
                if (event.nativeEvent.isComposing && event.key === "Enter")
                  event.preventDefault();
              }}
            />
            <p
              className="mt-1 text-xs text-muted-foreground"
              role={error ? "alert" : undefined}
            >
              {error || "5%–500%，按 Enter 应用"}
            </p>
          </form>
          {[
            {
              label: "放大视图",
              icon: Plus,
              action: () => onScaleChange(scale * 1.1),
            },
            {
              label: "缩小视图",
              icon: Minus,
              action: () => onScaleChange(scale / 1.1),
            },
            { label: "适应画布", icon: Focus, action: onFit },
            { label: "缩放至选中项", icon: Focus, action: onFitSelection },
            ...[0.5, 1, 2].map((value) => ({
              label: `缩放至${value * 100}%`,
              icon: Focus,
              action: () => onScaleChange(value),
            })),
          ].map(({ label, action }) => (
            <Button
              key={label}
              variant="ghost"
              className="w-full justify-start"
              disabled={!action}
              onClick={() => {
                action?.();
                setZoomOpen(false);
              }}
            >
              {label}
            </Button>
          ))}
          <Button
            variant="ghost"
            className="w-full justify-between"
            aria-pressed={snapToGrid}
            onClick={onToggleSnap}
          >
            网格吸附{snapToGrid && <Check aria-hidden="true" />}
          </Button>
        </PopoverContent>
      </Popover>
    </div>
  );
}
