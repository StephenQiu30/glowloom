"use client";

import { useId } from "react";
import { zodResolver } from "@hookform/resolvers/zod";
import { Controller, useForm } from "react-hook-form";
import { z } from "zod";

import { Button } from "@/components/ui/button";
import {
  Field,
  FieldDescription,
  FieldError,
  FieldLabel,
  FieldTitle,
} from "@/components/ui/field";

type ParamValue = string | number | boolean;
type ParamValues = Record<string, ParamValue>;
type ParamType = "string" | "integer" | "number" | "boolean";
type ParamComponent =
  "input" | "textarea" | "select" | "slider" | "switch" | "segmented" | "voice";

export type ParamField = {
  field: string;
  label: string;
  type: ParamType;
  component: ParamComponent;
  default?: ParamValue;
  enum?: (string | number)[];
  for_modes?: string[];
  required?: boolean;
  min?: number;
  max?: number;
  step?: number;
  description?: string;
};

type ModelParamsFormProps = {
  modelKey: string;
  profileVersionId: string;
  mode: string;
  schema: ParamField[];
  onSubmit: (values: ParamValues) => void;
};

function optionLabel(field: ParamField, value: string | number) {
  if (field.field === "duration_ms" && typeof value === "number") {
    return `${value / 1000} 秒`;
  }
  return String(value);
}

function valueSchema(field: ParamField): z.ZodType<unknown> {
  const label = field.label;
  let schema: z.ZodType<unknown>;
  if (field.type === "boolean") {
    schema = z.boolean({ error: `请选择${label}` });
  } else if (field.type === "integer" || field.type === "number") {
    let numberSchema = z.number({ error: `请填写${label}` }).finite();
    if (field.type === "integer") numberSchema = numberSchema.int("请输入整数");
    if (field.min !== undefined)
      numberSchema = numberSchema.min(field.min, `不能小于 ${field.min}`);
    if (field.max !== undefined)
      numberSchema = numberSchema.max(field.max, `不能大于 ${field.max}`);
    schema = numberSchema;
  } else {
    schema = z.string({ error: `请填写${label}` });
    if (field.required)
      schema = schema.refine(
        (value) => typeof value === "string" && value.trim().length > 0,
        `请填写${label}`,
      );
  }
  if (field.enum) {
    schema = schema.refine(
      (value) => field.enum?.includes(value as string | number),
      `请选择有效的${label}`,
    );
  }
  return z.preprocess(
    (value) => (value === "" ? undefined : value),
    field.required ? schema : schema.optional(),
  );
}

