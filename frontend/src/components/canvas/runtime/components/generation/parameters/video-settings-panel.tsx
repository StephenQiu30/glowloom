import { Field, FieldGroup, FieldLabel } from "@/components/ui/field";
import { MediaRatioIcon } from "@/components/canvas/runtime/components/generation/parameters/media-ratio-icon";
import {
  InputGroup,
  InputGroupAddon,
  InputGroupInput,
} from "@/components/ui/input-group";
import { Slider } from "@/components/ui/slider";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import { ArrowLeftRight } from "lucide-react";

import { type CanvasTheme } from "@/components/canvas/runtime/lib/theme";
import {
  clampVideoSeconds,
  computeVideoSize,
  inferVideoRatio,
  parseVideoResolution,
  readVideoDimensions,
  VIDEO_SECONDS_MAX,
  VIDEO_SECONDS_MIN,
  videoRatioOptions,
} from "@/components/canvas/runtime/lib/media/media-size";
import { type AiConfig } from "@/components/canvas/runtime/store/use-config-store";

const resolutionOptions = [
  { value: "480", label: "480p" },
  { value: "720", label: "720p" },
  { value: "1080", label: "1080p" },
];
const videoModeOptions = [
  { value: "frames", labelKey: "frames" },
  { value: "reference", labelKey: "reference" },
];

export const videoResolutionOptions = resolutionOptions.map((item) => ({
  value: item.value,
  label: item.label,
}));
export const videoSizeOptions = videoRatioOptions.map((item) => ({
  value: item.value,
  get label() {
    return item.value === "auto" ? "自动" : item.value;
  },
}));
export const videoSecondsRange = {
  min: VIDEO_SECONDS_MIN,
  max: VIDEO_SECONDS_MAX,
};

type VideoSettingsPanelProps = {
  config: AiConfig;
  onConfigChange: (
    key:
      | "vquality"
      | "size"
      | "videoSeconds"
      | "videoGenerateAudio"
      | "videoWatermark"
      | "videoMode",
    value: string,
  ) => void;
  theme: CanvasTheme;
  showTitle?: boolean;
  className?: string;
};

export function VideoSettingsPanel({
  config,
  onConfigChange,
  theme,
  showTitle = true,
  className = "w-picker-panel flex flex-col gap-4 rounded-2xl px-1 py-0.5",
}: VideoSettingsPanelProps) {
  const seconds = Number(clampVideoSeconds(config.videoSeconds || "6"));
  const videoMode = normalizeVideoModeValue(config.videoMode);
  const resolution = parseVideoResolution(config.vquality);
  const selectedRatio = inferVideoRatio(config.size || "auto");
  const dimensions = readVideoDimensions(
    config.size || "auto",
    resolution,
    selectedRatio,
  );
  const applySize = (nextResolution: string, ratio: string) => {
    onConfigChange("vquality", nextResolution);
    onConfigChange("size", computeVideoSize(nextResolution, ratio));
  };
  const selectResolution = (nextResolution: string) => {
    if (selectedRatio === "auto") onConfigChange("vquality", nextResolution);
    else applySize(nextResolution, selectedRatio);
  };

  return (
    <FieldGroup
      className={className}
      style={{ color: theme.node.text }}
      onMouseDown={(event) => event.stopPropagation()}
    >
      {showTitle ? (
        <div className="text-lg font-semibold">{"视频设置"}</div>
      ) : null}
      <Field>
        <FieldLabel>清晰度</FieldLabel>
        <ToggleGroup
          type="single"
          variant="outline"
          spacing={2}
          value={String(resolution)}
          onValueChange={(value) => {
            if (value) selectResolution(value);
          }}
          className="grid w-full grid-cols-4 gap-2.5"
          aria-label="视频清晰度"
        >
          {resolutionOptions.map((item) => (
            <ToggleGroupItem
              key={item.value}
              value={String(item.value)}
              className="min-w-0"
            >
              {item.label}
            </ToggleGroupItem>
          ))}
          <ResolutionInput
            value={resolution}
            theme={theme}
            onChange={selectResolution}
          />
        </ToggleGroup>
      </Field>
      <Field>
        <FieldLabel>尺寸</FieldLabel>
        <div className="grid grid-cols-[1fr_auto_1fr] items-center gap-2.5">
          <DimensionInput
            prefix="W"
            value={dimensions.width}
            disabled={selectedRatio === "auto"}
            theme={theme}
            onChange={(value) =>
              updateDimension("width", value, dimensions, onConfigChange)
            }
          />
          <ArrowLeftRight className="size-4 opacity-45" aria-hidden />
          <DimensionInput
            prefix="H"
            value={dimensions.height}
            disabled={selectedRatio === "auto"}
            theme={theme}
            onChange={(value) =>
              updateDimension("height", value, dimensions, onConfigChange)
            }
          />
        </div>
      </Field>
      <Field>
        <FieldLabel>比例</FieldLabel>
        <ToggleGroup
          type="single"
          variant="outline"
          spacing={2}
          value={String(selectedRatio)}
          onValueChange={(value) => {
            if (value) applySize(resolution, value);
          }}
          className="grid w-full grid-cols-4 gap-2.5"
          aria-label="视频比例"
        >
          {videoRatioOptions.map((item) => (
            <ToggleGroupItem
              key={item.value}
              value={String(item.value)}
              className="h-18 min-w-0 flex-col gap-1.5"
            >
              <MediaRatioIcon width={item.width} height={item.height} />
              <span>{item.value === "auto" ? "自动" : item.value}</span>
            </ToggleGroupItem>
          ))}
        </ToggleGroup>
      </Field>
      <Field>
        <FieldLabel>秒数</FieldLabel>
        <div
          className="flex items-center gap-3"
          onMouseDown={(event) => event.stopPropagation()}
        >
          <Slider
            aria-label="视频时长"
            className="min-w-0 flex-1"
            min={VIDEO_SECONDS_MIN}
            max={VIDEO_SECONDS_MAX}
            step={1}
            value={[seconds]}
            onValueChange={([value]) =>
              ((value) =>
                onConfigChange(
                  "videoSeconds",
                  String(Array.isArray(value) ? value[0] : value),
                ))(value)
            }
          />
          <SecondsInput
            value={seconds}
            theme={theme}
            onCommit={(value) => onConfigChange("videoSeconds", String(value))}
          />
          <span
            className="shrink-0 text-sm"
            style={{ color: theme.node.muted }}
          >
            s
          </span>
        </div>
      </Field>
      <Field>
        <FieldLabel>模式</FieldLabel>
        <ToggleGroup
          type="single"
          variant="outline"
          spacing={2}
          value={String(videoMode)}
          onValueChange={(value) => {
            if (value) onConfigChange("videoMode", value);
          }}
          className="grid w-full grid-cols-2 gap-2.5"
          aria-label="视频模式"
        >
          {videoModeOptions.map((item) => (
            <ToggleGroupItem
              key={item.value}
              value={String(item.value)}
              className="min-w-0"
            >
              {(
                { frames: "首尾帧模式", reference: "全能参考模式" } as Record<
                  string,
                  string
                >
              )[String(item.labelKey)] || String(item.labelKey)}
            </ToggleGroupItem>
          ))}
        </ToggleGroup>
      </Field>
    </FieldGroup>
  );
}

