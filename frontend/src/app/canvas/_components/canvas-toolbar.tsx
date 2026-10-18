"use client";
import { useState, type ComponentProps } from "react";
import {
  Tooltip,
  TooltipTrigger,
  TooltipContent,
  TooltipProvider,
} from "@/components/ui/tooltip";
import { Separator } from "@/components/ui/separator";
import {
  Type,
  ImageIcon,
  VideoIcon,
  AudioLines,
  Contact,
  Box,
  Folder,
  Upload,
  Hand,
  MousePointer2,
  Plus,
  Minus,
  Maximize,
  Map,
  Link2,
  Sparkles,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogTitle,
  DialogDescription,
} from "@/components/ui/dialog";
import { Kbd } from "@/components/ui/kbd";
import { Input } from "@/components/ui/input";
import { Field, FieldLabel } from "@/components/ui/field";
import {
  DropdownMenu,
  DropdownMenuTrigger,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuSeparator,
} from "@/components/ui/dropdown-menu";
function ToolButton({ title, ...props }: ComponentProps<typeof Button>) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button {...props} />
      </TooltipTrigger>
      <TooltipContent>{title ?? props["aria-label"]}</TooltipContent>
    </Tooltip>
  );
}
export function CanvasToolbar({
  tool,
  onToolChange,
  onAddTool,
  onAI,
  onAdd,
  onAddText,
  onMaterials,
  onUpload,
  zoom,
  onZoom,
  onFit,
  miniMap,
  onMiniMap,
  shortcutsOpen,
  onShortcutsOpenChange,
  hasSelection,
  showEdges,
  onShowEdges,
  onFitSelection,
}: {
  tool: "select" | "pan";
  onAddTool: (kind: "director") => void;
  onAI: () => void;
  onToolChange: (tool: "select" | "pan") => void;
  onAddText: () => void;
  onAdd: (kind: "image_generation" | "video_generation") => void;
  onMaterials: (tab?: "assets" | "subjects", kind?: string) => void;
  onUpload: () => void;
  zoom: number;
  onZoom: (zoom: number) => void;
  onFit: () => void;
  miniMap: boolean;
  onMiniMap: () => void;
  shortcutsOpen: boolean;
  onShortcutsOpenChange: (open: boolean) => void;
  hasSelection: boolean;
  showEdges: boolean;
  onShowEdges: () => void;
  onFitSelection: () => void;
}) {
  const [percentage, setPercentage] = useState("");
  return (
    <TooltipProvider>
      <div
        className="canvas-main-dock"
        role="toolbar"
        aria-label="画布工具"
        data-canvas-no-zoom
      >
        <ToolButton
          variant="ghost"
          size="icon"
          aria-label="添加文本"
          title="文本"
          onClick={onAddText}
        >
          <Type data-icon="inline-start" />
        </ToolButton>
        <ToolButton
          variant="ghost"
          size="icon"
          aria-label="新增图片节点"
          title="图片"
          onClick={() => onAdd("image_generation")}
        >
          <ImageIcon data-icon="inline-start" />
        </ToolButton>
        <ToolButton
          variant="ghost"
          size="icon"
          aria-label="新增视频节点"
          title="视频"
          onClick={() => onAdd("video_generation")}
        >
          <VideoIcon data-icon="inline-start" />
        </ToolButton>
        <ToolButton
          variant="ghost"
          size="icon"
          aria-label="音频"
          title="添加音频素材"
          onClick={() => onMaterials("assets", "audio")}
        >
          <AudioLines data-icon="inline-start" />
        </ToolButton>
        <ToolButton
          variant="ghost"
          size="icon"
          aria-label="主体"
          title="主体"
          onClick={() => onMaterials("subjects")}
        >
          <Contact data-icon="inline-start" />
        </ToolButton>
        <ToolButton
          variant="ghost"
          size="icon"
          aria-label="导演台"
          title="导演台"
          onClick={() => onAddTool("director")}
        >
          <Box data-icon="inline-start" />
        </ToolButton>
        <Separator className="canvas-dock-divider" />
        <ToolButton
          variant="ghost"
          size="icon"
          aria-label="素材库"
          title="素材库"
          onClick={() => onMaterials("assets")}
        >
          <Folder data-icon="inline-start" />
        </ToolButton>
        <ToolButton
          variant="ghost"
          size="icon"
          aria-label="上传素材"
          title="上传素材"
          onClick={onUpload}
        >
          <Upload data-icon="inline-start" />
        </ToolButton>
      </div>
      <div
        className="canvas-zoom-controls"
        role="toolbar"
        aria-label="画布视口"
        data-canvas-no-zoom
      >
        <ToolButton
          variant="ghost"
          size="icon-sm"
          aria-label={tool === "select" ? "选择工具" : "抓手工具"}
          aria-pressed={tool === "pan"}
          title={
            tool === "select"
              ? "选择工具 · V；点击切换抓手"
              : "抓手工具；点击切换选择"
          }
          onClick={() => onToolChange(tool === "select" ? "pan" : "select")}
        >
          {tool === "select" ? (
            <MousePointer2 data-icon="inline-start" />
          ) : (
            <Hand data-icon="inline-start" />
          )}
        </ToolButton>
        <ToolButton
          variant="ghost"
          size="icon-sm"
          aria-label="小地图"
          aria-pressed={miniMap}
          onClick={onMiniMap}
        >
          <Map data-icon="inline-start" />
        </ToolButton>
        <ToolButton
          variant="ghost"
          size="icon-sm"
          aria-label="显示连线"
          aria-pressed={showEdges}
          onClick={onShowEdges}
        >
          <Link2 data-icon="inline-start" />
        </ToolButton>
        <DropdownMenu
          onOpenChange={(open) => {
            if (open) setPercentage(String(Math.round(zoom * 100)));
          }}
        >
          <DropdownMenuTrigger asChild>
            <Button
              variant="ghost"
              size="sm"
              className="canvas-zoom-trigger"
              aria-label={`缩放比例 ${Math.round(zoom * 100)}%`}
            >
              {Math.round(zoom * 100)}%
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent
            align="start"
            side="top"
            className="canvas-view-menu"
          >
            <DropdownMenuGroup>
              <DropdownMenuItem
                disabled={zoom >= 8}
                onSelect={(event) => {
                  event.preventDefault();
                  onZoom(Math.min(8, zoom * 1.2));
                }}
              >
                <Plus />
                放大视图<Kbd className="ml-auto">⌘ +</Kbd>
              </DropdownMenuItem>
              <DropdownMenuItem
                disabled={zoom <= 0.08}
                onSelect={(event) => {
                  event.preventDefault();
                  onZoom(Math.max(0.08, zoom / 1.2));
                }}
              >
                <Minus />
                缩小视图<Kbd className="ml-auto">⌘ −</Kbd>
              </DropdownMenuItem>
              <DropdownMenuItem
                onSelect={(event) => {
                  event.preventDefault();
                  onFit();
                }}
              >
                <Maximize />
                适配画布<Kbd className="ml-auto">⇧ 1</Kbd>
              </DropdownMenuItem>
              <DropdownMenuItem
                disabled={!hasSelection}
                onSelect={(event) => {
                  event.preventDefault();
                  onFitSelection();
                }}
              >
                缩放至选中项<Kbd className="ml-auto">⇧ 2</Kbd>
              </DropdownMenuItem>
            </DropdownMenuGroup>
            <DropdownMenuSeparator />
            <DropdownMenuGroup>
              {[50, 100, 200].map((value) => (
                <DropdownMenuItem
                  key={value}
                  onSelect={(event) => {
                    event.preventDefault();
                    onZoom(value / 100);
                  }}
                >
                  缩放至{value}%
                  {value === 100 && <Kbd className="ml-auto">⌘ 1</Kbd>}
                </DropdownMenuItem>
              ))}
            </DropdownMenuGroup>
            <DropdownMenuSeparator />
            <Field>
              <FieldLabel className="sr-only" htmlFor="canvas-zoom-percent">
                缩放百分比
              </FieldLabel>
              <Input
                id="canvas-zoom-percent"
                inputMode="decimal"
                value={percentage}
                aria-label="缩放百分比"
                onChange={(event) => setPercentage(event.target.value)}
                onKeyDown={(event) => {
                  event.stopPropagation();
                  if (event.key === "Enter") {
                    const value = Number(percentage);
                    if (Number.isFinite(value) && value > 0) {
                      const clamped = Math.max(8, Math.min(800, value));
                      setPercentage(String(clamped));
                      onZoom(clamped / 100);
                    }
                  }
                }}
              />
            </Field>
          </DropdownMenuContent>
        </DropdownMenu>
        <Dialog open={shortcutsOpen} onOpenChange={onShortcutsOpenChange}>
          <DialogContent>
            <DialogTitle>画布快捷键</DialogTitle>
            <DialogDescription>
              编辑文字时，画布快捷键不会触发。生成草稿关联服务端版本后不能直接移除。
            </DialogDescription>
            <dl className="grid grid-cols-2 gap-4 text-sm">
              <dt>选择工具</dt>
              <dd>V；点击工具按钮切换抓手</dd>
              <dt>临时平移</dt>
              <dd>空格 + 拖动，或鼠标中键</dd>
              <dt>缩放</dt>
              <dd>Ctrl/⌘ + 滚轮 / 双指捏合</dd>
              <dt>平移</dt>
              <dd>滚轮 / 触控板双指滚动</dd>
              <dt>适配全图 / 选中项</dt>
              <dd>Shift+1 / Shift+2</dd>
              <dt>100%</dt>
              <dd>Ctrl/⌘ + 1</dd>
              <dt>移动节点</dt>
              <dd>方向键，Shift 加速</dd>
              <dt>撤销 / 重做编辑</dt>
              <dd>⌘ / Ctrl + Z、Shift + Z / Y</dd>
              <dt>保存</dt>
              <dd>⌘ / Ctrl + S</dd>
            </dl>
          </DialogContent>
        </Dialog>
      </div>
      <Button variant="ghost" className="canvas-ai-trigger" onClick={onAI}>
        <Sparkles data-icon="inline-start" />与 AI 对话
      </Button>
    </TooltipProvider>
  );
}
