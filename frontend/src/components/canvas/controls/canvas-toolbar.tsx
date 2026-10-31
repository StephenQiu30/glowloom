import type { ReactNode } from "react";
import { ThemeToggle } from "@/components/controls/theme-controls";
import { Button } from "@/components/ui/button";
import { CanvasGuideDialog } from "./canvas-guide-dialog";
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
import type { CanvasBackgroundMode } from "@/components/canvas/runtime/lib/theme";
import {
  listNodeDefinitions,
  useNodeRegistryVersion,
} from "@/components/canvas/lib/node-registry";
import {
  Sparkles,
  Camera,
  Palette,
  Clapperboard,
  MoveRight,
  Folder,
  Eraser,
  Keyboard,
  Info,
  MoreHorizontal,
  Redo2,
  Trash2,
  Undo2,
  Upload,
} from "lucide-react";

export function CanvasToolbar({
  header,
  viewControls,
  canEdit = true,
  selectedCount,
  canUndo,
  canRedo,
  backgroundMode,
  showImageInfo,
  onCreateNode,
  onUndo,
  onRedo,
  onUpload,
  onBrowseAssets,
  onOpenAssistant,
  onDelete,
  onClear,
  onBackgroundModeChange,
  onShowImageInfoChange,
}: {
  header?: ReactNode;
  viewControls?: ReactNode;
  canEdit?: boolean;
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
  onBrowseAssets: () => void;
  onOpenAssistant?: () => void;
  onDelete: () => void;
  onClear: () => void;
  onCanvasToolChange: (tool: "select" | "pan") => void;
  onBackgroundModeChange: (mode: CanvasBackgroundMode) => void;
  onShowImageInfoChange: (show: boolean) => void;
}) {
  useNodeRegistryVersion();
  const definitions = listNodeDefinitions().filter(
    (definition) => definition.showInCreateMenu !== false,
  );

  return (
    <div className="canvas-controls-layout" data-canvas-controls>
      <header className="canvas-control-header" data-canvas-no-zoom>
        {header}
        <div
          data-canvas-no-zoom
          className="canvas-global-actions flex shrink-0 items-center gap-1"
        >
          <Popover>
            <PopoverTrigger asChild>
              <Button
                variant="ghost"
                size="icon-lg"
                className="canvas-more-trigger"
                aria-label="更多画布操作"
                title="更多画布操作"
              >
                <MoreHorizontal data-icon="inline-start" />
              </Button>
            </PopoverTrigger>
            <PopoverContent
              side="bottom"
              align="end"
              sideOffset={8}
              className="w-64"
              data-canvas-no-zoom
            >
              <div className="mb-3 flex flex-wrap items-center gap-1">
                {[
                  {
                    label: "撤销",
                    icon: Undo2,
                    action: onUndo,
                    enabled: canUndo,
                  },
                  {
                    label: "重做",
                    icon: Redo2,
                    action: onRedo,
                    enabled: canRedo,
                  },
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
                        <Icon data-icon="inline-start" />
                      </Button>
                    </TooltipTrigger>
                    <TooltipContent side="top">{label}</TooltipContent>
                  </Tooltip>
                ))}

                <CanvasGuideDialog kind="shortcuts">
                  <Button
                    variant="ghost"
                    size="icon-lg"
                    aria-label="快捷键"
                    title="快捷键"
                  >
                    <Keyboard data-icon="inline-start" />
                  </Button>
                </CanvasGuideDialog>
              </div>
              <FieldGroup>
                <Field orientation="horizontal">
                  <FieldLabel>主题</FieldLabel>
                  <ThemeToggle />
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
                    <ToggleGroupItem value="blank">无背景</ToggleGroupItem>
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
              <Separator className="my-3" />
              {definitions
                .filter(
                  (item) =>
                    ![
                      "text",
                      "image",
                      "video",
                      "audio",
                      "timeline",
                      "subject",
                    ].includes(item.type),
                )
                .map((definition) => (
                  <Button
                    key={definition.type}
                    variant="ghost"
                    className="w-full justify-start"
                    disabled={!canEdit}
                    onClick={() => onCreateNode(definition.type)}
                  >
                    {definition.icon}添加{definition.title}节点
                  </Button>
                ))}
              <Button
                variant="destructive"
                className="w-full justify-start"
                disabled={!canEdit || !selectedCount}
                onClick={onDelete}
              >
                <Trash2 />
                删除选中
              </Button>
              <Button
                variant="destructive"
                className="w-full justify-start"
                disabled={!canEdit}
                onClick={onClear}
              >
                <Eraser />
                清空画布
              </Button>
            </PopoverContent>
          </Popover>
        </div>
      </header>
      <div
        data-canvas-no-zoom
        className="canvas-action-dock"
        aria-label="画布工具栏"
      >
        <div className="canvas-dock-surface flex flex-col items-center">
          {definitions
            .filter((item) =>
              [
                "text",
                "image",
                "video",
                "audio",
                "timeline",
                "subject",
                "director",
              ].includes(item.type),
            )
            .sort(
              (a, b) =>
                [
                  "text",
                  "image",
                  "video",
                  "audio",
                  "timeline",
                  "subject",
                  "director",
                ].indexOf(a.type) -
                [
                  "text",
                  "image",
                  "video",
                  "audio",
                  "timeline",
                  "subject",
                  "director",
                ].indexOf(b.type),
            )
            .map((definition) =>
              definition.type === "director" ? (
                <div key={definition.type} className="flex flex-col">
                  <Popover>
                    <PopoverTrigger asChild>
                      <Button
                        variant="ghost"
                        size="icon-lg"
                        aria-label="导演工具箱"
                        disabled={!canEdit}
                      >
                        <Clapperboard />
                      </Button>
                    </PopoverTrigger>
                    <PopoverContent
                      side="right"
                      align="center"
                      className="w-44 p-1"
                      data-canvas-no-zoom
                      data-canvas-shortcuts-ignore
                    >
                      {(
                        [
                          { kind: "camera", label: "相机", Icon: Camera },
                          { kind: "palette", label: "配色", Icon: Palette },
                          { kind: "style", label: "风格", Icon: Clapperboard },
                          { kind: "motion", label: "运镜", Icon: MoveRight },
                        ] as const
                      ).map(({ kind, label, Icon }) => (
                        <Button
                          key={kind}
                          variant="ghost"
                          className="w-full justify-start"
                          onClick={() => onCreateNode(`cinema_studio:${kind}`)}
                        >
                          <Icon />
                          {label}
                        </Button>
                      ))}
                    </PopoverContent>
                  </Popover>
                  <Tooltip>
                    <TooltipTrigger asChild>
                      <Button
                        variant="ghost"
                        size="icon-lg"
                        aria-label={`添加${definition.title}节点`}
                        disabled={!canEdit}
                        onClick={() => onCreateNode(definition.type)}
                      >
                        {definition.icon}
                      </Button>
                    </TooltipTrigger>
                    <TooltipContent side="right">
                      {definition.title}
                    </TooltipContent>
                  </Tooltip>
                </div>
              ) : (
                <Tooltip key={definition.type}>
                  <TooltipTrigger asChild>
                    <Button
                      variant="ghost"
                      size="icon-lg"
                      aria-label={`添加${definition.title}节点`}
                      disabled={!canEdit}
                      onClick={() => onCreateNode(definition.type)}
                    >
                      {definition.icon}
                    </Button>
                  </TooltipTrigger>
                  <TooltipContent side="right">
                    {definition.title}
                  </TooltipContent>
                </Tooltip>
              ),
            )}
          <Separator className="my-1 max-w-8" />
          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                variant="ghost"
                size="icon-lg"
                aria-label="资产管理"
                disabled={!canEdit}
                onClick={onBrowseAssets}
              >
                <Folder data-icon="inline-start" />
              </Button>
            </TooltipTrigger>
            <TooltipContent side="right">从参考素材添加</TooltipContent>
          </Tooltip>
          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                variant="ghost"
                size="icon-lg"
                aria-label="本地上传"
                disabled={!canEdit}
                onClick={onUpload}
              >
                <Upload data-icon="inline-start" />
              </Button>
            </TooltipTrigger>
            <TooltipContent side="right">本地上传</TooltipContent>
          </Tooltip>
        </div>
      </div>
      <footer className="canvas-control-footer" data-canvas-no-zoom>
        {viewControls}
        <div className="canvas-assistant-action">
          <Tooltip>
            <TooltipTrigger asChild>
              <span
                tabIndex={onOpenAssistant ? undefined : 0}
                aria-label={onOpenAssistant ? undefined : "AI 对话暂不可用"}
              >
                <Button
                  variant="secondary"
                  size="sm"
                  aria-label="与 AI 对话"
                  onClick={onOpenAssistant}
                  disabled={!onOpenAssistant}
                  className="rounded-full"
                >
                  <Sparkles data-icon="inline-start" />
                  <span className="hidden sm:inline">与 AI 对话</span>
                </Button>
              </span>
            </TooltipTrigger>
            <TooltipContent side="top">
              {onOpenAssistant ? "与 AI 对话" : "AI 对话暂不可用"}
            </TooltipContent>
          </Tooltip>
        </div>
      </footer>
    </div>
  );
}
