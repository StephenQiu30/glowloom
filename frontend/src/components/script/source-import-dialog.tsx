"use client";

import { useId, useState, type ReactNode } from "react";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Textarea } from "@/components/ui/textarea";
import { parseSourceImport, type SourceImportMode } from "./source-import";
import { sourceCommandSchema, type SourceCommand } from "./source-model";
type ImportBody = Extract<SourceCommand, { action: "import" }>["body"];
export function SourceImportDialog({
  open,
  base,
  locked,
  onClose,
  onSubmit,
  onDirty,
  feedback,
}: {
  open: boolean;
  base: { expected_revision: number; base_version_id?: string };
  locked: boolean;
  onClose: () => void;
  onSubmit: (body: ImportBody) => Promise<void> | void;
  onDirty: (dirty: boolean) => void;
  feedback?: ReactNode;
}) {
  const id = useId();
  const [text, setText] = useState("");
  const [mode, setMode] = useState<SourceImportMode>("typed");
  const [rights, setRights] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [reading, setReading] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [discardRequested, setDiscardRequested] = useState(false);
  const blocked = locked || reading || submitting;
  function close() {
    if (blocked) return;
    if (text) {
      setDiscardRequested(true);
      return;
    }
    onClose();
  }
  return (
    <Dialog
      open={open}
      onOpenChange={(value) => {
        if (!value) close();
      }}
    >
      <DialogContent
        showCloseButton={!blocked}
        className="max-h-[calc(100dvh-2rem)] overflow-y-auto sm:max-w-3xl"
        onEscapeKeyDown={(event) => {
          if (blocked || text) {
            event.preventDefault();
            close();
          }
        }}
        onInteractOutside={(event) => {
          if (blocked || text) {
            event.preventDefault();
            close();
          }
        }}
      >
        <DialogHeader>
          <DialogTitle>完整章节批量导入</DialogTitle>
          <DialogDescription>
            一次 1 至 2500
            章，全部预校验后原子写入新版本。错误保留这份输入。正文文件请使用原文导入入口。
          </DialogDescription>
        </DialogHeader>
        <Label htmlFor={`${id}-mode`}>章节数据格式</Label>
        <Select
          value={mode}
          disabled={blocked}
          onValueChange={(value) => {
            if (value === "typed" || value === "beeftv") {
              setMode(value);
              setError(null);
            }
          }}
        >
          <SelectTrigger id={`${id}-mode`} aria-label="章节数据格式">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="typed">规范富文本章节 JSON</SelectItem>
            <SelectItem value="beeftv">BeefTV 完整章节 JSON</SelectItem>
          </SelectContent>
        </Select>
        <p className="text-sm text-muted-foreground">
          {mode === "typed"
            ? "输入数组，每章包含 source_kind、title、status、document 或 original_html（二选一）、provenance。"
            : "输入 ProjectUnit 数组，原 HTML 保留；id 与 position 保存为来源标签，当前系统生成自己的 UUID 与规范正文计数。"}
        </p>
        <Label htmlFor={`${id}-file`}>读取章节 JSON 文件</Label>
        <Input
          id={`${id}-file`}
          type="file"
          accept=".json,application/json"
          disabled={blocked}
          onChange={async (event) => {
            const file = event.target.files?.[0];
            if (!file) return;
            if (
              !file.name.toLowerCase().endsWith(".json") ||
              file.size > 36 * 1024 * 1024
            ) {
              setError("请选择不超过 36MiB 的 JSON 文件。");
              return;
            }
            setReading(true);
            try {
              const decoded = new TextDecoder("utf-8", { fatal: true }).decode(
                await file.arrayBuffer(),
              );
              setText(decoded);
              onDirty(true);
              setError(null);
            } catch {
              setError("文件不是有效 UTF-8 JSON，原输入保留。");
            } finally {
              setReading(false);
            }
          }}
        />
        <Label htmlFor={`${id}-json`}>完整章节 JSON</Label>
        <Textarea
          id={`${id}-json`}
          value={text}
          disabled={blocked}
          className="min-h-60 font-mono text-sm"
          onChange={(event) => {
            setText(event.target.value);
            onDirty(true);
            setError(null);
            setDiscardRequested(false);
          }}
        />
        <div className="flex items-start gap-2">
          <input
            id={`${id}-rights`}
            type="checkbox"
            checked={rights}
            disabled={blocked}
            className="mt-0.5 size-4 accent-current"
            onChange={(event) => setRights(event.target.checked)}
          />
          <Label htmlFor={`${id}-rights`}>已获授权使用全部章节正文</Label>
        </div>
        {error && (
          <p role="alert" className="text-sm text-destructive">
            {error}
          </p>
        )}
        {discardRequested && (
          <p role="alert" className="text-sm text-destructive">
            这份章节草稿尚未提交。仅明确放弃后关闭。
          </p>
        )}
        {feedback}
        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            disabled={blocked}
            onClick={close}
          >
            关闭导入
          </Button>
          {discardRequested && (
            <Button
              type="button"
              variant="destructive"
              disabled={blocked}
              onClick={() => {
                onDirty(false);
                onClose();
              }}
            >
              放弃此草稿并关闭
            </Button>
          )}
          <Button
            type="button"
            disabled={blocked || !rights}
            onClick={async () => {
              setError(null);
              setSubmitting(true);
              try {
                const sources = parseSourceImport(text, mode);
                const command = sourceCommandSchema.parse({
                  action: "import",
                  body: { ...base, rights_confirmed: true, sources },
                });
                if (command.action === "import") await onSubmit(command.body);
              } catch (failure) {
                setError(
                  failure instanceof Error
                    ? failure.message
                    : "批量输入未通过校验。请保留原稿后检查。",
                );
              } finally {
                setSubmitting(false);
              }
            }}
          >
            {submitting ? "正在确认导入…" : "原子导入全部章节"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
