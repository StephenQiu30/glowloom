import { CanvasModeMenu } from "./canvas-mode-menu";
import {
  DropdownMenu,
  DropdownMenuTrigger,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuCheckboxItem,
  DropdownMenuSeparator,
} from "@/components/ui/dropdown-menu";
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
import { PanelTop, Link2, Focus, Minus, Plus, Settings2 } from "lucide-react";
import { useState } from "react";

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
  const percent = Math.round(scale * 100);
  return (
    <div
      data-canvas-no-zoom
      className="canvas-viewport-controls absolute bottom-3 left-4 z-50 flex items-center gap-2"
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
        }}
      >
        <PopoverTrigger asChild>
          <Button
            variant="ghost"
            size="sm"
            className="min-w-16 tabular-nums"
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
        </PopoverContent>
      </Popover>
      <div className="canvas-view-settings">
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button variant="ghost" size="icon-lg" aria-label="视图设置">
              <Settings2 />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent
            side="top"
            align="start"
            className="w-60"
            data-canvas-no-zoom
          >
            <DropdownMenuGroup>
              <DropdownMenuItem onSelect={onFit}>
                <Focus />
                适合屏幕
              </DropdownMenuItem>
            </DropdownMenuGroup>
            <DropdownMenuSeparator />
            <DropdownMenuGroup>
              <DropdownMenuCheckboxItem
                checked={isMiniMapOpen}
                onCheckedChange={onToggleMiniMap}
              >
                小地图
              </DropdownMenuCheckboxItem>
              <DropdownMenuCheckboxItem
                checked={showConnections}
                onCheckedChange={onToggleConnections}
              >
                显示连线
              </DropdownMenuCheckboxItem>
              <DropdownMenuCheckboxItem
                checked={snapToGrid}
                onCheckedChange={onToggleSnap}
              >
                网格吸附
              </DropdownMenuCheckboxItem>
            </DropdownMenuGroup>
          </DropdownMenuContent>
        </DropdownMenu>
      </div>
    </div>
  );
}
