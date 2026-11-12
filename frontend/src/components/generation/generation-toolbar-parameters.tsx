"use client";

import type { ReactNode } from "react";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import {
  GenerationParameters,
  outputCountLimit,
} from "./generation-parameters";
import { ComposerToolbarPopover } from "./composer-toolbar";
import type { ParamField } from "./contracts";
import type { PromptDraft } from "./prompt-document";

/** Only separates existing controls; it never invents a model capability or submits. */
export function GenerationToolbarParameters({
  enabled,
  disabled,
  params,
  maxOutputs,
  duration,
  children,
  onChange,
}: {
  enabled: boolean;
  disabled: boolean;
  params: PromptDraft["params"];
  maxOutputs: number;
  duration?: ParamField;
  children?: ReactNode;
  onChange: (params: PromptDraft["params"]) => void;
}) {
  const summary = [params.aspect_ratio, params.resolution]
    .filter((v) => typeof v === "string" && v)
    .join(" · ");
  const durationValue = params.duration_ms ?? duration?.default;
  return (
    <>
      <ComposerToolbarPopover
        label="生成参数"
        value={summary || "参数"}
        disabled={disabled}
      >
        {enabled ? (
          children
        ) : (
          <p className="text-muted-foreground">
            请先选择可用模型，再配置生成参数。
          </p>
        )}
      </ComposerToolbarPopover>
      {enabled && (
        <ComposerToolbarPopover
          label="生成数量"
          value={`${params.output_count ?? 1}个`}
          disabled={disabled}
          width="quantity"
          chevron={false}
        >
          <span className="text-xs text-muted-foreground">选择生成数量</span>
          <ToggleGroup
            type="single"
            value={String(params.output_count ?? 1)}
            disabled={disabled}
            aria-label="生成数量"
            className="composer-parameter-options"
            onValueChange={(value) => {
              if (value) onChange({ ...params, output_count: Number(value) });
            }}
          >
            {Array.from(
              { length: outputCountLimit(maxOutputs) },
              (_, i) => i + 1,
            ).map((value) => (
              <ToggleGroupItem key={value} value={String(value)}>
                {value}
              </ToggleGroupItem>
            ))}
          </ToggleGroup>
        </ComposerToolbarPopover>
      )}
      {enabled && duration && (
        <ComposerToolbarPopover
          label="视频时长"
          value={
            typeof durationValue === "number"
              ? `${durationValue / 1000}s`
              : "时长"
          }
          disabled={disabled}
          width="compact"
          chevron={false}
        >
          <GenerationParameters
            fields={[duration]}
            params={params}
            maxOutputs={maxOutputs}
            disabled={disabled}
            compact
            includeOutputCount={false}
            onChange={onChange}
          />
        </ComposerToolbarPopover>
      )}
    </>
  );
}
