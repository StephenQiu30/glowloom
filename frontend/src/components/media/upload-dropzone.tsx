"use client";

import { useRef, useState } from "react";
import { Upload } from "lucide-react";
import { cn } from "cn";
import { Button } from "@/components/ui/button";
import {
  Empty,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from "@/components/ui/empty";

/** File selection surface. The owner keeps validation, consent and upload state. */
export function UploadDropzone({
  file,
  disabled = false,
  describedBy,
  onBrowse,
  onChoose,
  onError,
}: {
  file: File | null;
  disabled?: boolean;
  describedBy?: string;
  onBrowse: () => void;
  onChoose: (file: File) => void;
  onError: (message: string) => void;
}) {
  const [dragging, setDragging] = useState(false);
  const dragDepth = useRef(0);
  return (
    <Empty
      role="group"
      aria-label="文件上传区域"
      data-dragging={(dragging && !disabled) || undefined}
      className={cn(
        "relative h-[180px] flex-none border border-input bg-background p-5 sm:h-52",
        dragging && !disabled && "border-ring bg-accent",
      )}
      onDragEnter={(event) => {
        event.preventDefault();
        if (disabled || !event.dataTransfer.types.includes("Files")) return;
        dragDepth.current += 1;
        setDragging(true);
      }}
      onDragLeave={(event) => {
        event.preventDefault();
        dragDepth.current = Math.max(0, dragDepth.current - 1);
        if (!dragDepth.current) setDragging(false);
      }}
      onDragOver={(event) => {
        event.preventDefault();
        event.dataTransfer.dropEffect = disabled ? "none" : "copy";
      }}
      onDrop={(event) => {
        event.preventDefault();
        dragDepth.current = 0;
        setDragging(false);
        if (disabled) return;
        if (event.dataTransfer.files.length !== 1) {
          onError("请每次上传一个文件。");
          return;
        }
        onChoose(event.dataTransfer.files[0]);
      }}
    >
      <Button
        type="button"
        variant="ghost"
        className="absolute inset-0 h-full w-full rounded-xl"
        aria-label={file ? "更换上传文件" : "点击或拖拽上传"}
        title={file?.name}
        aria-describedby={describedBy}
        disabled={disabled}
        onClick={onBrowse}
      />
      <EmptyHeader className="pointer-events-none relative w-full min-w-0">
        <EmptyMedia variant="icon">
          <Upload />
        </EmptyMedia>
        <EmptyTitle>
          {dragging && !disabled
            ? "松开即可添加"
            : disabled
              ? "文件已保留"
              : file
                ? "点击或拖拽更换"
                : "点击或拖拽上传"}
        </EmptyTitle>
      </EmptyHeader>
      {file ? (
        <div
          className={cn(
            "relative flex w-full min-w-0 items-center justify-center gap-3 text-sm text-muted-foreground",
            !disabled && "pointer-events-none",
          )}
        >
          <span className="min-w-0 truncate" title={file.name}>
            {file.name}
          </span>
          <span className="shrink-0 whitespace-nowrap">
            {(file.size / 1024 / 1024).toFixed(2)} MB
          </span>
        </div>
      ) : null}
    </Empty>
  );
}
