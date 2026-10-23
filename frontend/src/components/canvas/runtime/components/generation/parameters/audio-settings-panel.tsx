import { Field, FieldGroup, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import { useId } from "react";

import {
  audioFormatOptions,
  audioSpeedLabel,
  audioVoiceOptions,
  normalizeAudioFormatValue,
  normalizeAudioSpeedValue,
  normalizeAudioVoiceValue,
} from "@/components/canvas/runtime/lib/generation/audio-options";
import { type CanvasTheme } from "@/components/canvas/runtime/lib/theme";
import type { AiConfig } from "@/components/canvas/runtime/store/use-config-store";

const speedOptions = ["0.75", "1", "1.25", "1.5"];

type AudioSettingKey =
  "audioVoice" | "audioFormat" | "audioSpeed" | "audioInstructions";

type AudioSettingsPanelProps = {
  config: AiConfig;
  onConfigChange: (key: AudioSettingKey, value: string) => void;
  theme: CanvasTheme;
  showTitle?: boolean;
  className?: string;
};

export function AudioSettingsPanel({
  config,
  onConfigChange,
  theme,
  showTitle = true,
  className = "w-picker-panel flex flex-col gap-4 rounded-2xl px-1 py-0.5",
}: AudioSettingsPanelProps) {
  const voice = normalizeAudioVoiceValue(config.audioVoice);
  const speedId = useId();
  const instructionsId = useId();
  const format = normalizeAudioFormatValue(config.audioFormat);
  const speed = normalizeAudioSpeedValue(config.audioSpeed);

  return (
    <FieldGroup
      className={className}
      style={{ color: theme.node.text }}
      onMouseDown={(event) => event.stopPropagation()}
    >
      {showTitle ? (
        <div className="text-lg font-semibold">{"音频设置"}</div>
      ) : null}
      <Field>
        <FieldLabel>声音</FieldLabel>
        <ToggleGroup
          type="single"
          variant="outline"
          spacing={2}
          value={String(voice)}
          onValueChange={(value) => {
            if (value) onConfigChange("audioVoice", value);
          }}
          className="grid w-full grid-cols-3 gap-2.5"
          aria-label="声音"
        >
          {audioVoiceOptions.map((item) => (
            <ToggleGroupItem
              key={item.value}
              value={String(item.value)}
              className="min-w-0"
            >
              {item.label}
            </ToggleGroupItem>
          ))}
        </ToggleGroup>
      </Field>
      <Field>
        <FieldLabel>格式</FieldLabel>
        <ToggleGroup
          type="single"
          variant="outline"
          spacing={2}
          value={String(format)}
          onValueChange={(value) => {
            if (value) onConfigChange("audioFormat", value);
          }}
          className="grid w-full grid-cols-3 gap-2.5"
          aria-label="音频格式"
        >
          {audioFormatOptions.map((item) => (
            <ToggleGroupItem
              key={item.value}
              value={String(item.value)}
              className="min-w-0"
            >
              {item.label}
            </ToggleGroupItem>
          ))}
        </ToggleGroup>
      </Field>
      <Field>
        <FieldLabel htmlFor={speedId}>语速</FieldLabel>
        <ToggleGroup
          type="single"
          variant="outline"
          spacing={2}
          value={String(speed)}
          onValueChange={(value) => {
            if (value) onConfigChange("audioSpeed", value);
          }}
          className="grid w-full grid-cols-4 gap-2.5"
          aria-label="语速"
        >
          {speedOptions.map((value) => (
            <ToggleGroupItem
              key={value}
              value={String(value)}
              className="min-w-0"
            >
              {audioSpeedLabel(value)}
            </ToggleGroupItem>
          ))}
        </ToggleGroup>
        <Input
          id={speedId}
          type="number"
          min={0.25}
          max={4}
          step={0.05}
          value={config.audioSpeed || "1"}
          onChange={(event) => onConfigChange("audioSpeed", event.target.value)}
          onBlur={(event) =>
            onConfigChange(
              "audioSpeed",
              normalizeAudioSpeedValue(event.target.value),
            )
          }
          onMouseDown={(event) => event.stopPropagation()}
        />
      </Field>
      <Field>
        <FieldLabel htmlFor={instructionsId}>声音指令</FieldLabel>
        <Textarea
          id={instructionsId}
          value={config.audioInstructions || ""}
          placeholder={"例如：自然、温暖、适合旁白。"}
          className="thin-scrollbar h-20 resize-none"
          onChange={(event) =>
            onConfigChange("audioInstructions", event.target.value)
          }
          onMouseDown={(event) => event.stopPropagation()}
        />
      </Field>
    </FieldGroup>
  );
}
