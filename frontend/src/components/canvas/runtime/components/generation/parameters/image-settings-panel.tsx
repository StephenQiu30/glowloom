import { Field, FieldGroup, FieldLabel } from "@/components/ui/field";
import { MediaRatioIcon } from "@/components/canvas/runtime/components/generation/parameters/media-ratio-icon";
import {
  InputGroup,
  InputGroupAddon,
  InputGroupInput,
} from "@/components/ui/input-group";
import { Switch } from "@/components/ui/switch";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import { ArrowLeftRight } from "lucide-react";
import { useId, useState } from "react";

import { type CanvasTheme } from "@/components/canvas/runtime/lib/theme";
import {
  computeMediaSize,
  inferMediaRatio,
  inferMediaScale,
  mediaRatioOptions,
  mediaScaleOptions,
  readMediaDimensions,
} from "@/components/canvas/runtime/lib/media/media-size";
import type { AiConfig } from "@/components/canvas/runtime/store/use-config-store";

const qualityOptions = [
  { value: "auto", labelKey: "auto" },
  { value: "high", labelKey: "high" },
  { value: "medium", labelKey: "medium" },
  { value: "low", labelKey: "low" },
];
const DIMENSION_STEP = 16;

export const imageQualityOptions = qualityOptions.map((item) => ({
  value: item.value,
  get label() {
    return (
      (
        {
          auto: "自动",
          low: "低",
          medium: "中",
          high: "高",
          xhigh: "极高",
        } as Record<string, string>
      )[String(item.labelKey)] || String(item.labelKey)
    );
  },
}));
export const imageAspectOptions = mediaRatioOptions.map((item) => ({
  value: item.value,
  label: item.value === "auto" ? "自动" : item.value,
}));
export const imageScaleOptions = mediaScaleOptions.map((value) => ({
  value,
  label: value === "auto" ? "自动" : value,
}));

type ImageSettingsPanelProps = {
  config: AiConfig;
  onConfigChange: (
    key: "quality" | "size" | "count" | "background",
    value: string,
  ) => void;
  theme: CanvasTheme;
  showTitle?: boolean;
  className?: string;
  maxCount?: number;
  quickCount?: number;
};

