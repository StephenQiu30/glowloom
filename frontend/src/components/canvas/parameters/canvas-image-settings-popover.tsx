import { Button } from "@/components/ui/button";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import { Settings2 } from "lucide-react";
import { useState } from "react";

import {
  ImageSettingsPanel,
  imageQualityLabel,
  imageSizeLabel,
} from "@/components/canvas/runtime/components/generation/parameters/image-settings-panel";
import { canvasThemes } from "@/components/canvas/runtime/lib/theme";
import type { AiConfig } from "@/components/canvas/runtime/store/use-config-store";
import { useThemeStore } from "@/components/canvas/runtime/store/use-theme-store";

type CanvasImageSettingsPopoverProps = {
  config: AiConfig;
  onConfigChange: (key: keyof AiConfig, value: string) => void;
  onOpenChange?: (open: boolean) => void;
  buttonClassName?: string;
  getPopupContainer?: (triggerNode: HTMLElement) => HTMLElement;
  placement?:
    "topLeft" | "top" | "topRight" | "bottomLeft" | "bottom" | "bottomRight";
  autoAdjustOverflow?: boolean;
};

export function CanvasImageSettingsPopover({
  config,
  onConfigChange,
  onOpenChange,
  buttonClassName,
  placement = "topLeft",
}: CanvasImageSettingsPopoverProps) {
  const theme = canvasThemes[useThemeStore((state) => state.theme)];
  const [open, setOpen] = useState(false);
  const quality = config.quality || "auto";
  const count = Math.max(
    1,
    Math.min(15, Math.floor(Math.abs(Number(config.count)) || 1)),
  );
  const activeSize = config.size || "auto";
  return (
    <Popover
      open={open}
      onOpenChange={(open) => {
        setOpen(open);
        onOpenChange?.(open);
      }}
    >
      <PopoverTrigger asChild>
        <Button
          type={"button"}
          variant={"ghost"}
          size="sm"
          className={buttonClassName || "max-w-generation-panel justify-start"}
        >
          {<Settings2 data-icon="inline-start" />}
          <span className="truncate">
            {imageQualityLabel(quality)} · {imageSizeLabel(activeSize)} ·{" "}
            {`${count} 张`}
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
        <ImageSettingsPanel
          config={config}
          onConfigChange={(key, value) => onConfigChange(key, value)}
          theme={theme}
          className="flex flex-col gap-4"
        />
      </PopoverContent>
    </Popover>
  );
}
