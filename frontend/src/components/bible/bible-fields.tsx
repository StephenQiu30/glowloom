"use client";
import { useId, useState } from "react";
import { useFieldArray, useForm } from "react-hook-form";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import {
  characterInputSchema,
  definitionFields,
  locationSchema,
  type BibleKind,
  type CharacterInput,
} from "./bible-model";
import type { z } from "zod";

export type BibleInput = CharacterInput | z.infer<typeof locationSchema>;
type Draft = {
  name: string;
  aliases: { value: string }[];
  description: string;
  definition: CharacterInput["definition"];
  prompt: string;
};
export function BibleFields({
  kind,
  initial,
  locked,
  onDirty,
  onSubmit,
}: {
  kind: BibleKind;
  initial?: BibleInput;
  locked: boolean;
  onDirty: () => void;
  onSubmit: (content: BibleInput) => void | Promise<void>;
}) {
  const id = useId(),
    [error, setError] = useState<string>();
  const form = useForm<Draft>({
    defaultValues: {
      name: initial?.name ?? "",
      aliases: initial?.aliases?.map((value) => ({ value })) ?? [],
      description: initial?.description ?? "",
      definition: initial && "definition" in initial ? initial.definition : {},
      prompt: initial && "prompt" in initial ? (initial.prompt ?? "") : "",
    },
  });
  const aliases = useFieldArray({ control: form.control, name: "aliases" }),
    busy = locked || form.formState.isSubmitting;
  function changed() {
    onDirty();
    setError(undefined);
  }
  return (
    <form
      className="space-y-4"
      onChange={changed}
      onSubmit={form.handleSubmit(async (draft) => {
        try {
          const labels = {
            name: draft.name,
            ...(draft.aliases.length
              ? { aliases: draft.aliases.map((item) => item.value) }
              : {}),
            ...(draft.description ? { description: draft.description } : {}),
          };
          const content =
            kind === "character"
              ? characterInputSchema.parse({
                  ...labels,
                  definition: draft.definition,
                })
              : locationSchema.parse({
                  ...labels,
                  ...(draft.prompt ? { prompt: draft.prompt } : {}),
                });
          setError(undefined);
          await onSubmit(content);
        } catch {
          setError(
            "名称、别名或完整字段未通过校验，请保留草稿；名称最多512个Unicode字符，别名最多64个且不得重复，描述字段最多8192个字符。",
          );
        }
      })}
    >
      <fieldset disabled={busy} className="space-y-4">
        <div>
          <Label htmlFor={`${id}-name`}>名称</Label>
          <Textarea id={`${id}-name`} {...form.register("name")} />
          <p className="text-xs text-muted-foreground">
            保留原名称空白，1..512个Unicode字符。
          </p>
        </div>
        <div>
          <Label htmlFor={`${id}-description`}>说明</Label>
          <Textarea
            id={`${id}-description`}
            {...form.register("description")}
          />
        </div>
        <fieldset className="space-y-2">
          <legend>别名</legend>
          {aliases.fields.map((item, n) => (
            <div key={item.id} className="space-y-1">
              <Label htmlFor={`${id}-alias-${n}`}>别名{n + 1}</Label>
              <Textarea
                id={`${id}-alias-${n}`}
                {...form.register(`aliases.${n}.value`)}
              />
              <Button
                type="button"
                variant="ghost"
                onClick={() => {
                  aliases.remove(n);
                  changed();
                }}
              >
                移除别名{n + 1}
              </Button>
            </div>
          ))}
          <Button
            type="button"
            variant="outline"
            disabled={aliases.fields.length >= 64}
            onClick={() => {
              aliases.append({ value: "" });
              changed();
            }}
          >
            添加别名
          </Button>
        </fieldset>
        {kind === "character" ? (
          definitionFields.map(([key, label]) => (
            <div key={key}>
              <Label htmlFor={`${id}-${key}`}>{label}</Label>
              <Textarea
                id={`${id}-${key}`}
                {...form.register(`definition.${key}`, {
                  setValueAs: (value: string) => value || undefined,
                })}
              />
            </div>
          ))
        ) : (
          <div>
            <Label htmlFor={`${id}-prompt`}>生成约束</Label>
            <Textarea id={`${id}-prompt`} {...form.register("prompt")} />
          </div>
        )}
        <p className="text-xs text-muted-foreground">
          保存产生不可变版本。当前造型、参考图与声音由各自的明确命令维护。
        </p>
        <Button type="submit">保存完整设定</Button>
      </fieldset>
      {error && <p role="alert">{error}</p>}
    </form>
  );
}
