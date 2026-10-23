import { Button } from "@/components/ui/button";
import { CanvasHistoryDialog } from "../history/canvas-history-dialog";
import { CanvasGuideDialog } from "./canvas-guide-dialog";
import { CanvasModeMenu } from "@/components/canvas/controls/canvas-mode-menu";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Field, FieldGroup, FieldLabel } from "@/components/ui/field";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import { Separator } from "@/components/ui/separator";
import { Switch } from "@/components/ui/switch";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import {
  canvasThemes,
  type CanvasBackgroundMode,
} from "@/components/canvas/runtime/lib/theme";
import {
  listNodeDefinitions,
  useNodeRegistryVersion,
} from "@/components/canvas/lib/node-registry";
import { useThemeStore } from "@/components/canvas/runtime/store/use-theme-store";
import {
  Eraser,
  History,
  Keyboard,
  Info,
  MoreHorizontal,
  Palette,
  Plus,
  Redo2,
  Trash2,
  Undo2,
  Upload,
} from "lucide-react";
import { useState } from "react";

export function CanvasToolbar({
  selectedCount,
  canvasTool,
  canUndo,
  canRedo,
  backgroundMode,
  showImageInfo,
  onCreateNode,
  onUndo,
  onRedo,
  onUpload,
  onDelete,
  onClear,
  onCanvasToolChange,
  onBackgroundModeChange,
  onShowImageInfoChange,
}: {
  selectedCount: number;
  canvasTool: "select" | "pan";
  canUndo: boolean;
  canRedo: boolean;
  backgroundMode: CanvasBackgroundMode;
  showImageInfo: boolean;
  onCreateNode: (type: string) => void;
  onUndo: () => void;
  onRedo: () => void;
  onUpload: () => void;
  onDelete: () => void;
  onClear: () => void;
  onCanvasToolChange: (tool: "select" | "pan") => void;
  onBackgroundModeChange: (mode: CanvasBackgroundMode) => void;
  onShowImageInfoChange: (show: boolean) => void;
}) {
  const [createOpen, setCreateOpen] = useState(false);
  const colorTheme = useThemeStore((state) => state.theme);
  const setTheme = useThemeStore((state) => state.setTheme);
  const theme = canvasThemes[colorTheme];
  useNodeRegistryVersion();
  const definitions = listNodeDefinitions().filter(
    (definition) => definition.showInCreateMenu !== false,
  );

  return (
    <>
      <div
        data-canvas-no-zoom
        className="canvas-action-dock max-w-node-label absolute bottom-2 left-1/2 z-50 flex -translate-x-1/2 items-center gap-2"
        aria-label="画布工具栏"
      >
        <div
          className="canvas-dock-surface flex items-center"
          style={{
            background: theme.toolbar.panel,
            borderColor: theme.toolbar.border,
          }}
        >
          <Popover open={createOpen} onOpenChange={setCreateOpen}>
            <Tooltip>
              <TooltipTrigger asChild>
                <PopoverTrigger asChild>
                  <Button size="icon-lg" aria-label="添加节点">
                    <Plus />
                  </Button>
                </PopoverTrigger>
              </TooltipTrigger>
              <TooltipContent side="top">添加节点</TooltipContent>
            </Tooltip>
            <PopoverContent
              side="top"
              align="start"
              sideOffset={12}
              className="max-h-menu w-64 overflow-y-auto"
              data-canvas-no-zoom
            >
              <p className="mb-3 text-sm font-medium">添加节点</p>
              <div className="grid grid-cols-2 gap-1">
                {definitions.map((definition) => (
                  <Button
                    key={definition.type}
                    variant="ghost"
                    className="justify-start"
                    onClick={() => {
                      onCreateNode(definition.type);
                      setCreateOpen(false);
                    }}
                  >
                    {definition.icon}
                    {definition.title}
                  </Button>
                ))}
              </div>
              <Separator className="my-3" />
              <Button
                variant="ghost"
                className="w-full justify-start"
                onClick={() => {
                  onUpload();
                  setCreateOpen(false);
                }}
              >
                <Upload />
                上传图片、视频或音频
              </Button>
            </PopoverContent>
          </Popover>
          <CanvasModeMenu value={canvasTool} onChange={onCanvasToolChange} />
          {[
            { label: "撤销", icon: Undo2, action: onUndo, enabled: canUndo },
            { label: "重做", icon: Redo2, action: onRedo, enabled: canRedo },
          ].map(({ label, icon: Icon, action, enabled }) => (
            <Tooltip key={label}>
              <TooltipTrigger asChild>
                <Button
                  variant="ghost"
                  size="icon-lg"
                  aria-label={label}
                  onClick={action}
                  disabled={!enabled}
                >
                  <Icon />
                </Button>
              </TooltipTrigger>
              <TooltipContent side="top">{label}</TooltipContent>
            </Tooltip>
          ))}
          <CanvasHistoryDialog>
            <Button
              variant="ghost"
              size="icon-lg"
              aria-label="生成历史"
              title="生成历史"
            >
              <History />
            </Button>
          </CanvasHistoryDialog>
          <CanvasGuideDialog kind="shortcuts">
            <Button
              variant="ghost"
              size="icon-lg"
              aria-label="快捷键"
              title="快捷键"
            >
              <Keyboard />
            </Button>
          </CanvasGuideDialog>
        </div>
      </div>
      <div
        data-canvas-no-zoom
        className="canvas-dock-options absolute right-4 bottom-4 z-50 flex items-center"
      >
        <Popover>
          <PopoverTrigger asChild>
            <Button
              variant="ghost"
              size="icon-lg"
              aria-label="画布外观"
              title="画布外观"
            >
              <Palette />
            </Button>
          </PopoverTrigger>
          <PopoverContent
            side="top"
            sideOffset={12}
            className="w-64"
            data-canvas-no-zoom
          >
            <FieldGroup>
              <Field>
                <FieldLabel className="sr-only">主题</FieldLabel>
                <ToggleGroup
                  type="single"
                  value={colorTheme}
                  onValueChange={(value) => {
                    if (value) setTheme(value as "light" | "dark");
                  }}
                  aria-label="主题"
                >
                  <ToggleGroupItem value="light">浅色</ToggleGroupItem>
                  <ToggleGroupItem value="dark">深色</ToggleGroupItem>
                </ToggleGroup>
              </Field>
              <Field>
                <FieldLabel>画布背景</FieldLabel>
                <ToggleGroup
                  type="single"
                  value={backgroundMode}
                  onValueChange={(value) => {
                    if (value)
                      onBackgroundModeChange(value as CanvasBackgroundMode);
                  }}
                  aria-label="画布背景"
                >
                  <ToggleGroupItem value="dots">点阵</ToggleGroupItem>
                  <ToggleGroupItem value="lines">网格</ToggleGroupItem>
                  <ToggleGroupItem value="blank">纯色</ToggleGroupItem>
                </ToggleGroup>
              </Field>
              <Field orientation="horizontal">
                <FieldLabel htmlFor="canvas-image-info">
                  <Info />
                  图片信息
                </FieldLabel>
                <Switch
                  id="canvas-image-info"
                  checked={showImageInfo}
                  onCheckedChange={onShowImageInfoChange}
                />
              </Field>
            </FieldGroup>
          </PopoverContent>
        </Popover>
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button variant="ghost" size="icon-lg" aria-label="更多画布操作">
              <MoreHorizontal />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent side="top" align="end">
            <DropdownMenuGroup>
              <DropdownMenuItem
                variant="destructive"
                disabled={!selectedCount}
                onSelect={onDelete}
              >
                <Trash2 />
                删除选中
              </DropdownMenuItem>
              <DropdownMenuItem variant="destructive" onSelect={onClear}>
                <Eraser />
                清空画布
              </DropdownMenuItem>
            </DropdownMenuGroup>
          </DropdownMenuContent>
        </DropdownMenu>
      </div>
    </>
  );
}
