"use client";

import { useId } from "react";
import { cn } from "cn";
import { Field, FieldGroup, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { Checkbox } from "@/components/ui/checkbox";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import {
  NativeSelect,
  NativeSelectOption,
} from "@/components/ui/native-select";
import type { ParamField } from "./contracts";
import type { PromptDraft } from "./prompt-document";

function RatioIcon({ value }: { value: string }) {
  const [width, height] = value.split(":").map(Number);
  if (!(width > 0 && height > 0)) return null;
  const scale = 16 / Math.max(width, height);
  return (
    <span aria-hidden="true" className="grid h-5 place-items-center">
      <span
        className="rounded-[2px] border-[1.5px] border-current"
        style={{ width: width * scale, height: height * scale }}
      />
    </span>
  );
}

const segmentClass =
  "min-w-0 flex-1 rounded-lg px-1 text-xs font-normal text-muted-foreground hover:bg-foreground/5 data-[state=on]:bg-foreground/10 data-[state=on]:text-foreground data-[state=on]:shadow-sm";

/** One schema-driven parameter editor for both the popover and full editor. */
export function GenerationParameters({
  fields,
  params,
  maxOutputs,
  disabled,
  compact,
  onChange,
}: {
  fields: ParamField[];
  params: PromptDraft["params"];
  maxOutputs: number;
  disabled: boolean;
  compact: boolean;
  onChange: (params: PromptDraft["params"]) => void;
}) {
  const id = useId();
  const count = Number.isFinite(maxOutputs)
    ? Math.max(1, Math.min(8, Math.floor(maxOutputs)))
    : 1;
  return (
    <FieldGroup className={cn("gap-5", !compact && "grid sm:grid-cols-2")}>
      {fields
        .filter((field) => field.field !== "output_count")
        .map((field) => {
          const fieldID = `${id}-${field.field}`;
          const ratio = field.field === "aspect_ratio";
          const label = compact
            ? ratio
              ? "比例"
              : field.field === "resolution"
                ? "分辨率"
                : field.label
            : field.label;
          const update = (value: string) =>
            onChange({
              ...params,
              [field.field]:
                field.type === "number" || field.type === "integer"
                  ? value === ""
                    ? null
                    : Number(value)
                  : value,
            });
          return (
            <Field key={field.field} data-disabled={disabled}>
              <FieldLabel
                htmlFor={fieldID}
                className={cn(
                  compact && "text-xs font-normal text-muted-foreground",
                )}
              >
                {label}
              </FieldLabel>
              {field.enum && compact && field.enum.length <= 8 ? (
                <ToggleGroup
                  id={fieldID}
                  aria-label={label}
                  type="single"
                  value={String(params[field.field] ?? "")}
                  disabled={disabled}
                  spacing={1}
                  className="w-full flex-wrap rounded-xl bg-foreground/5 p-0.5"
                  onValueChange={(value) => {
                    if (value) update(value);
                  }}
                >
                  {field.enum.map((value) => (
                    <ToggleGroupItem
                      key={String(value)}
                      value={String(value)}
                      aria-label={String(value)}
                      className={cn(
                        segmentClass,
                        ratio ? "h-14 flex-col gap-1" : "h-9",
                      )}
                    >
                      {ratio && <RatioIcon value={String(value)} />}
                      {value}
                    </ToggleGroupItem>
                  ))}
                </ToggleGroup>
              ) : field.enum ? (
                <NativeSelect
                  id={fieldID}
                  disabled={disabled}
                  value={String(params[field.field] ?? "")}
                  onChange={(event) => update(event.target.value)}
                >
                  <NativeSelectOption value="">请选择</NativeSelectOption>
                  {field.enum.map((value) => (
                    <NativeSelectOption key={String(value)} value={value}>
                      {value}
                    </NativeSelectOption>
                  ))}
                </NativeSelect>
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
                <Input
                  id={fieldID}
                  disabled={disabled}
                  type={field.type === "string" ? "text" : "number"}
                  value={String(params[field.field] ?? "")}
                  min={field.min}
                  max={field.max}
                  step={field.step}
                  onChange={(event) => update(event.target.value)}
                />
              )}
            </Field>
          );
        })}
      <Field data-disabled={disabled}>
        <FieldLabel
          htmlFor={`${id}-count`}
          className={cn(compact && "text-xs font-normal text-muted-foreground")}
        >
          {compact ? "数量" : "候选数量"}
        </FieldLabel>
        {compact ? (
          <ToggleGroup
            id={`${id}-count`}
            aria-label="数量"
            type="single"
            disabled={disabled}
            value={String(params.output_count ?? 1)}
            spacing={1}
            className="w-full flex-wrap rounded-xl bg-foreground/5 p-0.5"
            onValueChange={(value) => {
              if (value) onChange({ ...params, output_count: Number(value) });
            }}
          >
            {Array.from({ length: count }, (_, index) => index + 1).map(
              (value) => (
                <ToggleGroupItem
                  key={value}
                  value={String(value)}
                  className={cn(segmentClass, "h-9")}
                >
                  {value}
                </ToggleGroupItem>
              ),
            )}
          </ToggleGroup>
        ) : (
          <Input
            id={`${id}-count`}
            type="number"
            min={1}
            max={count}
            disabled={disabled}
            value={Number(params.output_count ?? 1)}
            onChange={(event) =>
              onChange({ ...params, output_count: Number(event.target.value) })
            }
          />
        )}
      </Field>
    </FieldGroup>
  );
}
