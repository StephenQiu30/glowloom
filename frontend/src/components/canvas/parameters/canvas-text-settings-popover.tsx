import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import { Settings2 } from "lucide-react";

import {
  reasoningEffortLabel,
  TextSettingsPanel,
} from "@/components/canvas/runtime/components/generation/parameters/text-settings-panel";
import { canvasThemes } from "@/components/canvas/runtime/lib/theme";
import type {
  AiConfig,
  ReasoningEffort,
} from "@/components/canvas/runtime/store/use-config-store";
import { useThemeStore } from "@/components/canvas/runtime/store/use-theme-store";

type CanvasTextSettingsPopoverProps = {
  config: AiConfig;
  onConfigChange: (key: "reasoningEffort", value: ReasoningEffort) => void;
  count?: number;
  onCountChange?: (count: number) => void;
  buttonClassName?: string;
  placement?:
    "topLeft" | "top" | "topRight" | "bottomLeft" | "bottom" | "bottomRight";
};

export function CanvasTextSettingsPopover({
  config,
  onConfigChange,
  count,
  onCountChange,
  buttonClassName,
  placement = "topLeft",
}: CanvasTextSettingsPopoverProps) {
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
            {"推理"} · {reasoningEffortLabel(config.reasoningEffort)}
            {onCountChange ? ` · ${`${count} 次`}` : ""}
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
        <TextSettingsPanel
          config={config}
          onConfigChange={onConfigChange}
          theme={theme}
        />
        {onCountChange ? (
          <div className="mt-4 flex flex-col gap-2.5">
            <div
              className="text-sm font-medium"
              style={{ color: theme.node.muted }}
            >
              {"生成次数"}
            </div>
            <Input
              aria-label="生成次数"
              className="w-full"
              min={1}
              max={15}
              type="number"
              value={count ?? ""}
              onChange={(event) =>
                ((value) => onCountChange(value || 1))(
                  event.target.value === "" ? null : Number(event.target.value),
                )
              }
            />
          </div>
        ) : null}
      </PopoverContent>
    </Popover>
  );
}
