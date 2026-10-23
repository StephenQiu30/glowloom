import { Spinner } from "@/components/ui/spinner";
import { Button } from "@/components/ui/button";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import {
  Image as ImageIcon,
  MessageSquare,
  Music2,
  Play,
  Settings2,
  Square,
  Video,
} from "lucide-react";
import { Badge } from "@/components/ui/badge";

import { ModelPicker } from "@/components/canvas/runtime/components/generation/model/model-picker";
import { canvasThemes } from "@/components/canvas/runtime/lib/theme";
import {
  defaultConfig,
  resolveModelForCapability,
  useEffectiveConfig,
  type AiConfig,
} from "@/components/canvas/runtime/store/use-config-store";
import { useThemeStore } from "@/components/canvas/runtime/store/use-theme-store";
import type {
  CanvasGenerationMode,
  CanvasNodeData,
  CanvasNodeMetadata,
} from "@/components/canvas/types/canvas";
import {
  CanvasAudioSettingsPopover,
  type CanvasAudioSettingKey,
} from "../parameters/canvas-audio-settings-popover";
import { CanvasImageSettingsPopover } from "../parameters/canvas-image-settings-popover";
import { CanvasTextSettingsPopover } from "../parameters/canvas-text-settings-popover";
import { CanvasVideoSettingsPopover } from "../parameters/canvas-video-settings-popover";

type CanvasConfigNodePanelProps = {
  node: CanvasNodeData;
  isRunning: boolean;
  inputSummary: {
    textCount: number;
    imageCount: number;
    videoCount: number;
    audioCount: number;
  };
  onConfigChange: (nodeId: string, patch: Partial<CanvasNodeMetadata>) => void;
  onGenerate: (nodeId: string) => void;
  onStop: (nodeId: string) => void;
  onComposerToggle: () => void;
};

