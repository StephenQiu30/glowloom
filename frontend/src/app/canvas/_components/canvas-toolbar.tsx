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
  Clapperboard,
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
  Keyboard,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogTitle,
  DialogDescription,
  DialogTrigger,
} from "@/components/ui/dialog";
import { UnavailableDialog } from "@/components/availability/unavailable";
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
  onAdd,
  zoom,
  onZoom,
  onFit,
  miniMap,
  onMiniMap,
}: {
  tool: "select" | "pan";
  onToolChange: (tool: "select" | "pan") => void;
  onAdd: (kind: "image_generation" | "video_generation") => void;
  zoom: number;
  onZoom: (zoom: number) => void;
  onFit: () => void;
  miniMap: boolean;
  onMiniMap: () => void;
}) {
  const [feature, setFeature] = useState<string | null>(null);
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
          onClick={() => setFeature("文本节点")}
        >
          <Type />
        </ToolButton>
        <ToolButton
          variant="ghost"
          size="icon"
          aria-label="新增图片节点"
          title="图片"
          onClick={() => onAdd("image_generation")}
        >
          <ImageIcon />
        </ToolButton>
        <ToolButton
          variant="ghost"
          size="icon"
          aria-label="新增视频节点"
          title="视频"
          onClick={() => onAdd("video_generation")}
        >
          <VideoIcon />
        </ToolButton>
        {[
          { name: "音频", icon: AudioLines },
          { name: "分镜", icon: Clapperboard },
          { name: "主体", icon: Contact },
          { name: "3D", icon: Box },
        ].map(({ name, icon: Icon }) => (
          <ToolButton
            key={name}
            variant="ghost"
            size="icon"
            aria-label={name}
            title={name}
            onClick={() => setFeature(name)}
          >
            <Icon />
          </ToolButton>
        ))}
        <Separator className="canvas-dock-divider" />
        <ToolButton
          variant="ghost"
          size="icon"
          aria-label="素材库"
          title="素材库"
          onClick={() => setFeature("素材库")}
        >
          <Folder />
        </ToolButton>
        <ToolButton
          variant="ghost"
          size="icon"
          aria-label="上传素材"
          title="上传素材"
          onClick={() => setFeature("素材上传")}
        >
          <Upload />
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
          aria-label="区域选择"
          aria-pressed={tool === "select"}
          onClick={() => onToolChange("select")}
        >
          <MousePointer2 />
        </ToolButton>
        <ToolButton
          variant="ghost"
          size="icon-sm"
          aria-label="抓手工具"
          aria-pressed={tool === "pan"}
          onClick={() => onToolChange("pan")}
        >
          <Hand />
        </ToolButton>
        <ToolButton
          variant="ghost"
          size="icon-sm"
          aria-label="缩小"
          onClick={() => onZoom(Math.max(0.05, zoom / 1.25))}
        >
          <Minus />
        </ToolButton>
        <span className="min-w-10 text-center text-xs">
          {Math.round(zoom * 100)}%
        </span>
        <ToolButton
          variant="ghost"
          size="icon-sm"
          aria-label="放大"
          onClick={() => onZoom(Math.min(2, zoom * 1.25))}
        >
          <Plus />
        </ToolButton>
        <ToolButton
          variant="ghost"
          size="icon-sm"
          aria-label="适应画布"
          onClick={onFit}
        >
          <Maximize />
        </ToolButton>
        <ToolButton
          variant="ghost"
          size="icon-sm"
          aria-label="小地图"
          aria-pressed={miniMap}
          onClick={onMiniMap}
        >
          <Map />
        </ToolButton>
        <Dialog>
          <DialogTrigger asChild>
            <Button variant="ghost" size="icon-sm" aria-label="画布快捷键">
              <Keyboard />
            </Button>
          </DialogTrigger>
          <DialogContent>
            <DialogTitle>画布快捷键</DialogTitle>
            <DialogDescription>
              编辑文字时，画布快捷键不会触发。
            </DialogDescription>
            <dl className="grid grid-cols-2 gap-4 text-sm">
              <dt>选择 / 抓手</dt>
              <dd>V / H</dd>
              <dt>临时平移</dt>
              <dd>空格 + 拖动，或鼠标中键</dd>
              <dt>缩放</dt>
              <dd>滚轮 / 双指捏合</dd>
              <dt>移动节点</dt>
              <dd>方向键，Shift 加速</dd>
              <dt>保存</dt>
              <dd>⌘ / Ctrl + S</dd>
            </dl>
          </DialogContent>
        </Dialog>
      </div>
      <UnavailableDialog feature={feature} onClose={() => setFeature(null)} />
    </TooltipProvider>
  );
}
