"use client";

import { useId, useRef, useState } from "react";
import dynamic from "next/dynamic";
import type { FileTransfersHandle } from "@/components/canvas/runtime/components/assets/transfer/file-transfers-panel";
import { Field, FieldLabel } from "@/components/ui/field";
import { Button } from "@/components/ui/button";
import { Spinner } from "@/components/ui/spinner";
import { ResourceCover, type CoverReference } from "./resource-cover";

const FileTransfersPanel = dynamic(() =>
  import("@/components/canvas/runtime/components/assets/transfer/file-transfers-panel").then(
    (m) => m.FileTransfersPanel,
  ),
);

export function ResourceCoverField({
  title,
  kind,
  cover,
  disabled,
  uploading,
  onBusyChange,
  onChange,
  isCurrentActor,
}: {
  title: string;
  kind: "project" | "canvas";
  cover: CoverReference;
  disabled: boolean;
  uploading: boolean;
  onBusyChange: (busy: boolean) => void;
  onChange: (cover: CoverReference) => void;
  isCurrentActor: () => boolean;
}) {
  const id = useId();
  const upload = useRef<FileTransfersHandle>(null);
  const [open, setOpen] = useState(false);
  return (
    <Field data-disabled={disabled}>
      <FieldLabel>封面</FieldLabel>
      <div className="resource-edit-cover">
        <ResourceCover title={title} kind={kind} {...cover} showType={false} />
      </div>
      <div className="flex flex-wrap gap-2">
        <Button
          type="button"
          variant="outline"
          disabled={disabled}
          onClick={() => upload.current?.selectFile()}
        >
          {uploading && <Spinner data-icon="inline-start" />}
          {uploading ? "正在上传" : cover.coverFileId ? "更换封面" : "上传封面"}
        </Button>
        {cover.coverFileId && (
          <Button
            type="button"
            variant="ghost"
            disabled={disabled}
            onClick={() => onChange({ coverFileId: "", coverFileVersion: "" })}
          >
            移除
          </Button>
        )}
      </div>
      <FileTransfersPanel
        uploadRef={upload}
        uploadOpen={open}
        onUploadOpenChange={setOpen}
        onUploadStarted={() => undefined}
        onBusyChange={onBusyChange}
        disabled={disabled}
        showRecords={false}
        inputId={`resource-cover-${id}`}
        picker={{
          accept: ".jpg,.jpeg,.png,.webp",
          bizTypes: ["project_image"],
          label: "封面",
          description: "选择 JPEG、PNG 或 WebP 图片，最大 10 MiB。",
        }}
        onAdopt={async (file) => {
          if (!isCurrentActor()) return;
          if (
            !file.fileId ||
            !file.version ||
            file.status !== "READY" ||
            !["image/jpeg", "image/png", "image/webp"].includes(
              file.contentType ?? "",
            )
          )
            throw new Error("请选择已上传的JPEG、PNG或WebP图片");
          onChange({
            coverFileId: file.fileId,
            coverFileVersion: file.version,
          });
        }}
      />
    </Field>
  );
}
