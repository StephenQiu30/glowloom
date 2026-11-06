"use client";

import { useId, useState, type Ref } from "react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Field,
  FieldLabel,
  FieldLegend,
  FieldSet,
} from "@/components/ui/field";
import { Empty, EmptyHeader, EmptyTitle } from "@/components/ui/empty";
import { Input } from "@/components/ui/input";
import { Separator } from "@/components/ui/separator";

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
        <FieldLabel htmlFor={field} className="sr-only">
          搜索模型
        </FieldLabel>
        <Input
          id={field}
          ref={searchRef}
          placeholder="搜索名称或模型 ID"
          className="max-md:min-h-11 max-md:text-base"
          value={search}
          onChange={(event) => setSearch(event.target.value)}
          onKeyDown={(event) => {
            if (event.key === "Enter") event.preventDefault();
          }}
        />
      </Field>
      <div className="max-h-64 min-h-0 overflow-y-auto p-px">
        {!visible.length && !onlineOnly.length && (
          <Empty className="border-0 p-4">
            <EmptyHeader>
              <EmptyTitle className="text-sm">
                暂无匹配模型，可手动添加。
              </EmptyTitle>
            </EmptyHeader>
          </Empty>
        )}
        {Object.entries(groups).map(([group, label]) => {
          const entries = visible.filter(
            (model) => model.capability?.split(".")[0] === group,
          );
          if (!entries.length) return null;
          return (
            <FieldSet key={group} className="gap-1 px-1 pb-1">
              <FieldLegend className="mb-1 text-xs font-medium text-muted-foreground">
                {label}
              </FieldLegend>
              {entries.map((model) => {
                const key = model.key!;
                const name = model.display_name ?? model.provider_model_id;
                const checked = picked.includes(key);
                const configured = isConfigured?.(model);
                return (
                  <FieldLabel
                    key={key}
                    className="flex min-h-11 w-full cursor-pointer items-center gap-2.5 rounded-md px-2 py-1.5 text-sm hover:bg-muted has-data-checked:bg-muted has-[:focus-visible]:ring-1 has-[:focus-visible]:ring-ring/50"
                  >
                    <Checkbox
                      checked={checked}
                      aria-label={name}
                      disabled={disabled || canSelect?.(model) === false}
                      onCheckedChange={() => onPreset(model)}
                    />
                    <span className="flex min-w-0 flex-1 flex-col gap-0.5">
                      <span className="break-words text-foreground">
                        {name}
                      </span>
                      {model.display_name && model.provider_model_id && (
                        <span className="text-xs break-all text-muted-foreground">
                          {model.provider_model_id}
                        </span>
                      )}
                    </span>
                    {(configured || model.requires_model_selection) && (
                      <Badge variant="secondary">
                        {configured ? "已添加" : "需填写接入点"}
                      </Badge>
                    )}
                  </FieldLabel>
                );
              })}
            </FieldSet>
          );
        })}
        {onlineOnly.length > 0 && (
          <section className="flex flex-col gap-1 px-1" aria-label="在线目录">
            {visible.length > 0 && <Separator className="my-1" />}
            <p className="text-xs text-muted-foreground">在线目录</p>
            {onlineOnly.slice(0, 50).map((id) => (
              <Button
                key={id}
                variant="ghost"
                size="sm"
                type="button"
                disabled={disabled}
                className="h-auto min-h-9 justify-between gap-2 whitespace-normal max-md:min-h-11"
                aria-label={id}
                onClick={() => onOnline(id)}
              >
                <span className="min-w-0 text-left font-mono break-all">
                  {id}
                </span>
                <span aria-hidden="true" className="shrink-0">
                  选择能力
                </span>
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
