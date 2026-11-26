import { useIsMobile } from "@/hooks/use-mobile";
import {
  CanvasToolbarButton,
  CanvasToolbarGroup,
} from "./canvas-toolbar-primitives.tsx";
import { useState, type ComponentProps, type ReactNode } from "react";
import { ThemeToggle } from "@/components/controls/theme-controls";
import { Button } from "@/components/ui/button";
import { CanvasGuideDialog } from "./canvas-guide-dialog.tsx";
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
import type { CanvasBackgroundMode } from "../runtime/lib/theme.ts";
import {
  listNodeDefinitions,
  useNodeRegistryVersion,
} from "../lib/node-registry.ts";
import {
  Sparkles,
  Camera,
  Palette,
  Clapperboard,
  Axis3D,
  Film,
  SquareUserRound,
  SwatchBook,
  MoveRight,
  FolderClosed,
  Eraser,
  Keyboard,
  Info,
  MoreHorizontal,
  Plus,
  Redo2,
  Trash2,
  Undo2,
  Upload,
} from "lucide-react";

function CanvasCreationTool({
  label,
  children,
  ...props
}: ComponentProps<typeof CanvasToolbarButton> & { label: string }) {
  return (
    <CanvasToolbarButton
      {...props}
      className="canvas-creation-tool"
      data-icon-scale="navigation"
    >
      {children}
      <span className="canvas-creation-label" aria-hidden="true">
        {label}
      </span>
    </CanvasToolbarButton>
  );
}