export function CanvasConfigNodePanel({
  node,
  isRunning,
  inputSummary,
  onConfigChange,
  onGenerate,
  onStop,
  onComposerToggle,
}: CanvasConfigNodePanelProps) {
  const globalConfig = useEffectiveConfig();
  const theme = canvasThemes[useThemeStore((state) => state.theme)];
  const mode = node.metadata?.generationMode || "image";
  const config = buildNodeConfig(globalConfig, node, mode);
  const hasAnyInput = Boolean(
    inputSummary.textCount ||
    inputSummary.imageCount ||
    inputSummary.videoCount ||
    inputSummary.audioCount,
  );
  const hasComposerContent = Boolean(
    (node.metadata?.composerContent ?? node.metadata?.prompt ?? "").trim(),
  );
  const canGenerate =
    hasComposerContent ||
    (mode === "audio" ? inputSummary.textCount > 0 : hasAnyInput);

  return (
    <div
      className="flex h-full w-full cursor-move flex-col px-3 pt-7 pb-3 text-sm"
      style={{ color: theme.node.text }}
      onWheel={(event) => event.stopPropagation()}
    >
      <div className="mb-2 flex items-center justify-between gap-3">
        <div className="shrink-0 text-sm font-semibold">{"生成配置"}</div>
        <div
          className="cursor-default"
          onMouseDown={(event) => event.stopPropagation()}
        >
          <ToggleGroup
            type="single"
            variant="outline"
            className={"canvas-config-mode !rounded-md !p-0.5"}
            value={String(mode)}
            size="sm"
            onValueChange={(value) => {
              if (value)
                ((value) =>
                  onConfigChange(node.id, {
                    generationMode: value as CanvasGenerationMode,
                  }))(value);
            }}
          >
            {[
              {
                value: "image",
                label: (
                  <span className="inline-flex items-center gap-1">
                    <ImageIcon className="size-3.5" />
                    {"生图"}
                  </span>
                ),
              },
              {
                value: "text",
                label: (
                  <span className="inline-flex items-center gap-1">
                    <MessageSquare className="size-3.5" />
                    {"文本"}
                  </span>
                ),
              },
              {
                value: "video",
                label: (
                  <span className="inline-flex items-center gap-1">
                    <Video className="size-3.5" />
                    {"视频"}
                  </span>
                ),
              },
              {
                value: "audio",
                label: (
                  <span className="inline-flex items-center gap-1">
                    <Music2 className="size-3.5" />
                    {"音频"}
                  </span>
                ),
              },
            ].map((item) => {
              const option =
                typeof item === "object" ? item : { value: item, label: item };
              return (
                <ToggleGroupItem
                  key={String(option.value)}
                  value={String(option.value)}
                >
                  {option.label}
                </ToggleGroupItem>
              );
            })}
          </ToggleGroup>
        </div>
      </div>

      <div className="mb-2 flex flex-wrap gap-1.5">
        <InputChip label={"提示词"} value={`${inputSummary.textCount} 个`} />
        <InputChip label={"参考图"} value={`${inputSummary.imageCount} 张`} />
        <InputChip label={"参考视频"} value={`${inputSummary.videoCount} 个`} />
        <InputChip label={"参考音频"} value={`${inputSummary.audioCount} 个`} />
        <Button
          variant="outline"
          type="button"
          className="inline-flex h-7 cursor-pointer items-center gap-1 border px-2"
          onMouseDown={(event) => event.stopPropagation()}
          onClick={onComposerToggle}
        >
          <Settings2 />
          {"组装提示词"}
        </Button>
      </div>

      <div
        className="grid-cols-node-config mb-2 grid min-w-0 cursor-default items-center gap-2"
        onMouseDown={(event) => event.stopPropagation()}
      >
        <ModelPicker
          className="canvas-compact-control h-10"
          config={config}
          value={config.model}
          onChange={(model) => onConfigChange(node.id, { model })}
          capability={mode}
          fullWidth
        />
        {mode === "video" ? (
          <CanvasVideoSettingsPopover
            config={config}
            placement="topRight"
            buttonClassName="canvas-compact-control !h-10 !w-full !justify-start !rounded-lg !px-2"
            onConfigChange={(key, value) =>
              onConfigChange(node.id, videoConfigPatch(key, value))
            }
          />
        ) : mode === "image" ? (
          <CanvasImageSettingsPopover
            config={config}
            placement="topRight"
            autoAdjustOverflow={false}
            buttonClassName="canvas-compact-control !h-10 !w-full !justify-start !rounded-lg !px-2"
            onConfigChange={(key, value) =>
              onConfigChange(
                node.id,
                key === "count"
                  ? { count: Number(value) || 1 }
                  : { [key]: value },
              )
            }
          />
        ) : mode === "audio" ? (
          <CanvasAudioSettingsPopover
            config={config}
            placement="topRight"
            buttonClassName="canvas-compact-control !h-10 !w-full !justify-start !rounded-lg !px-2"
            onConfigChange={(key, value) =>
              onConfigChange(node.id, audioConfigPatch(key, value))
            }
          />
        ) : (
          <CanvasTextSettingsPopover
            config={config}
            count={node.metadata?.textCount || 1}
            placement="topRight"
            buttonClassName="canvas-compact-control !h-10 !w-full !justify-start !rounded-lg !px-2"
            onConfigChange={(_, value) =>
              onConfigChange(node.id, { reasoningEffort: value })
            }
            onCountChange={(textCount) =>
              onConfigChange(node.id, { textCount })
            }
          />
        )}
      </div>

      <Button
        onMouseDown={(event) => event.stopPropagation()}
        onClick={() => (isRunning ? onStop(node.id) : onGenerate(node.id))}
        type={"button"}
        variant={isRunning ? "destructive" : "default"}
        size="default"
        disabled={!isRunning && !canGenerate}
        className={"mt-auto !h-9 !w-full !cursor-pointer"}
      >
        <span className="inline-flex items-center gap-1.5">
          {isRunning ? (
            <>
              <Spinner data-icon="inline-start" aria-hidden="true" />
              <Square className="fill-current" aria-hidden="true" />
              <span>{"停止"}</span>
            </>
          ) : (
            <>
              <Play data-icon="inline-start" aria-hidden="true" />
              <span>{"开始生成"}</span>
            </>
          )}
        </span>
      </Button>
    </div>
  );
}

function InputChip({ label, value }: { label: string; value: string }) {
  return (
    <Badge variant="secondary" className="gap-1">
      <span>{label}</span>
      <span>{value}</span>
    </Badge>
  );
}

function buildNodeConfig(
  globalConfig: AiConfig,
  node: CanvasNodeData,
  mode: CanvasGenerationMode,
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
