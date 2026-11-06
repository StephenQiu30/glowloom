"use client";

import { useId, useState, type Ref } from "react";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Field,
  FieldLabel,
  FieldLegend,
  FieldSet,
} from "@/components/ui/field";
import { Input } from "@/components/ui/input";

const groups: Record<string, string> = {
  text: "文本",
  image: "图片",
  video: "视频",
  audio: "音频",
};

export function ModelCatalogChoices({
  models,
  online,
  picked,
  disabled,
  canSelect,
  isConfigured,
  onPreset,
  onOnline,
  searchRef,
}: {
  models: API.ModelCandidatePreset[];
  online: string[];
  picked: string[];
  disabled?: boolean;
  canSelect?: (model: API.ModelCandidatePreset) => boolean;
  isConfigured?: (model: API.ModelCandidatePreset) => boolean;
  onPreset: (model: API.ModelCandidatePreset) => void;
  onOnline: (id: string) => void;
  searchRef?: Ref<HTMLInputElement>;
}) {
  const field = useId();
  const [search, setSearch] = useState("");
  const term = search.trim().toLowerCase();
  const visible = models.filter(
    (model) =>
      model.key &&
      [model.display_name, model.provider_model_id].some((value) =>
        value?.toLowerCase().includes(term),
      ),
  );
  const knownIDs = new Set(models.map((model) => model.provider_model_id));
  const onlineOnly = online.filter(
    (id) => !knownIDs.has(id) && id.toLowerCase().includes(term),
  );
  return (
    <>
      <Field>
        <FieldLabel htmlFor={field}>搜索模型</FieldLabel>
        <Input
          id={field}
          ref={searchRef}
          value={search}
          onChange={(event) => setSearch(event.target.value)}
          onKeyDown={(event) => {
            if (event.key === "Enter") event.preventDefault();
          }}
        />
      </Field>
      <div className="max-h-52 min-h-0 overflow-y-auto rounded-md border">
        {!visible.length && !onlineOnly.length && (
          <p className="p-3 text-sm text-muted-foreground">
            暂无匹配模型，可手动添加。
          </p>
        )}
        {Object.entries(groups).map(([group, label]) => {
          const entries = visible.filter(
            (model) => model.capability?.split(".")[0] === group,
          );
          if (!entries.length) return null;
          return (
            <FieldSet
              key={group}
              className="mx-3 mb-2 border-b pb-2 last:border-b-0"
            >
              <FieldLegend className="pt-2 text-xs font-medium text-muted-foreground">
                {label}
              </FieldLegend>
              {entries.map((model) => {
                const key = model.key!;
                const name = model.display_name ?? model.provider_model_id;
                return (
                  <FieldLabel
                    key={key}
                    className="flex cursor-pointer items-center gap-2 py-2 text-sm"
                  >
                    <Checkbox
                      checked={picked.includes(key)}
                      aria-label={name}
                      disabled={disabled || canSelect?.(model) === false}
                      onCheckedChange={() => onPreset(model)}
                    />
                    <span className="min-w-0 break-all">{name}</span>
                    <span className="ml-auto min-w-0 text-right text-xs break-all text-muted-foreground">
                      {isConfigured?.(model)
                        ? "已添加"
                        : model.requires_model_selection
                          ? "需填写接入点"
                          : model.display_name
                            ? model.provider_model_id
                            : ""}
                    </span>
                  </FieldLabel>
                );
              })}
            </FieldSet>
          );
        })}
        {onlineOnly.length > 0 && (
          <section className="flex flex-col gap-1 p-3" aria-label="在线目录">
            <p className="text-xs text-muted-foreground">
              在线目录中的其他型号需要选择能力
            </p>
            {onlineOnly.slice(0, 50).map((id) => (
              <Button
                key={id}
                variant="ghost"
                size="sm"
                type="button"
                disabled={disabled}
                className="h-auto justify-start break-all whitespace-normal"
                onClick={() => onOnline(id)}
              >
                {id}
              </Button>
            ))}
            {onlineOnly.length > 50 && (
              <p className="text-xs text-muted-foreground">
                请搜索以缩小目录范围。
              </p>
            )}
          </section>
        )}
      </div>
    </>
  );
}