function ModelParamsFormFields({
  mode,
  schema,
  onSubmit,
}: Omit<ModelParamsFormProps, "modelKey" | "profileVersionId">) {
  const idPrefix = useId();
  const fields = schema.filter(
    (field) => !field.for_modes || field.for_modes.includes(mode),
  );
  const shape: Record<string, z.ZodType<unknown>> = {};
  const defaults: Record<string, ParamValue | undefined> = {};
  for (const field of fields) {
    shape[field.field] = valueSchema(field);
    defaults[field.field] =
      field.default ??
      (field.type === "boolean"
        ? false
        : field.component === "slider"
          ? (field.min ?? 0)
          : "");
  }
  const { control, handleSubmit } = useForm<Record<string, unknown>>({
    defaultValues: defaults,
    resolver: zodResolver(z.object(shape)),
    shouldUnregister: true,
  });

  return (
    <form
      noValidate
      autoComplete="off"
      onSubmit={handleSubmit((values) => {
        const defined = Object.fromEntries(
          Object.entries(values).filter(([, value]) => value !== undefined),
        );
        onSubmit(defined as ParamValues);
      })}
      className="space-y-6"
    >
      {fields.map((param) => (
        <Controller
          key={param.field}
          name={param.field}
          control={control}
          render={({ field, fieldState }) => {
            const id = `${idPrefix}-${param.field}`;
            const labelId = `${id}-label`;
            const descriptionId = `${id}-description`;
            const errorId = `${id}-error`;
            const describedBy =
              [param.description && descriptionId, fieldState.error && errorId]
                .filter(Boolean)
                .join(" ") || undefined;
            const className =
              "w-full min-h-10 rounded-lg border border-input bg-background px-3 text-sm text-foreground outline-none focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50 aria-invalid:border-destructive";
            const inputProps = {
              id,
              name: field.name,
              "aria-invalid": !!fieldState.error,
              "aria-describedby": describedBy,
              onBlur: field.onBlur,
            };

            let controlElement;
            if (param.component === "switch") {
              controlElement = (
                <input
                  {...inputProps}
                  ref={field.ref}
                  type="checkbox"
                  checked={field.value === true}
                  onChange={(event) => field.onChange(event.target.checked)}
                  className="size-5 accent-primary"
                />
              );
            } else if (param.component === "textarea") {
              controlElement = (
                <textarea
                  {...inputProps}
                  ref={field.ref}
                  value={typeof field.value === "string" ? field.value : ""}
                  onChange={(event) => field.onChange(event.target.value)}
                  rows={3}
                  className={`${className} py-2`}
                />
              );
            } else if (
              param.component === "select" ||
              param.component === "voice"
            ) {
              controlElement = (
                <select
                  {...inputProps}
                  ref={field.ref}
                  value={
                    typeof field.value === "string" ||
                    typeof field.value === "number"
                      ? String(field.value)
                      : ""
                  }
                  onChange={(event) =>
                    field.onChange(
                      event.target.value === ""
                        ? ""
                        : param.type === "string"
                          ? event.target.value
                          : Number(event.target.value),
                    )
                  }
                  className={className}
                >
                  <option value="">请选择</option>
                  {param.enum?.map((option) => (
                    <option key={String(option)} value={String(option)}>
                      {optionLabel(param, option)}
                    </option>
                  ))}
                </select>
              );
            } else if (param.component === "segmented") {
              controlElement = (
                <div
                  role="group"
                  aria-labelledby={labelId}
                  aria-describedby={describedBy}
                  className="flex flex-wrap gap-2"
                >
                  {param.enum?.map((option, index) => (
                    <Button
                      key={String(option)}
                      ref={index === 0 ? field.ref : undefined}
                      type="button"
                      variant={field.value === option ? "default" : "outline"}
                      aria-pressed={field.value === option}
                      onClick={() => field.onChange(option)}
                    >
                      {optionLabel(param, option)}
                    </Button>
                  ))}
                </div>
              );
            } else if (param.component === "slider") {
              controlElement = (
                <div className="flex items-center gap-4">
                  <input
                    {...inputProps}
                    ref={field.ref}
                    type="range"
                    min={param.min}
                    max={param.max}
                    step={param.step ?? "any"}
                    value={
                      typeof field.value === "number"
                        ? field.value
                        : (param.min ?? 0)
                    }
                    onChange={(event) =>
                      field.onChange(Number(event.target.value))
                    }
                    className="min-h-10 min-w-0 flex-1 accent-primary"
                  />
                  <output
                    htmlFor={id}
                    className="min-w-10 text-right text-sm tabular-nums"
                  >
                    {String(field.value ?? param.min ?? 0)}
                  </output>
                </div>
              );
            } else {
              controlElement = (
                <input
                  {...inputProps}
                  ref={field.ref}
                  type={param.type === "string" ? "text" : "number"}
                  min={param.min}
                  max={param.max}
                  step={param.type === "integer" ? 1 : (param.step ?? "any")}
                  value={
                    typeof field.value === "string" ||
                    typeof field.value === "number"
                      ? field.value
                      : ""
                  }
                  onChange={(event) =>
                    field.onChange(
                      event.target.value === ""
                        ? ""
                        : param.type === "string"
                          ? event.target.value
                          : Number(event.target.value),
                    )
                  }
                  className={className}
                />
              );
            }

            return (
              <Field data-invalid={fieldState.invalid}>
                {param.component === "segmented" ? (
                  <FieldTitle id={labelId}>{param.label}</FieldTitle>
                ) : (
                  <FieldLabel htmlFor={id}>{param.label}</FieldLabel>
                )}
                {controlElement}
                {param.description && (
                  <FieldDescription id={descriptionId}>
                    {param.description}
                  </FieldDescription>
                )}
                {fieldState.error && (
                  <FieldError id={errorId} aria-live="polite">
                    {fieldState.error.message}
                  </FieldError>
                )}
              </Field>
            );
          }}
        />
      ))}
      <Button type="submit">继续报价</Button>
    </form>
  );
}

export function ModelParamsForm(props: ModelParamsFormProps) {
  return (
    <ModelParamsFormFields
      key={`${props.modelKey}:${props.profileVersionId}:${props.mode}`}
      mode={props.mode}
      schema={props.schema}
      onSubmit={props.onSubmit}
    />
  );
}
