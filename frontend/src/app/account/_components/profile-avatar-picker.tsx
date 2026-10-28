"use client";

import { useEffect, useId, useRef, useState } from "react";
import { getMyAvatar } from "@/api/media";
import { BoundAvatar } from "@/components/account/avatar-editor";
import { Avatar, AvatarImage, AvatarFallback } from "@/components/ui/avatar";
import { Button } from "@/components/ui/button";
import { Field, FieldDescription, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";

export type AvatarDraft = File | "default" | null;

export function ProfileAvatarPicker({
  assetID,
  label,
  value,
  onChange,
  disabled,
}: {
  assetID?: string;
  label: string;
  value: AvatarDraft;
  onChange: (value: AvatarDraft) => void;
  disabled: boolean;
}) {
  const id = useId();
  const input = useRef<HTMLInputElement>(null);
  const [preview, setPreview] = useState<{ file: File; url: string } | null>(
    null,
  );
  const [error, setError] = useState("");
  useEffect(
    () => () => {
      if (preview) URL.revokeObjectURL(preview.url);
    },
    [preview],
  );
  const local =
    value instanceof File && preview?.file === value ? preview.url : "";
  return (
    <section aria-label="编辑头像" className="flex flex-col gap-3">
      <div className="flex flex-wrap items-center gap-3">
        {value ? (
          <Avatar className="size-12">
            <AvatarImage src={local || undefined} alt={`${label}的头像预览`} />
            <AvatarFallback>
              {Array.from(label.trim())[0] || "织"}
            </AvatarFallback>
          </Avatar>
        ) : (
          <BoundAvatar
            assetID={assetID}
            label={label}
            size="compact"
            load={(signal) => getMyAvatar({ signal })}
          />
        )}
        <Button
          type="button"
          variant="secondary"
          disabled={disabled}
          onClick={() => input.current?.click()}
        >
          更换头像
        </Button>
        {(assetID || value instanceof File) && value !== "default" ? (
          <Button
            type="button"
            variant="ghost"
            disabled={disabled}
            onClick={() => {
              setPreview(null);
              setError("");
              onChange("default");
            }}
          >
            恢复默认头像
          </Button>
        ) : null}
        {value ? (
          <Button
            type="button"
            variant="ghost"
            disabled={disabled}
            onClick={() => {
              setPreview(null);
              setError("");
              onChange(null);
            }}
          >
            撤销头像更改
          </Button>
        ) : null}
      </div>
      <Field data-disabled={disabled} data-invalid={!!error}>
        <FieldLabel htmlFor={id} className="sr-only">
          选择头像（JPEG、PNG、WebP，最多 5 MB）
        </FieldLabel>
        <Input
          ref={input}
          id={id}
          className="sr-only"
          type="file"
          accept="image/jpeg,image/png,image/webp"
          disabled={disabled}
          aria-invalid={!!error}
          aria-describedby={`${id}-description`}
          onChange={(event) => {
            const file = event.target.files?.[0];
            event.target.value = "";
            if (!file) return;
            if (
              !["image/jpeg", "image/png", "image/webp"].includes(file.type)
            ) {
              setError("请选择 JPEG、PNG 或 WebP 图片。");
              return;
            }
            if (file.size > 5 * 1024 * 1024) {
              setError("图片不能超过 5 MB。");
              return;
            }
            setError("");
            setPreview({ file, url: URL.createObjectURL(file) });
            onChange(file);
          }}
        />
        <FieldDescription
          id={`${id}-description`}
          role={error ? "alert" : undefined}
        >
          {error ||
            "支持 JPEG、PNG、WebP，最多 5 MB，无需裁剪为正方形。头像随资料一起保存。"}
        </FieldDescription>
      </Field>
    </section>
  );
}
