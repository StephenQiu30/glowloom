"use client";

import { useId } from "react";
import { cn } from "cn";
import { Field, FieldGroup, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { Checkbox } from "@/components/ui/checkbox";
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
    <FieldGroup
      className={cn("gap-3", !compact && "grid gap-5 sm:grid-cols-2")}
    >
      {fields
        .filter((field) => field.field !== "output_count")
        .map((field) => {
          const fieldID = `${id}-${field.field}`;
          const ratio = field.field === "aspect_ratio";
          const label = compact
            ? ratio
              ? "画面比例"
              : field.field === "resolution"
                ? "清晰度"
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
              <FieldLabel htmlFor={fieldID}>{label}</FieldLabel>
              {field.enum ? (
                <Select
                  disabled={disabled}
                  value={String(params[field.field] ?? "")}
                  onValueChange={(selectedValue) => update(selectedValue)}
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
                          {value}
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
              onChange({ ...params, output_count: Number(event.target.value) })
            }
          />
        )}
      </Field>
    </FieldGroup>
  );
}
