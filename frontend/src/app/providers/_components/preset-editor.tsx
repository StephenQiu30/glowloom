"use client";

import { useId, useState, type RefObject } from "react";
import { FeedbackToast } from "@/components/feedback/feedback-toast";
import {
  Field,
  FieldSet,
  FieldLegend,
  FieldGroup,
  FieldLabel,
} from "@/components/ui/field";
import { Textarea } from "@/components/ui/textarea";
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
  SheetDescription,
  SheetFooter,
} from "@/components/ui/sheet";
import {
  createProviderPreset,
  previewProviderPreset,
  publishProviderPresetVersion,
} from "@/api/settings";
import {
  catalogMessage,
  MutationStatus,
  useCatalogMutation,
  zeroID,
} from "@/components/catalog/mutation";
import { PresetModels } from "./preset-models";

export function PresetEditor({
  preset,
  presets,
  onClose,
  onSaved,
  returnFocusRef,
}: {
  preset?: API.Preset;
  presets: API.Preset[];
  onClose: () => void;
  returnFocusRef?: RefObject<HTMLElement | null>;
  onSaved: () => void;
}) {
  const fieldID = useId();
  const base =
    preset?.document ??
    presets.find(
      (p) =>
        p.origin === "builtin" &&
        p.document?.config?.adapter_key === "openrouter",
    )?.document ??
    presets[0]?.document;
  const [document, setDocument] = useState<API.PresetDocument>(
    base
      ? {
          ...base,
          name: preset?.origin === "custom" ? base.name : `${base.name} 自定义`,
        }
      : {},
  );
  const [source, setSource] = useState("");
  const [advanced, setAdvanced] = useState(false);
  const [slug, setSlug] = useState(
    preset?.origin === "custom" ? (preset.slug ?? "") : "",
  );
  const [preview, setPreview] = useState<API.PresetPreview | null>(null);
  const [error, setError] = useState("");
  const [checking, setChecking] = useState(false);
  const mutation = useCatalogMutation(onSaved, `preset.${preset?.id ?? "new"}`);
  const editing = preset?.origin === "custom";
  function update(next: API.PresetDocument) {
    setDocument(next);
    setPreview(null);
    setError("");
  }
  function config(name: keyof API.ProviderConfig, value: number) {
    update({ ...document, config: { ...document.config, [name]: value } });
  }
  return (
    <Sheet
      open
      onOpenChange={(open) => {
        if (!open && !mutation.busy) onClose();
      }}
    >
      <SheetContent
        onCloseAutoFocus={(event) => {
          if (returnFocusRef?.current?.isConnected) {
            event.preventDefault();
            returnFocusRef.current.focus();
          }
        }}
        className="gap-0 data-[side=right]:w-full data-[side=right]:sm:max-w-xl max-md:[&_[data-slot=sheet-close]]:size-11"
      >
        <SheetHeader className="p-6">
          <SheetTitle>
            {editing ? "创建预设新版本" : preset ? "复制预设" : "新建预设"}
          </SheetTitle>
          <SheetDescription>预设不包含 API Key。</SheetDescription>
        </SheetHeader>
        <div className="flex min-h-0 flex-1 flex-col gap-5 overflow-y-auto px-6 pb-6">
          <FieldSet
            disabled={checking || mutation.busy}
            className="[&_[data-slot=input]]:h-10 max-md:[&_[data-slot=input]]:h-11 [&_[data-slot=native-select]]:h-10 max-md:[&_[data-slot=native-select]]:h-11"
          >
            {!advanced ? (
              <FieldGroup className="gap-4">
                <Field>
                  <FieldLabel htmlFor={`${fieldID}-name`}>预设名称</FieldLabel>
                  <Input
                    id={`${fieldID}-name`}
                    value={document.name ?? ""}
                    maxLength={200}
                    onChange={(e) =>
                      update({ ...document, name: e.target.value })
                    }
                  />
                </Field>
                <Field>
                  <FieldLabel htmlFor={`${fieldID}-protocol`}>
                    平台协议
                  </FieldLabel>
                  <Select
                    value={document.config?.adapter_key ?? ""}
                    onValueChange={(value) => {
                      const base = presets.find(
                        (p) =>
                          p.origin === "builtin" &&
                          p.document?.config?.adapter_key === value,
                      )?.document;
                      if (base)
                        update({
                          ...document,
                          config: base.config,
                          model_templates: [],
                        });
                    }}
                  >
                    <SelectTrigger
                      id={`${fieldID}-protocol`}
                      className="w-full"
                    >
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectGroup>
                        {presets
                          .filter((p) => p.origin === "builtin")
                          .map((p) =>
                            p.document?.config?.adapter_key ? (
                              <SelectItem
                                key={p.id}
                                value={p.document?.config?.adapter_key}
                              >
                                {p.document?.name}
                              </SelectItem>
                            ) : null,
                          )}
                      </SelectGroup>
                    </SelectContent>
                  </Select>
                </Field>
                <Field>
                  <FieldLabel htmlFor={`${fieldID}-region`}>区域</FieldLabel>
                  <Input
                    id={`${fieldID}-region`}
                    readOnly
                    value={
                      document.config?.region === "domestic"
                        ? "国内"
                        : document.config?.region === "overseas"
                          ? "海外"
                          : "未知"
                    }
                  />
                </Field>
                <Field>
                  <FieldLabel htmlFor={`${fieldID}-endpoint`}>
                    服务端点
                  </FieldLabel>
                  <Input
                    id={`${fieldID}-endpoint`}
                    readOnly
                    value={document.config?.base_url ?? ""}
                  />
                </Field>
                <PresetModels
                  key={document.config?.adapter_key}
                  adapter={document.config?.adapter_key ?? ""}
                  templates={document.model_templates ?? []}
                  onChange={(templates) =>
                    update({ ...document, model_templates: templates })
                  }
                />
              </FieldGroup>
            ) : (
              <Field>
                <FieldLabel htmlFor={`${fieldID}-json`}>
                  预设 JSON（高级导入）
                </FieldLabel>
                <Textarea
                  id={`${fieldID}-json`}
                  className="min-h-64"
                  value={source}
                  onChange={(e) => {
                    setSource(e.target.value);
                    setPreview(null);
                  }}
                  spellCheck={false}
                />
              </Field>
            )}
            <Collapsible>
              <CollapsibleTrigger asChild>
                <Button type="button" variant="ghost">
                  高级设置
                </Button>
              </CollapsibleTrigger>
              <CollapsibleContent className="flex flex-col gap-5 pt-4">
                <FieldSet>
                  <FieldLegend>可提供的能力</FieldLegend>
                  <FieldGroup className="flex-row flex-wrap">
                    {(document.config?.adapter_key === "volcengine_speech"
                      ? ["audio"]
                      : document.config?.adapter_key === "deepseek"
                        ? ["text"]
                        : document.config?.adapter_key === "byteplus"
                          ? ["text", "image"]
                          : ["text", "image", "video"]
                    ).map((capability) => (
                      <Field key={capability} orientation="horizontal">
                        <Checkbox
                          id={`${fieldID}-${capability}`}
                          checked={
                            document.config?.capabilities?.includes(
                              capability,
                            ) ?? false
                          }
                          onCheckedChange={(checked) => {
                            const capabilities = new Set(
                              document.config?.capabilities,
                            );
                            if (checked === true) capabilities.add(capability);
                            else capabilities.delete(capability);
                            update({
                              ...document,
                              config: {
                                ...document.config,
                                capabilities: [...capabilities],
                              },
                            });
                          }}
                        />
                        <FieldLabel htmlFor={`${fieldID}-${capability}`}>
                          {
                            {
                              text: "文本",
                              image: "图片",
                              video: "视频",
                              audio: "音频",
                            }[capability]
                          }
                        </FieldLabel>
                      </Field>
                    ))}
                  </FieldGroup>
                </FieldSet>
                <FieldGroup className="grid grid-cols-1 gap-3 sm:grid-cols-3">
                  {(
                    [
                      ["timeout_seconds", "超时（秒）", 5, 300],
                      ["concurrency_limit", "最大并发", 1, 1000],
                      ["rate_limit_per_min", "每分钟请求数", 1, 100000],
                    ] as const
                  ).map(([name, label, min, max]) => (
                    <Field key={name}>
                      <FieldLabel htmlFor={`${fieldID}-${name}`}>
                        {label}
                      </FieldLabel>
                      <Input
                        id={`${fieldID}-${name}`}
                        type="number"
                        min={min}
                        max={max}
                        value={document.config?.[name] ?? ""}
                        onChange={(e) => config(name, Number(e.target.value))}
                      />
                    </Field>
                  ))}
                </FieldGroup>

                <Field className="mt-3">
                  <FieldLabel htmlFor={`${fieldID}-slug`}>
                    预设标识（留空自动生成）
                  </FieldLabel>
                  <Input
                    id={`${fieldID}-slug`}
                    value={slug}
                    onChange={(e) => setSlug(e.target.value)}
                    disabled={editing}
                    pattern="[a-z][a-z0-9_-]{0,63}"
                  />
                </Field>
                <Button
                  type="button"
                  variant="ghost"
                  disabled={checking || mutation.busy}
                  onClick={async () => {
                    if (advanced) {
                      setChecking(true);
                      try {
                        const result = await previewProviderPreset({
                          document_json: source,
                        });
                        if (!result.document?.config)
                          throw new Error("预设格式不正确。");
                        update(result.document);
                        setAdvanced(false);
                      } catch (error) {
                        setError(catalogMessage(error));
                      } finally {
                        setChecking(false);
                      }
                    } else {
                      setSource(JSON.stringify(document, null, 2));
                      setAdvanced(true);
                    }
                    setPreview(null);
                  }}
                >
                  {advanced ? "返回结构化表单" : "使用 JSON 导入"}
                </Button>
              </CollapsibleContent>
            </Collapsible>
          </FieldSet>
          {error ? <FeedbackToast message={error} /> : null}
          {preview ? (
            <div className="rounded bg-muted p-3 text-sm">
              <p>
                {preview.document?.name} ·{" "}
                {preview.document?.config?.adapter_key}
              </p>
              <p>
                {preview.document?.model_templates?.length ?? 0} 个模型模板 ·
                已有连接保持原版本
              </p>
            </div>
          ) : null}
          {mutation.message || mutation.pending ? (
            <MutationStatus mutation={mutation} />
          ) : null}
        </div>
        <SheetFooter className="flex-row flex-wrap justify-end gap-2 px-6 max-md:[&_button]:min-h-11">
          <Button
            className="max-md:h-11"
            variant="ghost"
            onClick={onClose}
            disabled={checking || mutation.busy}
          >
            取消
          </Button>
          <Button
            type="button"
            variant="outline"
            disabled={checking || mutation.busy}
            onClick={async () => {
              setChecking(true);
              setError("");
              try {
                const result = await previewProviderPreset({
                  document_json: advanced ? source : JSON.stringify(document),
                });
                setPreview(result);
              } catch (e) {
                setError(catalogMessage(e));
              } finally {
                setChecking(false);
              }
            }}
          >
            {checking ? "校验中…" : "校验并预览"}
          </Button>
          <Button
            disabled={!preview || checking || mutation.busy}
            onClick={() => {
              if (!preview?.document || !preview.hash) return;
              return mutation.run(
                "preset.save",
                editing ? preset.id! : zeroID,
                (requestID) => {
                  const input = {
                    slug: slug.trim() || `preset-${requestID}`,
                    document: preview.document,
                    preview_hash: preview.hash,
                    request_id: requestID,
                    expected_revision: editing ? preset.revision : 0,
                    parent_id: editing ? undefined : preset?.id,
                  };
                  return editing
                    ? publishProviderPresetVersion({ id: preset.id! }, input)
                    : createProviderPreset(input);
                },
              );
            }}
          >
            保存新版本
          </Button>
        </SheetFooter>
      </SheetContent>
    </Sheet>
  );
}
