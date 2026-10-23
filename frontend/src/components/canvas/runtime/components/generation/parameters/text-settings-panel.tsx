import { Field, FieldGroup, FieldLabel } from "@/components/ui/field";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";

import { type CanvasTheme } from "@/components/canvas/runtime/lib/theme";
import type {
  AiConfig,
  ReasoningEffort,
} from "@/components/canvas/runtime/store/use-config-store";

const reasoningEffortOptions: ReasoningEffort[] = [
  "auto",
  "low",
  "medium",
  "high",
  "xhigh",
];

type TextSettingsPanelProps = {
  config: AiConfig;
  onConfigChange: (key: "reasoningEffort", value: ReasoningEffort) => void;
  theme: CanvasTheme;
  className?: string;
};

export function TextSettingsPanel({
  config,
  onConfigChange,
  theme,
  className = "flex flex-col gap-4",
}: TextSettingsPanelProps) {
  return (
    <FieldGroup
      className={className}
      style={{ color: theme.node.text }}
      onMouseDown={(event) => event.stopPropagation()}
    >
      <div className="text-lg font-semibold">{"文本设置"}</div>
      <Field>
        <FieldLabel>{"推理强度"}</FieldLabel>
        <ToggleGroup
          type="single"
          variant="outline"
          spacing={2}
          value={String(config.reasoningEffort)}
          onValueChange={(value) => {
            if (value)
              onConfigChange("reasoningEffort", value as ReasoningEffort);
          }}
          className="grid w-full grid-cols-5 gap-2"
          aria-label="推理强度"
        >
          {reasoningEffortOptions.map((value) => (
            <ToggleGroupItem
              key={value}
              value={String(value)}
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
              )[String(value)] || String(value)}
            </ToggleGroupItem>
          ))}
        </ToggleGroup>
      </Field>
    </FieldGroup>
  );
}

export function reasoningEffortLabel(value: ReasoningEffort) {
  return reasoningEffortOptions.includes(value)
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
