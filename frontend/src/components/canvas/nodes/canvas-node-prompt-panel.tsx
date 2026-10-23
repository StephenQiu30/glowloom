import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { ArrowUp, Maximize2, Square, X } from "lucide-react";
import { useEffect, useState } from "react";

import { ModelPicker } from "@/components/canvas/runtime/components/generation/model/model-picker";
import { canvasThemes } from "@/components/canvas/runtime/lib/theme";
import type { CanvasResourceReference } from "@/components/canvas/lib/canvas-resource-references";
import {
  defaultConfig,
  resolveModelForCapability,
  useEffectiveConfig,
  type AiConfig,
} from "@/components/canvas/runtime/store/use-config-store";
import { useThemeStore } from "@/components/canvas/runtime/store/use-theme-store";
import {
  CanvasNodeType,
  type CanvasGenerationMode,
  type CanvasNodeData,
} from "@/components/canvas/types/canvas";
import {
  CanvasAudioSettingsPopover,
  type CanvasAudioSettingKey,
} from "../parameters/canvas-audio-settings-popover";
import { CanvasImageSettingsPopover } from "../parameters/canvas-image-settings-popover";
import { CanvasNodeReferenceBar } from "./canvas-node-reference-bar";
import { CanvasPromptChipInput } from "./canvas-prompt-chip-input";
import { CanvasTextSettingsPopover } from "../parameters/canvas-text-settings-popover";
import { CanvasVideoSettingsPopover } from "../parameters/canvas-video-settings-popover";

export type CanvasNodeGenerationMode = CanvasGenerationMode;

type CanvasNodePromptPanelProps = {
  node: CanvasNodeData;
  isRunning: boolean;
  onPromptChange: (nodeId: string, prompt: string) => void;
  onConfigChange: (
    nodeId: string,
    patch: Partial<CanvasNodeData["metadata"]>,
  ) => void;
  onGenerate: (
    nodeId: string,
    mode: CanvasNodeGenerationMode,
    prompt: string,
  ) => void;
  onStop: (nodeId: string) => void;
  onClose?: () => void;
  mentionReferences?: CanvasResourceReference[];
  nodes: CanvasNodeData[];
  connectedNodes?: CanvasNodeData[];
  onDisconnectReference?: (fromNodeId: string, toNodeId: string) => void;
  onStartReferenceSelection?: (nodeId: string) => void;
  onImageSettingsOpenChange?: (open: boolean) => void;
  modeOverride?: CanvasNodeGenerationMode; // Plugin nodes set their generation type through useBuiltinPanel.mode.
};

