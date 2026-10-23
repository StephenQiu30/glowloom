import { Button } from "@/components/ui/button";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import { Settings2 } from "lucide-react";

import {
  VideoSettingsPanel,
  videoModeLabel,
  videoResolutionLabel,
  videoSecondsLabel,
  videoSizeLabel,
} from "@/components/canvas/runtime/components/generation/parameters/video-settings-panel";
import { canvasThemes } from "@/components/canvas/runtime/lib/theme";
import type { AiConfig } from "@/components/canvas/runtime/store/use-config-store";
import { useThemeStore } from "@/components/canvas/runtime/store/use-theme-store";

type CanvasVideoSettingsPopoverProps = {
  config: AiConfig;
  onConfigChange: (key: keyof AiConfig, value: string) => void;
  buttonClassName?: string;
  placement?:
    "topLeft" | "top" | "topRight" | "bottomLeft" | "bottom" | "bottomRight";
};

export function CanvasVideoSettingsPopover({
  config,
  onConfigChange,
  buttonClassName,
  placement = "topLeft",
}: CanvasVideoSettingsPopoverProps) {
  const theme = canvasThemes[useThemeStore((state) => state.theme)];
  return (
    <Popover>
      <PopoverTrigger asChild>
        <Button
          type={"button"}
          variant={"ghost"}
          size="sm"
          className={buttonClassName || "max-w-parameter-menu justify-start"}
        >
          {<Settings2 data-icon="inline-start" />}
          <span className="truncate">
            {videoResolutionLabel(config.vquality)} ·{" "}
            {videoSizeLabel(config.size)} ·{" "}
            {videoSecondsLabel(config.videoSeconds)} ·{" "}
            {videoModeLabel(config.videoMode)}
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
        <VideoSettingsPanel
          config={config}
          onConfigChange={(key, value) => onConfigChange(key, value)}
          theme={theme}
          className="flex flex-col gap-4"
        />
      </PopoverContent>
    </Popover>
  );
}
