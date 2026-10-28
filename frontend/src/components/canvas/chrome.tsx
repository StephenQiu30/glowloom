"use client";

import { useState, type ComponentProps, type ReactNode } from "react";
import Link from "next/link";
import {
  AudioLines,
  Box,
  ChevronDown,
  Contact,
  Download,
  Folder,
  Hand,
  History,
  ImageIcon,
  Link2,
  Map,
  Maximize,
  Minus,
  MoreHorizontal,
  MousePointer2,
  Plus,
  Redo2,
  RotateCw,
  Search,
  Sparkles,
  Type,
  Undo2,
  Upload,
  UserRound,
  Video,
} from "lucide-react";
import { BrandMark } from "@/components/brand/brand";
import { Button } from "@/components/ui/button";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import {
  Command,
  CommandDialog,
  CommandEmpty,
  CommandInput,
  CommandItem,
  CommandList,
} from "@/components/ui/command";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import type { CanvasBackgroundMode } from "./theme";

export function CanvasIconButton({
  label,
  children,
  ...props
}: ComponentProps<typeof Button> & { label: string }) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button variant="ghost" size="icon" aria-label={label} {...props}>
          {children}
        </Button>
      </TooltipTrigger>
      <TooltipContent side="right">{label}</TooltipContent>
    </Tooltip>
  );
}

function Unavailable({
  label,
  reason,
  children,
  className,
}: {
  label: string;
  reason: string;
  children: ReactNode;
  className?: string;
}) {
  return (
    <Popover>
      <PopoverTrigger asChild>
        <Button variant="ghost" aria-label={label} className={className}>
          {children}
        </Button>
      </PopoverTrigger>
      <PopoverContent side="right" collisionPadding={12} className="w-60">
        <p className="font-medium">{label}</p>
        <p className="text-xs leading-5 text-muted-foreground">{reason}</p>
      </PopoverContent>
    </Popover>
  );
}

type SearchNode = { id: string; title: string };
export function CanvasHeader({
  name,
  status,
  nodes,
  onLocate,
  onExport,
  onImport,
  onRefresh,
  history,
}: {
  name: string;
  status: string;
  nodes: SearchNode[];
  onLocate: (id: string) => void;
  onExport?: () => void;
  onImport?: () => void;
  onRefresh?: () => void;
  history?: ReactNode;
}) {
  const [search, setSearch] = useState(false);
  return (
    <>
      <header
        data-canvas-chrome="header"
        className="absolute top-2.5 right-3 left-3 flex h-10 items-center gap-1 sm:gap-2"
      >
        <Button
          asChild
          variant="ghost"
          size="icon"
          className="shrink-0 [&_.brand-mark]:size-6"
        >
          <Link href="/projects" aria-label="返回画布列表">
            <BrandMark />
          </Link>
        </Button>
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button
              variant="ghost"
              size="sm"
              className="min-w-0 gap-1.5 px-2"
              aria-label="画布菜单"
            >
              <ChevronDown className="shrink-0" />
              <h1 className="truncate text-sm font-medium">{name}</h1>
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent className="w-60" collisionPadding={12}>
            <DropdownMenuLabel className="break-words whitespace-normal">
              {name}
            </DropdownMenuLabel>
            <DropdownMenuItem asChild>
              <Link href="/projects">
                <Folder />
                我的画布
              </Link>
            </DropdownMenuItem>
            {onImport && (
              <DropdownMenuItem onSelect={onImport}>
                <Upload />
                导入画布文件
              </DropdownMenuItem>
            )}
            {onExport && (
              <DropdownMenuItem onSelect={onExport}>
                <Download />
                导出画布文件
              </DropdownMenuItem>
            )}
            <DropdownMenuSeparator />
            <p className="px-2 py-1.5 text-xs leading-5 text-muted-foreground">
              {status}
            </p>
          </DropdownMenuContent>
        </DropdownMenu>
        <span
          role="status"
          className="hidden truncate text-xs text-muted-foreground lg:block"
        >
          {status}
        </span>
        <div className="flex-1" />
        <CanvasIconButton
          label="搜索节点"
          onClick={() => setSearch(true)}
          className="shrink-0"
        >
          <Search className="size-5" />
        </CanvasIconButton>
        {history && (
          <Popover>
            <PopoverTrigger asChild>
              <Button
                variant="ghost"
                size="icon"
                aria-label="编辑历史"
                className="shrink-0"
              >
                <History className="size-5" />
              </Button>
            </PopoverTrigger>
            <PopoverContent align="end" className="w-52">
              <p className="text-xs text-muted-foreground">编辑历史</p>
              <div className="flex gap-1">{history}</div>
            </PopoverContent>
          </Popover>
        )}
        <Unavailable
          label="分享画布"
          reason="此画布暂未开放链接分享。"
          className="hidden h-9 px-3 sm:inline-flex"
        >
          分享
        </Unavailable>
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button
              variant="secondary"
              size="icon"
              aria-label="更多画布操作"
              className="shrink-0"
            >
              <MoreHorizontal className="size-5" />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent
            align="end"
            className="w-52"
            collisionPadding={12}
          >
            {onExport && (
              <DropdownMenuItem onSelect={onExport}>
                <Download />
                导出画布文件
              </DropdownMenuItem>
            )}
            {onRefresh && (
              <DropdownMenuItem onSelect={onRefresh}>
                <RotateCw />
                重新读取画布
              </DropdownMenuItem>
            )}
            <DropdownMenuItem asChild>
              <Link href="/projects">
                <Folder />
                返回画布列表
              </Link>
            </DropdownMenuItem>
            <DropdownMenuSeparator />
            <DropdownMenuItem asChild>
              <a
                href="/licenses/infinite-canvas.txt"
                target="_blank"
                rel="noreferrer"
              >
                画布开源许可
              </a>
            </DropdownMenuItem>
            <p className="px-2 py-1.5 text-xs leading-5 text-muted-foreground">
              {status}
            </p>
          </DropdownMenuContent>
        </DropdownMenu>
        <Button asChild variant="ghost" size="icon" className="shrink-0">
          <Link href="/account" aria-label="个人空间">
            <UserRound className="size-5" />
          </Link>
        </Button>
      </header>
      <CommandDialog
        open={search}
        onOpenChange={setSearch}
        title="搜索画布节点"
        description="按节点名称搜索并定位到画布中。"
      >
        <Command>
          <CommandInput placeholder="搜索节点…" />
          <CommandList>
            <CommandEmpty>没有找到节点</CommandEmpty>
            {nodes.map((node) => (
              <CommandItem
                key={node.id}
                value={`${node.title} ${node.id}`}
                onSelect={() => {
                  setSearch(false);
                  onLocate(node.id);
                }}
              >
                <Search />
                <span className="truncate">{node.title}</span>
              </CommandItem>
            ))}
          </CommandList>
        </Command>
      </CommandDialog>
    </>
  );
}

