"use client";

import { useId } from "react";
import { cn } from "cn";
import { Field, FieldGroup, FieldLabel } from "@/components/ui/field";
import { Slider } from "@/components/ui/slider";
import { Input } from "@/components/ui/input";
import { Checkbox } from "@/components/ui/checkbox";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import type { ParamField } from "./contracts";
import type { PromptDraft } from "./prompt-document";

export function outputCountLimit(maxOutputs: number) {
  return Number.isFinite(maxOutputs)
    ? Math.max(1, Math.min(8, Math.floor(maxOutputs)))
    : 1;
}

function RatioShape({ value }: { value: string | number }) {
  const match = String(value).match(/^(\d+(?:\.\d+)?):(\d+(?:\.\d+)?)$/);
  if (!match) return null;
  const width = Number(match[1]),
    height = Number(match[2]);
  if (!width || !height) return null;
  return (
    <span
      aria-hidden="true"
      className="composer-ratio-shape"
      style={{
        width: 16 * Math.min(1, width / height),
        height: 16 * Math.min(1, height / width),
      }}
    />
  );
}

/** One schema-driven parameter editor for both the popover and full editor. */
export function GenerationParameters({
  fields,
  params,
  maxOutputs,
  disabled,
  compact,
  includeOutputCount = true,
  onChange,
}: {
  fields: ParamField[];
  params: PromptDraft["params"];
  maxOutputs: number;
  disabled: boolean;
  compact: boolean;
  includeOutputCount?: boolean;
  onChange: (params: PromptDraft["params"]) => void;
}) {
  const id = useId();
  const count = outputCountLimit(maxOutputs);
  return (
    <FieldGroup
      className={cn("gap-3", !compact && "grid gap-5 sm:grid-cols-2")}
    >
      {fields
        .filter((field) => field.field !== "output_count")
        .map((field) => {
          const fieldID = `${id}-${field.field}`;
          const ratio = field.field === "aspect_ratio";
          const duration =
            field.field === "duration_ms" &&
            (field.type === "integer" || field.type === "number");
          const scale = duration ? 1000 : 1;
          const label = duration
            ? "时长（秒）"
            : compact
              ? ratio
                ? "画面比例"
                : field.field === "resolution"
                  ? "清晰度"
                  : field.label
              : field.label;
          const update = (value: string, unit = scale) =>
            onChange({
              ...params,
              [field.field]:
                field.type === "number" || field.type === "integer"
                  ? value === ""
                    ? null
                    : Number(value) * unit
                  : value,
            });
          return (
            <Field key={field.field} data-disabled={disabled}>
              <FieldLabel htmlFor={fieldID}>{label}</FieldLabel>
              {field.field === "speaker" && !field.enum ? (
                <Select
                  disabled={disabled}
                  value={String(params.speaker ?? "")}
                  onValueChange={update}
                >
                  <SelectTrigger
                    id={fieldID}
                    className="w-full max-md:min-h-11"
                  >
                    <SelectValue placeholder="尚未配置音色" />
                  </SelectTrigger>
                  <SelectContent>
                    {typeof field.default === "string" && field.default && (
                      <SelectItem value={field.default}>默认音色</SelectItem>
                    )}
                    {typeof params.speaker === "string" &&
                      params.speaker &&
                      params.speaker !== field.default && (
                        <SelectItem value={params.speaker}>
                          已保存音色
                        </SelectItem>
                      )}
                  </SelectContent>
                </Select>
              ) : field.enum && (ratio || field.field === "resolution") ? (
                <ToggleGroup
                  type="single"
                  id={fieldID}
                  aria-label={label}
                  value={String(params[field.field] ?? "")}
                  disabled={disabled}
                  className={cn(
                    "composer-parameter-options",
                    ratio && "composer-ratio-options",
                  )}
                  onValueChange={(value) => {
                    if (value) update(value, 1);
                  }}
                >
                  {field.enum.map((value) => (
                    <ToggleGroupItem
                      key={String(value)}
                      value={String(value)}
                      aria-label={String(value)}
                    >
                      {ratio && <RatioShape value={value} />}
                      <span>{value}</span>
                    </ToggleGroupItem>
                  ))}
                </ToggleGroup>
              ) : field.enum ? (
                <Select
                  disabled={disabled}
                  value={String(params[field.field] ?? "")}
                  onValueChange={(selectedValue) => update(selectedValue, 1)}
                >
                  <SelectTrigger
                    id={fieldID}
                    className="w-full max-md:min-h-11"
                  >
                    <SelectValue placeholder={"请选择"} />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectGroup>
                      {field.enum.map((value) => (
                        <SelectItem key={String(value)} value={String(value)}>
                          {duration && typeof value === "number"
                            ? value / scale
                            : field.field === "music_input"
                              ? value === "lyrics"
                                ? "歌词"
                                : "描述"
                              : value}
                        </SelectItem>
                      ))}
                    </SelectGroup>
                  </SelectContent>
                </Select>
              ) : field.type === "boolean" ? (
                <Checkbox
                  id={fieldID}
                  disabled={disabled}
                  checked={!!params[field.field]}
                  onCheckedChange={(value) =>
                    onChange({ ...params, [field.field]: value === true })
                  }
                />
              ) : (
                <div className="flex flex-col gap-3">
                  {duration &&
                    field.min !== undefined &&
                    field.max !== undefined &&
                    Number.isFinite(field.min) &&
                    Number.isFinite(field.max) &&
                    field.max > field.min && (
                      <Slider
                        aria-label={label}
                        disabled={disabled}
                        min={field.min / scale}
                        max={field.max / scale}
                        step={(field.step ?? 1000) / scale}
                        value={[
                          typeof params[field.field] === "number"
                            ? Math.max(
                                field.min,
                                Math.min(
                                  field.max,
                                  params[field.field] as number,
                                ),
                              ) / scale
                            : field.min / scale,
                        ]}
                        onValueChange={([value]) => update(String(value))}
                      />
                    )}
                  <Input
                    id={fieldID}
                    disabled={disabled}
                    type={field.type === "string" ? "text" : "number"}
                    value={
                      params[field.field] == null
                        ? ""
                        : duration
                          ? String(Number(params[field.field]) / scale)
                          : String(params[field.field])
                    }
                    min={
                      field.min === undefined ? undefined : field.min / scale
                    }
                    max={
                      field.max === undefined ? undefined : field.max / scale
                    }
                    step={
                      field.step === undefined ? undefined : field.step / scale
                    }
                    onChange={(event) => update(event.target.value)}
                  />
                </div>
              )}
            </Field>
          );
        })}
      {includeOutputCount && (
        <Field data-disabled={disabled}>
          <FieldLabel htmlFor={`${id}-count`}>
            {compact ? "生成数量" : "候选数量"}
          </FieldLabel>
          {compact ? (
            <Select
              disabled={disabled}
              value={String(params.output_count ?? 1)}
              onValueChange={(value) =>
                onChange({ ...params, output_count: Number(value) })
              }
            >
              <SelectTrigger
                id={`${id}-count`}
                className="w-full max-md:min-h-11"
              >
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectGroup>
                  {Array.from({ length: count }, (_, index) => index + 1).map(
                    (value) => (
                      <SelectItem key={value} value={String(value)}>
                        {value}
                      </SelectItem>
                    ),
                  )}
                </SelectGroup>
              </SelectContent>
            </Select>
          ) : (
            <Input
              id={`${id}-count`}
              type="number"
              min={1}
              max={count}
              disabled={disabled}
              value={Number(params.output_count ?? 1)}
              onChange={(event) =>
                onChange({
                  ...params,
                  output_count: Number(event.target.value),
                })
              }
            />
          )}
        </Field>
      )}
    </FieldGroup>
  );
}