export function CanvasNodePromptPanel({
  node,
  nodes,
  isRunning,
  onPromptChange,
  onConfigChange,
  onGenerate,
  onStop,
  onClose,
  mentionReferences = [],
  connectedNodes = [],
  onDisconnectReference,
  onStartReferenceSelection,
  onImageSettingsOpenChange,
  modeOverride,
}: CanvasNodePromptPanelProps) {
  const globalConfig = useEffectiveConfig();
  const theme = canvasThemes[useThemeStore((state) => state.theme)];
  const mode = modeOverride ?? defaultMode(node.type);
  const config = buildNodeConfig(globalConfig, node, mode);
  const hasTextContent =
    node.type === CanvasNodeType.Text &&
    Boolean(node.metadata?.content?.trim());
  const hasImageContent =
    node.type === CanvasNodeType.Image && Boolean(node.metadata?.content);
  const isEditingExistingContent = hasTextContent || hasImageContent;
  const promptPlaceholder =
    mode === "image"
      ? hasImageContent
        ? "请输入你想要把这张图修改成什么"
        : "可直接文字生图，或上传图片输入文字指令进行编辑"
      : mode === "text"
        ? hasTextContent
          ? "请输入你想要将本段文本修改成什么"
          : "输入故事想法、人物关系和剧情走向"
        : mode === "video"
          ? "描述要生成的视频内容"
          : "描述要生成的音频内容";
  const [prompt, setPrompt] = useState(
    node.metadata?.composerContent ?? node.metadata?.prompt ?? "",
  );
  const [expanded, setExpanded] = useState(false);

  // Restore prompts only when switching nodes; preserve the current input after generation on the same node.
  useEffect(() => {
    // 选中的节点或编辑弹窗改变时重置本地草稿，避免沿用上一张素材的输入。
    // eslint-disable-next-line react-hooks/set-state-in-effect
    setPrompt(node.metadata?.composerContent ?? node.metadata?.prompt ?? "");
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [node.id]);

  const updatePrompt = (value: string) => {
    setPrompt(value);
    if (isEditingExistingContent)
      onConfigChange(node.id, { composerContent: value });
    else onPromptChange(node.id, value);
  };

  const submit = () => {
    const text = prompt.trim();
    if (!text || isRunning) return;
    onGenerate(node.id, mode, text);
  };

  const openExpandedEditor = () => {
    setExpanded(true);
  };

  return (
    <div
      data-canvas-no-zoom
      className="canvas-prompt-panel"
      style={{
        background: theme.toolbar.panel,
        borderColor: theme.toolbar.border,
        color: theme.node.text,
      }}
      onMouseDown={(event) => event.stopPropagation()}
      onWheel={(event) => event.stopPropagation()}
    >
      <div className="canvas-prompt-header">
        <CanvasNodeReferenceBar
          nodeId={node.id}
          nodes={nodes}
          connectedNodes={connectedNodes}
          onDisconnect={onDisconnectReference}
          onStartSelection={onStartReferenceSelection}
        />
        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              onClick={openExpandedEditor}
              aria-label={"放大编辑"}
              type={"button"}
              variant={"ghost"}
              size="icon-sm"
              className="shrink-0"
            >
              {<Maximize2 data-icon="inline-start" />}
            </Button>
          </TooltipTrigger>
          <TooltipContent side="top">{"放大编辑"}</TooltipContent>
        </Tooltip>
        {onClose ? (
          <Button
            variant="ghost"
            size="icon-sm"
            className="canvas-composer-close"
            onClick={onClose}
            aria-label="关闭节点参数"
          >
            <X />
          </Button>
        ) : null}
      </div>
      <CanvasPromptChipInput
        value={prompt}
        references={mentionReferences}
        onChange={updatePrompt}
        onSubmit={submit}
        className="canvas-prompt-input w-full cursor-text"
        placeholder={promptPlaceholder}
      />

      <div className="canvas-prompt-controls">
        <div className="canvas-prompt-parameters">
          {mode === "image" ? (
            <>
              <ModelPicker
                config={config}
                value={config.model}
                onChange={(model) => onConfigChange(node.id, { model })}
                capability="image"
                className="max-w-media-panel"
              />
              <CanvasImageSettingsPopover
                config={config}
                placement="topLeft"
                buttonClassName="max-w-resource-menu justify-start"
                onConfigChange={(key, value) =>
                  onConfigChange(
                    node.id,
                    key === "count"
                      ? { count: Number(value) || 1 }
                      : { [key]: value },
                  )
                }
                onOpenChange={onImageSettingsOpenChange}
              />
            </>
          ) : mode === "video" ? (
            <>
              <ModelPicker
                config={config}
                value={config.model}
                onChange={(model) => onConfigChange(node.id, { model })}
                capability="video"
                className="max-w-media-panel"
              />
              <CanvasVideoSettingsPopover
                config={config}
                buttonClassName="max-w-parameter-menu justify-start"
                onConfigChange={(key, value) =>
                  onConfigChange(node.id, videoConfigPatch(key, value))
                }
              />
            </>
          ) : mode === "audio" ? (
            <>
              <ModelPicker
                config={config}
                value={config.model}
                onChange={(model) => onConfigChange(node.id, { model })}
                capability="audio"
                className="max-w-media-panel"
              />
              <CanvasAudioSettingsPopover
                config={config}
                buttonClassName="max-w-resource-menu justify-start"
                onConfigChange={(key, value) =>
                  onConfigChange(node.id, audioConfigPatch(key, value))
                }
              />
            </>
          ) : (
            <>
              <ModelPicker
                config={config}
                value={config.model}
                onChange={(model) => onConfigChange(node.id, { model })}
                capability="text"
                className="max-w-media-panel"
              />
              <CanvasTextSettingsPopover
                config={config}
                count={node.metadata?.textCount || 1}
                onConfigChange={(_, value) =>
                  onConfigChange(node.id, { reasoningEffort: value })
                }
                onCountChange={(textCount) =>
                  onConfigChange(node.id, { textCount })
                }
              />
            </>
          )}
        </div>

        <Button
          onClick={() => (isRunning ? onStop(node.id) : submit())}
          aria-label={
            isRunning
              ? "停止生成"
              : `生成${mode === "image" ? "图片" : mode === "video" ? "视频" : mode === "text" ? "剧本" : "音频"}`
          }
          type={"button"}
          variant={isRunning ? "destructive" : "default"}
          size="icon-lg"
          disabled={!isRunning && !prompt.trim()}
          className="canvas-prompt-submit"
        >
          {isRunning ? (
            <Square className="fill-current" aria-hidden="true" />
          ) : (
            <ArrowUp aria-hidden="true" />
          )}
        </Button>
      </div>
      <Dialog
        open={expanded}
        onOpenChange={(open) => {
          if (!open) (() => setExpanded(false))();
        }}
      >
        <DialogContent
          className={"dialog-settings max-h-dialog w-full overflow-y-auto"}
        >
          <DialogHeader>
            <DialogTitle>{"编辑提示词"}</DialogTitle>
            <DialogDescription className="sr-only">
              查看并编辑当前节点的提示词内容。
            </DialogDescription>
          </DialogHeader>
          <div>
            <div
              data-canvas-no-zoom
              className="pt-2"
              onWheelCapture={(event) => event.stopPropagation()}
            >
              <CanvasNodeReferenceBar
                nodeId={node.id}
                nodes={nodes}
                connectedNodes={connectedNodes}
                onDisconnect={onDisconnectReference}
                onStartSelection={(nodeId) => {
                  setExpanded(false);
                  onStartReferenceSelection?.(nodeId);
                }}
              />
              <CanvasPromptChipInput
                value={prompt}
                references={mentionReferences}
                onChange={updatePrompt}
                className="canvas-prompt-input canvas-prompt-expanded w-full cursor-text"
                style={{
                  background: "transparent",
                  borderColor: theme.toolbar.border,
                  color: theme.node.text,
                }}
                placeholder={promptPlaceholder}
              />
            </div>
          </div>
        </DialogContent>
      </Dialog>
    </div>
  );
}

function defaultMode(type: CanvasNodeData["type"]): CanvasNodeGenerationMode {
  return type === CanvasNodeType.Text
    ? "text"
    : type === CanvasNodeType.Video
      ? "video"
      : type === CanvasNodeType.Audio
        ? "audio"
        : "image";
}

function buildNodeConfig(
  globalConfig: AiConfig,
  node: CanvasNodeData,
  mode: CanvasNodeGenerationMode,
): AiConfig {
  return {
    ...globalConfig,
    model: resolveModelForCapability(globalConfig, node.metadata?.model, mode),
    reasoningEffort:
      node.metadata?.reasoningEffort ||
      globalConfig.reasoningEffort ||
      defaultConfig.reasoningEffort,
    quality:
      node.metadata?.quality || globalConfig.quality || defaultConfig.quality,
    size: node.metadata?.size || globalConfig.size || defaultConfig.size,
    background:
      node.metadata?.background ??
      globalConfig.background ??
      defaultConfig.background,
    videoSeconds:
      node.metadata?.seconds ||
      globalConfig.videoSeconds ||
      defaultConfig.videoSeconds,
    vquality:
      node.metadata?.vquality ||
      globalConfig.vquality ||
      defaultConfig.vquality,
    videoGenerateAudio:
      node.metadata?.generateAudio ||
      globalConfig.videoGenerateAudio ||
      defaultConfig.videoGenerateAudio,
    videoWatermark:
      node.metadata?.watermark ||
      globalConfig.videoWatermark ||
      defaultConfig.videoWatermark,
    videoMode:
      node.metadata?.videoMode ||
      globalConfig.videoMode ||
      defaultConfig.videoMode,
    audioVoice:
      node.metadata?.audioVoice ||
      globalConfig.audioVoice ||
      defaultConfig.audioVoice,
    audioFormat:
      node.metadata?.audioFormat ||
      globalConfig.audioFormat ||
      defaultConfig.audioFormat,
    audioSpeed:
      node.metadata?.audioSpeed ||
      globalConfig.audioSpeed ||
      defaultConfig.audioSpeed,
    audioInstructions:
      node.metadata?.audioInstructions ||
      globalConfig.audioInstructions ||
      defaultConfig.audioInstructions,
    count: String(
      node.metadata?.count ||
        (mode === "image"
          ? globalConfig.canvasImageCount || globalConfig.count
          : globalConfig.count) ||
        defaultConfig.count,
    ),
  };
}

function videoConfigPatch(key: keyof AiConfig, value: string) {
  if (key === "videoSeconds") return { seconds: value };
  if (key === "videoGenerateAudio") return { generateAudio: value };
  if (key === "videoWatermark") return { watermark: value };
  if (key === "videoMode") return { videoMode: value };
  return { [key]: value };
}

function audioConfigPatch(key: CanvasAudioSettingKey, value: string) {
  if (key === "audioVoice") return { audioVoice: value };
  if (key === "audioFormat") return { audioFormat: value };
  if (key === "audioSpeed") return { audioSpeed: value };
  return { audioInstructions: value };
}