export type CanvasMoreControl = (options?: {
  shortcuts?: ReactNode;
  appearanceInAccount?: boolean;
}) => ReactNode;

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
  assistantOpen,
  onDelete,
  onClear,
  onBackgroundModeChange,
  onShowImageInfoChange,
}: {
  header?: ReactNode | ((more: CanvasMoreControl) => ReactNode);
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
  assistantOpen?: boolean;
  onDelete: () => void;
  onClear: () => void;
  onCanvasToolChange: (tool: "select" | "pan") => void;
  onBackgroundModeChange: (mode: CanvasBackgroundMode) => void;
  onShowImageInfoChange: (show: boolean) => void;
}) {
  useNodeRegistryVersion();
  const isMobile = useIsMobile();
  const [createOpen, setCreateOpen] = useState(false);
  const definitions = listNodeDefinitions().filter(
    (definition) => definition.showInCreateMenu !== false,
  );

  const create = (type: string) => {
    setCreateOpen(false);
    onCreateNode(type);
  };
  const dockOrder = [
    "text",
    "image",
    "video",
    "audio",
    "timeline",
    "subject",
    "director",
  ];
  const dock = (
    <CanvasToolbarGroup
      layout={isMobile ? "grid" : "rail"}
      className={isMobile ? undefined : "canvas-creation-rail"}
      data-canvas-no-zoom
      aria-label="画布工具栏"
    >
      {definitions
        .filter((item) => dockOrder.includes(item.type))
        .sort((a, b) => dockOrder.indexOf(a.type) - dockOrder.indexOf(b.type))
        .map((definition) =>
          definition.type === "director" ? (
            <div key={definition.type} className="contents">
              <Popover>
                <PopoverTrigger asChild>
                  <CanvasCreationTool
                    label="导演工具箱"
                    aria-label="导演工具箱"
                    disabled={!canEdit}
                  >
                    <Clapperboard />
                  </CanvasCreationTool>
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
                      { kind: "style", label: "风格", Icon: SwatchBook },
                      { kind: "motion", label: "运镜", Icon: MoveRight },
                    ] as const
                  ).map(({ kind, label, Icon }) => (
                    <Button
                      key={kind}
                      variant="ghost"
                      className="w-full justify-start"
                      onClick={() => create(`cinema_studio:${kind}`)}
                    >
                      <Icon />
                      {label}
                    </Button>
                  ))}
                </PopoverContent>
              </Popover>
              <CanvasCreationTool
                label={definition.title}
                aria-label={`添加${definition.title}节点`}
                disabled={!canEdit}
                onClick={() => create(definition.type)}
              >
                <Axis3D />
              </CanvasCreationTool>
            </div>
          ) : (
            <CanvasCreationTool
              key={definition.type}
              label={definition.title}
              aria-label={`添加${definition.title}节点`}
              disabled={!canEdit}
              onClick={() => create(definition.type)}
            >
              {definition.type === "timeline" ? (
                <Film />
              ) : definition.type === "subject" ? (
                <SquareUserRound />
              ) : (
                definition.icon
              )}
            </CanvasCreationTool>
          ),
        )}
      <Separator className="canvas-dock-divider" />
      <CanvasCreationTool
        label="素材库"
        aria-label="从素材库添加"
        disabled={!canEdit}
        onClick={() => {
          setCreateOpen(false);
          onBrowseAssets();
        }}
      >
        <FolderClosed aria-hidden="true" />
      </CanvasCreationTool>
      <CanvasCreationTool
        label="上传"
        aria-label="本地上传"
        disabled={!canEdit}
        onClick={() => {
          setCreateOpen(false);
          onUpload();
        }}
      >
        <Upload />
      </CanvasCreationTool>
    </CanvasToolbarGroup>
  );
  const renderMore: CanvasMoreControl = ({
    shortcuts,
    appearanceInAccount,
  } = {}) => (
    <CanvasToolbarGroup aria-label="画布操作">
      <Popover>
        <PopoverTrigger asChild>
          <CanvasToolbarButton aria-label="更多画布操作" title="更多画布操作">
            <MoreHorizontal />
          </CanvasToolbarButton>
        </PopoverTrigger>
        <PopoverContent
          side="bottom"
          align="end"
          sideOffset={8}
          className="w-64"
          data-canvas-no-zoom
        >
          {shortcuts && (
            <>
              <div
                className="flex flex-wrap items-center gap-1"
                aria-label="画布快捷入口"
              >
                {shortcuts}
              </div>
              <Separator className="my-3" />
            </>
          )}
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
                    size="toolbar-icon"
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
                size="toolbar-icon"
                aria-label="快捷键"
                title="快捷键"
              >
                <Keyboard data-icon="inline-start" />
              </Button>
            </CanvasGuideDialog>
          </div>
          <FieldGroup>
            {!appearanceInAccount && (
              <Field orientation="horizontal">
                <FieldLabel>主题</FieldLabel>
                <ThemeToggle />
              </Field>
            )}
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
    </CanvasToolbarGroup>
  );
  return (
    <div className="canvas-controls-layout" data-canvas-controls>
      <header className="canvas-control-header" data-canvas-no-zoom>
        {typeof header === "function" ? (
          (header(renderMore) ?? renderMore())
        ) : (
          <>
            {header}
            {renderMore()}
          </>
        )}
      </header>
      {!isMobile && <div className="canvas-action-dock">{dock}</div>}
      <footer className="canvas-control-footer" data-canvas-no-zoom>
        <div className="flex min-w-0 items-center gap-1">
          {viewControls}
          {isMobile && (
            <Popover open={createOpen} onOpenChange={setCreateOpen}>
              <PopoverTrigger asChild>
                <CanvasToolbarButton aria-label="添加节点" disabled={!canEdit}>
                  <Plus />
                </CanvasToolbarButton>
              </PopoverTrigger>
              <PopoverContent
                side="top"
                align="end"
                collisionPadding={12}
                className="w-auto max-w-[calc(100dvw-24px)]"
                aria-label="添加画布节点和素材"
                data-canvas-no-zoom
              >
                {dock}
              </PopoverContent>
            </Popover>
          )}
        </div>
        <CanvasToolbarGroup
          appearance="plain"
          className={
            assistantOpen
              ? "canvas-assistant-action invisible"
              : "canvas-assistant-action"
          }
        >
          <Tooltip>
            <TooltipTrigger asChild>
              <span
                tabIndex={onOpenAssistant ? undefined : 0}
                aria-label={onOpenAssistant ? undefined : "AI 对话暂不可用"}
              >
                <CanvasToolbarButton
                  iconOnly={isMobile}
                  variant="outline"
                  aria-label="与 AI 对话"
                  aria-expanded={assistantOpen}
                  aria-controls={
                    assistantOpen ? "canvas-conversation-panel" : undefined
                  }
                  onClick={onOpenAssistant}
                  disabled={!onOpenAssistant}
                  className="rounded-full"
                >
                  <Sparkles data-icon="inline-start" />
                  {!isMobile && <span>与 AI 对话</span>}
                </CanvasToolbarButton>
              </span>
            </TooltipTrigger>
            {!assistantOpen && (
              <TooltipContent side="top">
                {onOpenAssistant ? "与 AI 对话" : "AI 对话暂不可用"}
              </TooltipContent>
            )}
          </Tooltip>
        </CanvasToolbarGroup>
      </footer>
    </div>
  );
}