export function ImageSettingsPanel({
  config,
  onConfigChange,
  theme,
  showTitle = true,
  className = "w-picker-panel flex flex-col gap-4 rounded-2xl px-1 py-0.5",
  maxCount = 15,
  quickCount = 10,
}: ImageSettingsPanelProps) {
  const backgroundId = useId();
  const alignmentId = useId();
  const [snapDimensionToStep, setSnapDimensionToStep] = useState(true);
  const quality = config.quality || "auto";
  const count = Math.max(
    1,
    Math.min(maxCount, Math.floor(Math.abs(Number(config.count)) || 1)),
  );
  const activeSize = config.size || "auto";
  const transparentBackground = config.background === "transparent";
  const selectedScale = inferMediaScale(activeSize);
  const selectedRatio = inferMediaRatio(activeSize);
  const dimensions = readMediaDimensions(
    activeSize,
    selectedScale,
    selectedRatio,
  );
  const applySize = (scale: string, ratio: string) =>
    onConfigChange("size", computeMediaSize(scale, ratio));
  const selectScale = (scale: string) =>
    applySize(scale, selectedRatio === "auto" ? "1:1" : selectedRatio);
  const selectRatio = (ratio: string) => applySize(selectedScale, ratio);
  const updateDimension = (key: "width" | "height", value: number | null) => {
    const next = Math.max(1, Math.floor(value || dimensions[key] || 1024));
    const width = key === "width" ? next : dimensions.width;
    const height = key === "height" ? next : dimensions.height;
    onConfigChange(
      "size",
      `${alignDimension(width, snapDimensionToStep)}x${alignDimension(height, snapDimensionToStep)}`,
    );
  };

  return (
    <FieldGroup
      className={className}
      style={{ color: theme.node.text }}
      onMouseDown={(event) => {
        event.stopPropagation();
        if (event.target instanceof HTMLInputElement) return;
        if (
          document.activeElement instanceof HTMLInputElement &&
          event.currentTarget.contains(document.activeElement)
        )
          document.activeElement.blur();
      }}
    >
      {showTitle ? (
        <div className="text-lg font-semibold">{"图像设置"}</div>
      ) : null}
      <Field>
        <FieldLabel>{"质量"}</FieldLabel>
        <ToggleGroup
          type="single"
          variant="outline"
          spacing={2}
          value={String(quality)}
          onValueChange={(value) => {
            if (value) onConfigChange("quality", value);
          }}
          className="grid w-full grid-cols-4 gap-2.5"
          aria-label="图像质量"
        >
          {qualityOptions.map((item) => (
            <ToggleGroupItem
              key={item.value}
              value={String(item.value)}
              className="min-w-0"
            >
              {(
                {
                  auto: "自动",
                  low: "低",
                  medium: "中",
                  high: "高",
                  xhigh: "极高",
                } as Record<string, string>
              )[String(item.labelKey)] || String(item.labelKey)}
            </ToggleGroupItem>
          ))}
        </ToggleGroup>
      </Field>
      <Field>
        <div className="flex items-center justify-between gap-3">
          <FieldLabel>{"尺寸"}</FieldLabel>
          <div className="flex items-center gap-2">
            <FieldLabel htmlFor={alignmentId}>{"16 倍数对齐"}</FieldLabel>
            <span
              title={"输入完成后自动向上补成 16 的倍数"}
              onMouseDown={(event) => event.stopPropagation()}
            >
              <Switch
                id={alignmentId}
                aria-label="16 倍数对齐"
                checked={snapDimensionToStep}
                onCheckedChange={setSnapDimensionToStep}
              />
            </span>
          </div>
        </div>
        <div className="grid grid-cols-[1fr_auto_1fr] items-center gap-2.5">
          <DimensionInput
            prefix="W"
            value={dimensions.width}
            disabled={selectedRatio === "auto"}
            theme={theme}
            alignToStep={snapDimensionToStep}
            onChange={(value) => updateDimension("width", value)}
          />
          <ArrowLeftRight className="size-4 opacity-45" aria-hidden />
          <DimensionInput
            prefix="H"
            value={dimensions.height}
            disabled={selectedRatio === "auto"}
            theme={theme}
            alignToStep={snapDimensionToStep}
            onChange={(value) => updateDimension("height", value)}
          />
        </div>
      </Field>
      <Field>
        <FieldLabel>{"分辨率"}</FieldLabel>
        <ToggleGroup
          type="single"
          variant="outline"
          spacing={2}
          value={String(selectedScale)}
          onValueChange={(value) => {
            if (value) selectScale(value);
          }}
          className="grid w-full grid-cols-4 gap-2.5"
          aria-label="图像分辨率"
        >
          {mediaScaleOptions.map((value) => (
            <ToggleGroupItem
              key={value}
              value={String(value)}
              className="min-w-0"
            >
              {value === "auto" ? "自动" : value}
            </ToggleGroupItem>
          ))}
        </ToggleGroup>
      </Field>
      <Field>
        <FieldLabel>{"宽高比"}</FieldLabel>
        <ToggleGroup
          type="single"
          variant="outline"
          spacing={2}
          value={String(selectedRatio)}
          onValueChange={(value) => {
            if (value) selectRatio(value);
          }}
          className="grid w-full grid-cols-4 gap-2.5"
          aria-label="图像宽高比"
        >
          {mediaRatioOptions.map((item) => (
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
      <div className="flex items-center justify-between gap-3">
        <div className="flex flex-col gap-0.5">
          <FieldLabel htmlFor={backgroundId}>{"透明背景"}</FieldLabel>
          <div
            className="text-xs"
            style={{ color: theme.node.muted, opacity: 0.75 }}
          >
            {"开启后生成无背景的透明图像（仅部分模型可用）"}
          </div>
        </div>
        <span onMouseDown={(event) => event.stopPropagation()}>
          <Switch
            id={backgroundId}
            aria-label="透明背景"
            checked={transparentBackground}
            onCheckedChange={(checked) =>
              onConfigChange("background", checked ? "transparent" : "")
            }
          />
        </span>
      </div>
      <Field>
        <FieldLabel>{"生成张数"}</FieldLabel>
        <ToggleGroup
          type="single"
          variant="outline"
          spacing={2}
          value={String(count)}
          onValueChange={(value) => {
            if (value) onConfigChange("count", value);
          }}
          className="grid w-full grid-cols-4 gap-2.5"
          aria-label="生成张数"
        >
          {Array.from({ length: quickCount }, (_, index) => index + 1).map(
            (value) => (
              <ToggleGroupItem
                key={value}
                value={String(value)}
                className="min-w-0"
              >
                {`${value} 张`}
              </ToggleGroupItem>
            ),
          )}
          <CountInput
            value={count}
            max={maxCount}
            theme={theme}
            onChange={(value) => onConfigChange("count", String(value || 1))}
          />
        </ToggleGroup>
      </Field>
    </FieldGroup>
  );
}

export function imageQualityLabel(value: string) {
  return ["auto", "high", "medium", "low"].includes(value)
    ? (
        {
          auto: "自动",
          low: "低",
          medium: "中",
          high: "高",
          xhigh: "极高",
        } as Record<string, string>
      )[String(value)] || String(value)
    : value;
}

export function imageSizeLabel(size: string) {
  const scale = inferMediaScale(size);
  const ratio = inferMediaRatio(size);
  if (ratio === "auto" || size === "auto") return "自动";
  if (scale === "auto") return ratio;
  return `${scale} · ${ratio}`;
}

function DimensionInput({
  prefix,
  value,
  disabled,
  alignToStep,
  onChange,
}: {
  prefix: string;
  value: number;
  disabled: boolean;
  theme: CanvasTheme;
  alignToStep: boolean;
  onChange: (value: number | null) => void;
}) {
  const commit = (input: HTMLInputElement) => {
    const next = alignDimension(
      Math.max(1, Math.floor(Number(input.value) || value || 1024)),
      alignToStep,
    );
    input.value = String(next);
    onChange(next);
  };

  return (
    <InputGroup>
      <InputGroupInput
        aria-label={prefix === "W" ? "图像宽度" : "图像高度"}
        type="number"
        min={1}
        disabled={disabled}
        defaultValue={value || ""}
        key={`${prefix}-${value}`}
        onBlur={(event) => commit(event.currentTarget)}
        onKeyDown={(event) => {
          if (event.key === "Enter") event.currentTarget.blur();
        }}
        onMouseDown={(event) => event.stopPropagation()}
      />
      <InputGroupAddon>{prefix}</InputGroupAddon>
    </InputGroup>
  );
}

function CountInput({
  value,
  max,
  onChange,
}: {
  value: number;
  max: number;
  theme: CanvasTheme;
  onChange: (value: number | null) => void;
}) {
  return (
    <InputGroup>
      <InputGroupInput
        aria-label="自定义生成张数"
        type="number"
        min={1}
        max={max}
        value={value || ""}
        onChange={(event) => onChange(Number(event.target.value) || null)}
        onMouseDown={(event) => event.stopPropagation()}
      />
    </InputGroup>
  );
}

function alignDimension(value: number, enabled: boolean) {
  return enabled ? Math.ceil(value / DIMENSION_STEP) * DIMENSION_STEP : value;
}
