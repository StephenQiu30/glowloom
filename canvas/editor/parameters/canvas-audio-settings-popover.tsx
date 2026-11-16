import { Button } from "@glowloom/canvas-host/components/ui/button";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@glowloom/canvas-host/components/ui/popover";
import { Settings2 } from "lucide-react";

import { AudioSettingsPanel } from "@glowloom/canvas-host/components/canvas/runtime/components/generation/parameters/audio-settings-panel";
import {
  audioFormatLabel,
  audioSpeedLabel,
  audioVoiceLabel,
} from "@glowloom/canvas-host/components/canvas/runtime/lib/generation/audio-options";
import { canvasThemes } from "../runtime/lib/theme.ts";
import type { AiConfig } from "@glowloom/canvas-host/components/canvas/runtime/store/use-config-store";
import { useThemeStore } from "@glowloom/canvas-host/components/canvas/runtime/store/use-theme-store";

export type CanvasAudioSettingKey =
  "audioVoice" | "audioFormat" | "audioSpeed" | "audioInstructions";

type CanvasAudioSettingsPopoverProps = {
  config: AiConfig;
  onConfigChange: (key: CanvasAudioSettingKey, value: string) => void;
  buttonClassName?: string;
  placement?:
    "topLeft" | "top" | "topRight" | "bottomLeft" | "bottom" | "bottomRight";
};

export function CanvasAudioSettingsPopover({
  config,
  onConfigChange,
  buttonClassName,
  placement = "topLeft",
}: CanvasAudioSettingsPopoverProps) {
  const theme = canvasThemes[useThemeStore((state) => state.theme)];
  return (
    <Popover>
      <PopoverTrigger asChild>
        <Button
          type={"button"}
          variant={"ghost"}
          size="sm"
          className={buttonClassName || "max-w-resource-menu justify-start"}
        >
          {<Settings2 data-icon="inline-start" />}
          <span className="truncate">
            {audioVoiceLabel(config.audioVoice)} ·{" "}
            {audioFormatLabel(config.audioFormat)} ·{" "}
            {audioSpeedLabel(config.audioSpeed)}
          </span>
        </Button>
      </PopoverTrigger>
      <PopoverContent
        side={placement.startsWith("top") ? "top" : "bottom"}
        align={
          placement.endsWith("Right")
            ? "end"
            : placement.endsWith("Left")
              ? "start"
              : "center"
        }
        className="w-prompt-menu max-h-[var(--radix-popover-content-available-height)] overflow-y-auto"
        onMouseDown={(event) => event.stopPropagation()}
      >
        <AudioSettingsPanel
          config={config}
          onConfigChange={(key, value) => onConfigChange(key, value)}
          theme={theme}
          className="flex flex-col gap-4"
        />
      </PopoverContent>
    </Popover>
  );
}