export type CanvasCreateKind =
  "text" | "image" | "video" | "audio" | "upload" | "library";
export function CanvasToolPalette({
  onCreate,
  readOnly = false,
  busy = false,
}: {
  onCreate?: (kind: CanvasCreateKind) => void;
  readOnly?: boolean;
  busy?: boolean;
}) {
  const tools = [
    { kind: "text", label: "添加文本", Icon: Type },
    { kind: "image", label: "添加图片", Icon: ImageIcon },
    { kind: "video", label: "添加视频", Icon: Video },
    { kind: "audio", label: "添加音频", Icon: AudioLines },
  ] as const;
  return (
    <div
      role="toolbar"
      aria-label="画布工具"
      aria-orientation="vertical"
      data-canvas-chrome="tools"
      className="absolute top-1/2 left-3 flex max-h-[calc(100dvh-10rem)] w-12 -translate-y-1/2 flex-col items-center gap-2 overflow-y-auto rounded-2xl border bg-card py-1 shadow-sm [&_button]:size-11 [&_button]:shrink-0 [&_svg]:size-5"
    >
      {tools.map(({ kind, label, Icon }) =>
        readOnly ? (
          <Unavailable
            key={kind}
            label={`${label}（只读）`}
            reason="此画布含历史内容，当前以只读方式打开。"
          >
            <Icon />
          </Unavailable>
        ) : (
          <CanvasIconButton
            key={kind}
            label={label}
            disabled={busy}
            onClick={() => onCreate?.(kind)}
          >
            <Icon />
          </CanvasIconButton>
        ),
      )}
      <Unavailable label="主体" reason="画布暂未开放主体节点。">
        <Contact />
      </Unavailable>
      <Unavailable
        label="3D 导演"
        reason="3D 导演暂不可用。"
        className="opacity-40"
      >
        <Box />
      </Unavailable>
      <div className="h-3 shrink-0" />
      {readOnly ? (
        <Unavailable
          label="参考素材（只读）"
          reason="此画布当前为只读，不能添加素材。"
        >
          <Folder />
        </Unavailable>
      ) : (
        <CanvasIconButton
          label="从参考素材添加"
          disabled={busy}
          onClick={() => onCreate?.("library")}
        >
          <Folder />
        </CanvasIconButton>
      )}
      {readOnly ? (
        <Unavailable
          label="上传素材（只读）"
          reason="此画布当前为只读，不能上传素材。"
        >
          <Upload />
        </Unavailable>
      ) : (
        <CanvasIconButton
          label="导入素材"
          disabled={busy}
          onClick={() => onCreate?.("upload")}
        >
          <Upload />
        </CanvasIconButton>
      )}
    </div>
  );
}

