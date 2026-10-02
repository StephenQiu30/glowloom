"use client";

import dynamic from "next/dynamic";
import { useId, useState } from "react";
import { Controller, useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { canonicalRichDocument, type RichDocument } from "./rich-document";
import { SourceExtractionWarnings } from "./source-extraction-warnings";
import {
  sourceKindSchema,
  sourceStatusSchema,
  sourceTitleSchema,
  sourceWriteSchema,
  type SourceDetail,
} from "./source-model";

const RichEditor = dynamic(
  () =>
    import("./rich-source-editor").then((module) => module.RichSourceEditor),
  { ssr: false, loading: () => <p role="status">正在加载正文编辑器…</p> },
);
const metadataSchema = z.object({
  title: sourceTitleSchema,
  source_kind: sourceKindSchema,
  status: sourceStatusSchema,
  rights_confirmed: z
    .boolean()
    .refine((value) => value, "请确认已获授权使用此正文。"),
});
type SourceWrite = z.infer<typeof sourceWriteSchema>;
export function SourceEditForm({
  source,
  base,
  locked,
  readOnly,
  onSubmit,
  onDirty,
  submitLabel,
}: {
  source?: SourceDetail;
  base: { expected_revision: number; base_version_id?: string };
  locked: boolean;
  readOnly: boolean;
  onSubmit: (body: SourceWrite) => void | Promise<void>;
  onDirty: (dirty: boolean) => void;
  submitLabel?: string;
}) {
  const id = useId();
  const [document, setDocument] = useState<RichDocument>(
    () => source?.document ?? { type: "doc" },
  );
  const [richError, setRichError] = useState<string | null>(null);
  const [submitError, setSubmitError] = useState<string | null>(null);
  const form = useForm({
    resolver: zodResolver(metadataSchema),
    defaultValues: {
      title: source?.title ?? "",
      source_kind: source?.source_kind ?? "chapter",
      status: source?.status ?? "draft",
      rights_confirmed: false,
    },
  });
  const unavailable = locked || readOnly || form.formState.isSubmitting;
  const title = form.register("title");
  return (
    <form
      className="min-w-0 space-y-4"
      onSubmit={form.handleSubmit(async (metadata) => {
        if (unavailable || richError) return;
        const checked = sourceWriteSchema.safeParse({
          ...base,
          ...metadata,
          rights_confirmed: true,
          document: canonicalRichDocument(document),
          provenance: {
            ...(source?.provenance.external_id
              ? { external_id: source.provenance.external_id }
              : {}),
            ...(source?.provenance.external_position !== undefined
              ? { external_position: source.provenance.external_position }
              : {}),
          },
        });
        if (!checked.success) {
          setSubmitError(
            checked.error.issues[0]?.message ?? "正文输入无法完整保存。",
          );
          return;
        }
        setSubmitError(null);
        await onSubmit(checked.data);
      })}
    >
      <div className="space-y-2">
        <Label htmlFor={`${id}-title`}>来源标题</Label>
        <Input
          {...title}
          id={`${id}-title`}
          disabled={unavailable}
          aria-invalid={Boolean(form.formState.errors.title)}
          aria-describedby={`${id}-title-error`}
          onChange={(event) => {
            void title.onChange(event);
            onDirty(true);
          }}
        />
        <p
          id={`${id}-title-error`}
          role={form.formState.errors.title ? "alert" : undefined}
          className="text-sm text-destructive"
        >
          {form.formState.errors.title?.message}
        </p>
      </div>
      <div className="flex flex-wrap gap-4">
        <div className="space-y-2">
          <Label htmlFor={`${id}-kind`}>来源类型</Label>
          <Controller
            name="source_kind"
            control={form.control}
            render={({ field }) => (
              <Select
                value={field.value}
                disabled={unavailable}
                onValueChange={(value) => {
                  field.onChange(value);
                  onDirty(true);
                }}
              >
                <SelectTrigger id={`${id}-kind`} aria-label="来源类型">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="chapter">章节</SelectItem>
                  <SelectItem value="episode">来源集</SelectItem>
                  <SelectItem value="document">原文文档</SelectItem>
                </SelectContent>
              </Select>
            )}
          />
        </div>
        <div className="space-y-2">
          <Label htmlFor={`${id}-status`}>来源状态</Label>
          <Controller
            name="status"
            control={form.control}
            render={({ field }) => (
              <Select
                value={field.value}
                disabled={unavailable}
                onValueChange={(value) => {
                  field.onChange(value);
                  onDirty(true);
                }}
              >
                <SelectTrigger id={`${id}-status`} aria-label="来源状态">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="draft">草稿</SelectItem>
                  <SelectItem value="ready">可用</SelectItem>
                  <SelectItem value="completed">完成</SelectItem>
                </SelectContent>
              </Select>
            )}
          />
        </div>
      </div>
      <p className="text-sm text-muted-foreground">
        来源状态只描述这份输入；正式分集和制作结果另行确认。规范正文保留空段落、换行及全部受支持格式。
      </p>
      {source?.origin === "file" && (
        <p className="rounded-lg border p-3 text-sm">
          手工保存会生成新的来源快照。原始文件与抽取坐标保留在上一份文件来源历史中。
        </p>
      )}
      <SourceExtractionWarnings warnings={source?.provenance.warnings} />
      <RichEditor
        initialDocument={source?.document ?? { type: "doc" }}
        disabled={unavailable}
        onChange={(value) => {
          setDocument(value);
          onDirty(true);
        }}
        onInvalid={(message) => {
          setRichError(message);
          if (message) onDirty(true);
        }}
      />
      {richError && (
        <p role="alert" className="text-sm text-destructive">
          {richError}
        </p>
      )}
      {submitError && (
        <p role="alert" className="text-sm text-destructive">
          {submitError}
        </p>
      )}
      {!readOnly && (
        <>
          <div className="flex items-start gap-2">
            <Controller
              name="rights_confirmed"
              control={form.control}
              render={({ field }) => (
                <Checkbox
                  id={`${id}-rights`}
                  checked={field.value}
                  disabled={unavailable}
                  onCheckedChange={(value) => {
                    field.onChange(value === true);
                    onDirty(true);
                  }}
                />
              )}
            />
            <Label htmlFor={`${id}-rights`}>已获授权使用此正文</Label>
          </div>
          {form.formState.errors.rights_confirmed && (
            <p role="alert" className="text-sm text-destructive">
              {form.formState.errors.rights_confirmed.message}
            </p>
          )}
          <Button type="submit" disabled={unavailable || Boolean(richError)}>
            {form.formState.isSubmitting
              ? "正在确认保存…"
              : (submitLabel ?? (source ? "保存来源" : "创建来源"))}
          </Button>
        </>
      )}
    </form>
  );
}
