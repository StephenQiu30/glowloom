import { SelectGroup } from "@/components/ui/select";
import { Cpu } from "lucide-react";
import { useEffect, useId, useMemo, useState } from "react";

import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
} from "@/components/ui/select";
import { cn } from "@/lib/utils";
import {
  modelOptionLabel,
  selectableModelsByCapability,
  type AiConfig,
  type ModelCapability,
} from "@/components/canvas/runtime/store/use-config-store";

type ModelPickerProps = {
  id?: string;
  config: AiConfig;
  value?: string;
  onChange: (model: string) => void;
  capability?: ModelCapability;
  className?: string;
  fullWidth?: boolean;
  placeholder?: string;
};

export function ModelPicker({
  id,
  config,
  value,
  onChange,
  capability,
  className,
  fullWidth = false,
  placeholder,
}: ModelPickerProps) {
  const pickerId = useId();
  const [open, setOpen] = useState(false);
  const options = useMemo(
    () =>
      Array.from(
        new Set(
          [
            ...(!capability ? [value] : []),
            ...selectableModelsByCapability(config, capability),
          ].filter((model): model is string => Boolean(model)),
        ),
      ),
    [capability, config, value],
  );
  const current = value || "";
  const pickerPlaceholder = placeholder || "选择模型";

  useEffect(() => {
    const closeOtherPicker = (event: Event) => {
      if ((event as CustomEvent<string>).detail !== pickerId) setOpen(false);
    };
    window.addEventListener("model-picker-open", closeOtherPicker);
    return () =>
      window.removeEventListener("model-picker-open", closeOtherPicker);
  }, [pickerId]);

  return (
    <Select
      open={open}
      value={current}
      onOpenChange={(nextOpen) => {
        if (nextOpen)
          window.dispatchEvent(
            new CustomEvent("model-picker-open", { detail: pickerId }),
          );
        setOpen(nextOpen);
      }}
      onValueChange={onChange}
    >
      <SelectTrigger
        id={id}
        aria-label="生成模型"
        className={cn(
          "canvas-composer-model-picker max-w-full",
          fullWidth
            ? "w-full min-w-0 justify-start"
            : "min-w-model-trigger justify-start",
          className,
        )}
        onMouseDown={(event) => event.stopPropagation()}
        title={current ? modelOptionLabel(config, current) : pickerPlaceholder}
      >
        <Cpu aria-hidden="true" className="size-4 shrink-0 opacity-70" />
        <span className="canvas-model-picker-text min-w-0 flex-1 truncate text-left">
          {current ? modelOptionLabel(config, current) : pickerPlaceholder}
        </span>
      </SelectTrigger>
      <SelectContent
        data-canvas-no-zoom
        className="max-w-viewport-inset w-picker-panel"
        position="popper"
        align="start"
        side="bottom"
        sideOffset={6}
        onMouseDown={(event) => event.stopPropagation()}
      >
        <SelectGroup>
          {options.length ? (
            options.map((model) => (
              <SelectItem
                key={model}
                value={model}
                textValue={modelOptionLabel(config, model)}
              >
                <ModelLabel config={config} model={model} />
              </SelectItem>
            ))
          ) : (
            <SelectItem value="__empty__" disabled>
              {emptyModelLabel(config, capability)}
            </SelectItem>
          )}
        </SelectGroup>
      </SelectContent>
    </Select>
  );
}

function emptyModelLabel(config: AiConfig, capability?: ModelCapability) {
  const label = capability
    ? (
        { image: "生图", video: "视频", text: "文本", audio: "音频" } as Record<
          string,
          string
        >
      )[String(capability)] || String(capability)
    : "";
  return config.models.length ? `暂无可用的${label}模型` : "暂无可用模型";
}

function ModelLabel({ config, model }: { config: AiConfig; model: string }) {
  return (
    <span className="flex min-w-0 items-center gap-2">
      <Cpu aria-hidden="true" className="size-4 shrink-0 opacity-70" />
      <span className="truncate">{modelOptionLabel(config, model)}</span>
    </span>
  );
}