export function videoResolutionLabel(value: string) {
  return `${parseVideoResolution(value)}p`;
}

export function videoSizeLabel(value: string) {
  const ratio = inferVideoRatio(value);
  return ratio === "auto" ? "自适应" : ratio;
}

export function videoSecondsLabel(value: string) {
  if (String(value).trim() === "-1") return "智能";
  return `${value || "6"}s`;
}

export function videoModeLabel(value: string) {
  return (
    (
      { frames: "首尾帧模式", reference: "全能参考模式" } as Record<
        string,
        string
      >
    )[String(normalizeVideoModeValue(value))] ||
    String(normalizeVideoModeValue(value))
  );
}

export function normalizeVideoModeValue(value: string | undefined) {
  return value === "reference" ? "reference" : "frames";
}

export function normalizeVideoSizeValue(value: string, resolution = "720") {
  if (value === "auto") return "auto";
  if (/^\d+x\d+$/.test(value || "")) return value;
  const ratio = inferVideoRatio(value);
  return ratio === "auto" ? "auto" : computeVideoSize(resolution, ratio);
}

export function normalizeVideoResolutionValue(value: string) {
  return parseVideoResolution(value);
}

function updateDimension(
  key: "width" | "height",
  value: number | null,
  dimensions: { width: number; height: number },
  onConfigChange: VideoSettingsPanelProps["onConfigChange"],
) {
  const next = Math.max(1, Math.floor(value || dimensions[key] || 720));
  onConfigChange(
    "size",
    `${key === "width" ? next : dimensions.width}x${key === "height" ? next : dimensions.height}`,
  );
}

function ResolutionInput({
  value,
  onChange,
}: {
  value: string;
  theme: CanvasTheme;
  onChange: (value: string) => void;
}) {
  return (
    <InputGroup>
      <InputGroupInput
        aria-label="自定义清晰度"
        type="number"
        min={1}
        value={value}
        onChange={(event) => onChange(event.target.value)}
        onMouseDown={(event) => event.stopPropagation()}
      />
      <InputGroupAddon align="inline-end">p</InputGroupAddon>
    </InputGroup>
  );
}

function SecondsInput({
  value,
  onCommit,
}: {
  value: number;
  theme: CanvasTheme;
  onCommit: (value: number) => void;
}) {
  const commit = (input: HTMLInputElement) => {
    const next = Number(clampVideoSeconds(input.value));
    input.value = String(next);
    onCommit(next);
  };

  return (
    <InputGroup className="w-20 shrink-0">
      <InputGroupInput
        aria-label="视频秒数"
        type="number"
        min={VIDEO_SECONDS_MIN}
        max={VIDEO_SECONDS_MAX}
        defaultValue={value}
        key={value}
        onBlur={(event) => commit(event.currentTarget)}
        onKeyDown={(event) => {
          if (event.key === "Enter") event.currentTarget.blur();
        }}
        onMouseDown={(event) => event.stopPropagation()}
      />
    </InputGroup>
  );
}

function DimensionInput({
  prefix,
  value,
  disabled,
  onChange,
}: {
  prefix: string;
  value: number;
  disabled: boolean;
  theme: CanvasTheme;
  onChange: (value: number | null) => void;
}) {
  return (
    <InputGroup>
      <InputGroupInput
        aria-label={prefix === "W" ? "视频宽度" : "视频高度"}
        type="number"
        min={1}
        disabled={disabled}
        value={value || ""}
        onChange={(event) => onChange(Number(event.target.value) || null)}
        onMouseDown={(event) => event.stopPropagation()}
      />
      <InputGroupAddon>{prefix}</InputGroupAddon>
    </InputGroup>
  );
}
