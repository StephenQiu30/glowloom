"use client";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Field, FieldLabel } from "@/components/ui/field";
import { Textarea } from "@/components/ui/textarea";
import { useState, useId } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Dialog,
  DialogContent,
  DialogTitle,
  DialogDescription,
} from "@/components/ui/dialog";
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

export function PresetEditor({
  preset,
  onClose,
  onSaved,
}: {
  preset?: API.Preset;
  onClose: () => void;
  onSaved: () => void;
}) {
  const formFieldId = useId();

  const [source, setSource] = useState(
    preset ? JSON.stringify(preset.document, null, 2) : "",
  );
  const [slug, setSlug] = useState(
    preset?.origin === "custom" ? (preset.slug ?? "") : "",
  );
  const [preview, setPreview] = useState<API.PresetPreview | null>(null);
  const [error, setError] = useState("");
  const [checking, setChecking] = useState(false);
  const mutation = useCatalogMutation(onSaved, `preset.${preset?.id ?? "new"}`);
  const editing = preset?.origin === "custom";
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open && !mutation.busy) onClose();
      }}
    >
      <DialogContent className="max-h-[90dvh] overflow-y-auto sm:max-w-2xl">
        <DialogTitle>
          {editing ? "创建预设新版本" : "导入或复制预设"}
        </DialogTitle>
        <DialogDescription>
          只包含平台协议和默认配置。内置预设需复制后修改，不会影响已创建的连接。
        </DialogDescription>
        <Field data-disabled={editing}>
          <FieldLabel htmlFor={`${formFieldId}-field-1`}>
            <span>预设标识</span>
          </FieldLabel>
          <Input
            id={`${formFieldId}-field-1`}
            value={slug}
            onChange={(e) => setSlug(e.target.value)}
            disabled={editing}
            pattern="[a-z][a-z0-9_-]{0,63}"
            placeholder="例如：openrouter-team"
          />
        </Field>
        <Field>
          <FieldLabel htmlFor={`${formFieldId}-field-2`}>
            <span>预设 JSON</span>
          </FieldLabel>
          <Textarea
            id={`${formFieldId}-field-2`}
            className="min-h-64 w-full"
            value={source}
            onChange={(e) => {
              setSource(e.target.value);
              setPreview(null);
            }}
            spellCheck={false}
          />
        </Field>
        {error ? (
          <Alert variant="destructive">
            <AlertDescription>{error}</AlertDescription>
          </Alert>
        ) : null}
        {preview ? (
          <div className="rounded-lg bg-muted p-3 text-sm">
            <p>
              {preview.document?.name} · {preview.document?.config?.adapter_key}
            </p>
            <p>能力：{preview.document?.config?.capabilities?.join("、")}</p>
            <p>
              并发 {preview.document?.config?.concurrency_limit} · 每分钟{" "}
              {preview.document?.config?.rate_limit_per_min} 次 · 超时{" "}
              {preview.document?.config?.timeout_seconds} 秒
            </p>
          </div>
        ) : null}
        <MutationStatus mutation={mutation} />
        <div className="flex flex-wrap justify-end gap-2">
          <Button
            variant="outline"
            disabled={checking || mutation.busy}
            onClick={async () => {
              setChecking(true);
              setError("");
              try {
                const result = await previewProviderPreset({
                  document_json: source,
                });
                setPreview(result);
              } catch (error) {
                setError(catalogMessage(error));
              } finally {
                setChecking(false);
              }
            }}
          >
            {checking ? "校验中…" : "校验并预览"}
          </Button>
          <Button
            disabled={!preview || !slug || mutation.busy}
            onClick={() => {
              if (!preview?.document || !preview.hash) return;
              return mutation.run(
                "preset.save",
                editing ? preset.id! : zeroID,
                (requestID) => {
                  const input = {
                    slug,
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
        </div>
      </DialogContent>
    </Dialog>
  );
}
