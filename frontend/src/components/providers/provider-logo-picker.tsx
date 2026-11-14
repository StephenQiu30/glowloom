"use client";

import { useEffect, useId, useRef, useState } from "react";
import { uploadProviderLogo } from "@/api/media";
import { ProviderLogo } from "@/components/providers/provider-logo";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Field, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";

export function ProviderLogoPicker({
  value,
  onChange,
  adapterKey,
  disabled = false,
  onUploadingChange,
}: {
  value?: string;
  onChange: (assetID: string | undefined) => void;
  adapterKey?: string;
  disabled?: boolean;
  onUploadingChange?: (uploading: boolean) => void;
}) {
  const id = useId();
  const input = useRef<HTMLInputElement>(null);
  const failedFile = useRef<File | null>(null);
  const flight = useRef<AbortController | null>(null);
  const [uploading, setUploading] = useState(false);
  const [error, setError] = useState("");
  useEffect(
    () => () => {
      flight.current?.abort();
    },
    [],
  );
  async function upload(file: File) {
    if (disabled || flight.current) return;
    const controller = new AbortController();
    flight.current = controller;
    failedFile.current = file;
    setUploading(true);
    onUploadingChange?.(true);
    setError("");
    try {
      const result = await uploadProviderLogo({}, file, {
        signal: controller.signal,
      });
      if (controller.signal.aborted) return;
      if (!result.id) throw new Error("missing uploaded logo");
      failedFile.current = null;
      onChange(result.id);
    } catch {
      if (!controller.signal.aborted) setError("Logo 上传失败，请重试。");
    } finally {
      if (!controller.signal.aborted) {
        flight.current = null;
        setUploading(false);
        onUploadingChange?.(false);
      }
    }
  }
  return (
    <Field className="mt-4 max-md:[&_button]:min-h-11">
      <FieldLabel htmlFor={id}>Logo（可选）</FieldLabel>
      <div className="flex flex-wrap items-center gap-3">
        <ProviderLogo adapterKey={adapterKey} logoAssetID={value} iconOnly />
        <Input
          ref={input}
          id={id}
          type="file"
          accept="image/png,image/jpeg,image/webp"
          disabled={disabled || uploading}
          className="sr-only"
          onChange={(event) => {
            const file = event.target.files?.[0];
            event.target.value = "";
            if (file) void upload(file);
          }}
        />
        <Button
          type="button"
          variant="outline"
          size="sm"
          disabled={disabled || uploading}
          onClick={() => input.current?.click()}
        >
          {uploading ? "正在上传…" : "上传 Logo"}
        </Button>
        {value ? (
          <Button
            type="button"
            variant="ghost"
            size="sm"
            disabled={disabled || uploading}
            onClick={() => {
              setError("");
              failedFile.current = null;
              onChange(undefined);
            }}
          >
            清除 Logo
          </Button>
        ) : null}
      </div>
      {error ? (
        <Alert variant="destructive">
          <AlertDescription className="flex flex-wrap items-center justify-between gap-2">
            {error}
            <Button
              type="button"
              variant="outline"
              size="sm"
              disabled={disabled || uploading}
              onClick={() => {
                if (failedFile.current) void upload(failedFile.current);
              }}
            >
              重试上传
            </Button>
          </AlertDescription>
        </Alert>
      ) : null}
    </Field>
  );
}