export function CanvasViewControls({
  zoom,
  onZoom,
  onFit,
  onFitSelection,
  tool,
  onTool,
  map,
  onMap,
  connections,
  onConnections,
  background,
  onBackground,
  snap,
  onSnap,
}: {
  zoom: number;
  onZoom: (value: number) => void;
  onFit: () => void;
  onFitSelection?: () => void;
  tool: "select" | "pan";
  onTool?: (tool: "select" | "pan") => void;
  map?: boolean;
  onMap?: () => void;
  connections: boolean;
  onConnections: () => void;
  background: CanvasBackgroundMode;
  onBackground: (mode: CanvasBackgroundMode) => void;
  snap?: boolean;
  onSnap?: () => void;
}) {
  return (
    <div
      role="toolbar"
      aria-label="画布视图"
      data-canvas-chrome="view"
      className="absolute bottom-3 left-3 flex h-9 items-center gap-1 rounded-lg bg-background/90 sm:gap-2"
    >
      {onTool ? (
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button variant="secondary" size="icon" aria-label="切换画布工具">
              {tool === "select" ? (
                <MousePointer2 className="size-5" />
              ) : (
                <Hand className="size-5" />
              )}
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent side="top" className="w-44">
            <DropdownMenuRadioGroup
              value={tool}
              onValueChange={(v) => onTool(v as "select" | "pan")}
            >
              <DropdownMenuRadioItem value="select">
                选择工具{" "}
                <span className="ml-auto text-xs text-muted-foreground">V</span>
              </DropdownMenuRadioItem>
              <DropdownMenuRadioItem value="pan">
                平移工具{" "}
                <span className="ml-auto text-xs text-muted-foreground">H</span>
              </DropdownMenuRadioItem>
            </DropdownMenuRadioGroup>
          </DropdownMenuContent>
        </DropdownMenu>
      ) : (
        <CanvasIconButton label="平移工具" variant="secondary">
          <Hand className="size-5" />
        </CanvasIconButton>
      )}
      {onMap && (
        <CanvasIconButton
          label="小地图"
          aria-pressed={map}
          variant={map ? "secondary" : "ghost"}
          onClick={onMap}
        >
          <Map className="size-5" />
        </CanvasIconButton>
      )}
      <CanvasIconButton
        label="显示连线"
        aria-pressed={connections}
        variant={connections ? "secondary" : "ghost"}
        onClick={onConnections}
      >
        <Link2 className="size-5" />
      </CanvasIconButton>
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button
            variant="ghost"
            className="min-w-18 tabular-nums"
            aria-label="缩放菜单"
          >
            {Math.round(zoom * 100)}%
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent
          side="top"
          align="start"
          collisionPadding={12}
          className="w-52 p-2 [&_[role^=menuitem]]:min-h-9"
        >
          <DropdownMenuItem onSelect={() => onZoom(zoom * 1.2)}>
            <Plus />
            放大视图
          </DropdownMenuItem>
          <DropdownMenuItem onSelect={() => onZoom(zoom / 1.2)}>
            <Minus />
            缩小视图
          </DropdownMenuItem>
          <DropdownMenuItem onSelect={onFit}>
            <Maximize />
            适合屏幕
          </DropdownMenuItem>
          <DropdownMenuItem
            disabled={!onFitSelection}
            onSelect={onFitSelection}
          >
            缩放至选中项
          </DropdownMenuItem>
          <DropdownMenuSeparator />
          {[0.5, 1, 2].map((value) => (
            <DropdownMenuItem key={value} onSelect={() => onZoom(value)}>
              缩放至 {value * 100}%
            </DropdownMenuItem>
          ))}
          <DropdownMenuSeparator />
          <DropdownMenuLabel>画布背景</DropdownMenuLabel>
          {onSnap && (
            <DropdownMenuItem onSelect={onSnap}>
              {snap ? "关闭吸附对齐" : "开启吸附对齐"}
            </DropdownMenuItem>
          )}
          <DropdownMenuRadioGroup
            value={background}
            onValueChange={(v) => onBackground(v as CanvasBackgroundMode)}
          >
            <DropdownMenuRadioItem value="dots">点阵</DropdownMenuRadioItem>
            <DropdownMenuRadioItem value="lines">网格</DropdownMenuRadioItem>
            <DropdownMenuRadioItem value="blank">无背景</DropdownMenuRadioItem>
          </DropdownMenuRadioGroup>
        </DropdownMenuContent>
      </DropdownMenu>
    </div>
  );
}

export function CanvasAssistantEntry() {
  return (
    <div className="absolute right-3 bottom-3 hidden sm:block">
      <Unavailable
        label="与 AI 对话"
        reason="画布助手暂未开放。"
        className="h-9 gap-1.5 rounded-full bg-card px-3 text-xs"
      >
        <Sparkles className="size-4" />与 AI 对话
      </Unavailable>
    </div>
  );
}

export function CanvasHistoryActions({
  undo,
  redo,
  onUndo,
  onRedo,
}: {
  undo: boolean;
  redo: boolean;
  onUndo: () => void;
  onRedo: () => void;
}) {
  return (
    <>
      <CanvasIconButton label="撤销" disabled={!undo} onClick={onUndo}>
        <Undo2 />
      </CanvasIconButton>
      <CanvasIconButton label="重做" disabled={!redo} onClick={onRedo}>
        <Redo2 />
      </CanvasIconButton>
    </>
  );
}
