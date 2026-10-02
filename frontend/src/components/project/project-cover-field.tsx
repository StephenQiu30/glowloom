"use client";
import { useEffect, useRef, useState } from "react";
import dynamic from "next/dynamic";
import { Button } from "@/components/ui/button";
import { Field, FieldLabel, FieldDescription } from "@/components/ui/field";
import type { FolderScope } from "./folder-intent";
import { ProjectCoverPreview } from "./project-cover-preview";
const ProjectCoverPicker = dynamic(
  () =>
    import("./project-cover-picker").then(
      (module) => module.ProjectCoverPicker,
    ),
  { ssr: false },
);
const ProjectCoverUpload = dynamic(
  () =>
    import("./project-cover-upload").then(
      (module) => module.ProjectCoverUpload,
    ),
  { ssr: false },
);

export function ProjectCoverField({
  projectId,
  scope,
  value,
  savedAssetId,
  unavailable,
  previewAllowed = true,
  disabled,
  onChange,
  onBusyChange,
}: {
  projectId: string;
  scope?: FolderScope;
  value: string | null;
  savedAssetId: string | null;
  unavailable: boolean;
  previewAllowed?: boolean;
  disabled: boolean;
  onChange: (assetId: string | null) => void;
  onBusyChange: (busy: boolean) => void;
}) {
  const [dialog, setDialog] = useState<"picker" | "upload" | null>(null);
  const trigger = useRef<HTMLButtonElement | null>(null);
  const focusFrame = useRef<number | null>(null);
  useEffect(
    () => () => {
      if (focusFrame.current !== null)
        window.cancelAnimationFrame(focusFrame.current);
    },
    [],
  );
  const open = (value: "picker" | "upload", button: HTMLButtonElement) => {
    trigger.current = button;
    onBusyChange(true);
    setDialog(value);
  };
  const close = () => {
    setDialog(null);
    onBusyChange(false);
    if (focusFrame.current !== null)
      window.cancelAnimationFrame(focusFrame.current);
    focusFrame.current = window.requestAnimationFrame(() => {
      focusFrame.current = null;
      if (trigger.current?.isConnected) trigger.current.focus();
    });
  };
  const selected = (assetId: string) => {
    onChange(assetId);
    close();
  };
  return (
    <Field>
      <FieldLabel>项目主图</FieldLabel>
      <div className="relative aspect-video max-w-80 overflow-hidden rounded-xl bg-muted">
        {value && !previewAllowed ? (
          <span role="status" className="text-xs text-muted-foreground">
            当前主图授权尚未核验
          </span>
        ) : value && scope ? (
          <ProjectCoverPreview
            key={`${scope.actorId}:${scope.orgId}:${value}`}
            projectId={projectId}
            assetId={value}
            scope={scope}
            eager
            unavailable={value === savedAssetId && unavailable}
          />
        ) : (
          <div className="flex h-full items-center justify-center text-sm text-muted-foreground">
            {value ? "正在核验当前身份…" : "未设置主图"}
          </div>
        )}
      </div>
      <div className="flex flex-wrap gap-2">
        <Button
          type="button"
          variant="outline"
          disabled={disabled || !scope}
          onClick={(event) => open("picker", event.currentTarget)}
        >
          选择项目图片
        </Button>
        <Button
          type="button"
          variant="outline"
          disabled={disabled || !scope}
          onClick={(event) => open("upload", event.currentTarget)}
        >
          上传主图
        </Button>
        <Button
          type="button"
          variant="ghost"
          disabled={disabled || !value}
          onClick={() => onChange(null)}
        >
          清除主图
        </Button>
      </div>
      <FieldDescription>
        主图随项目设置保存。清除关联会保留原图片素材。
      </FieldDescription>
      {dialog === "picker" && scope && (
        <ProjectCoverPicker
          projectId={projectId}
          scope={scope}
          value={value}
          onClose={close}
          onSelected={selected}
        />
      )}
      {dialog === "upload" && scope && (
        <ProjectCoverUpload
          projectId={projectId}
          scope={scope}
          onClose={close}
          onSelected={selected}
        />
      )}
    </Field>
  );
}
