"use client";

import { useState } from "react";
import type { RecoveryField } from "@glowloom/canvas/recovery";
import { toast } from "@/components/feedback/notification";
import { Button } from "@/components/ui/button";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";

type Source = { sourceId: string; fields: RecoveryField[] };
type Actions = {
  writable: boolean;
  commitSeq: string;
  apply: (id: string, fields: RecoveryField[], seq: string) => Promise<void>;
  discard: (id: string) => Promise<void>;
};

const object = (value: unknown): Record<string, unknown> =>
  value && typeof value === "object" ? (value as Record<string, unknown>) : {};

function documentText(value: unknown): string {
  const node = object(value);
  if (typeof node.text === "string") return node.text;
  if (Array.isArray(node.content))
    return node.content.map(documentText).join(node.type === "doc" ? "\n" : "");
  return "";
}

function preview(field: RecoveryField, cell: RecoveryField["after"]) {
  if (!cell.exists) return "已移除";
  if (typeof cell.value === "string") return cell.value || "空白";
  const data = object(cell.value);
  const config = object(data.config);
  const draft = object(config.draft);
  const palette = object(object(config.cinema_kit).palette).name;
  const prompt = documentText(object(draft.composer).document) || draft.prompt;
  if (field.field === "native")
    return (
      [
        typeof palette === "string" && palette,
        typeof prompt === "string" && prompt,
      ]
        .filter(Boolean)
        .join(" · ") || "素材与节点设置"
    );
  if (field.field === "form")
    return (
      documentText(object(object(data.canvasInput).draft).document) ||
      "生成参数"
    );
  if (field.field === "textDocument")
    return documentText(data.content) || "空白正文";
  if (field.field === "entity" && typeof data.title === "string")
    return data.title;
  return (
    {
      placement: "节点位置",
      size: "节点尺寸",
      fileReferences: "引用素材",
      parameters: "生成参数",
    }[field.field] ?? "画布编辑"
  );
}

function Draft({
  source,
  writable,
  commitSeq,
  apply,
  discard,
}: Actions & { source: Source }) {
  const [busy, setBusy] = useState(false);
  const [confirmDiscard, setConfirmDiscard] = useState(false);
  const run = async (action: () => Promise<void>) => {
    setBusy(true);
    try {
      await action();
    } catch {
      toast.error("草稿尚未处理，请重试");
    } finally {
      setBusy(false);
    }
  };
  return (
    <section className="grid gap-2 border-b pb-2 last:border-0 last:pb-0">
      {source.fields.map((field) => (
        <div className="grid gap-1" key={field.key}>
          <div className="line-clamp-3 break-words">
            {preview(field, field.after)}
          </div>
          {!field.copyRequired && (
            <span className="line-clamp-2 text-xs text-muted-foreground">
              当前：{preview(field, field.current)}
            </span>
          )}
          <Button
            className="justify-self-start"
            variant="secondary"
            size="xs"
            disabled={busy || !writable}
            onClick={() =>
              void run(() =>
                apply(
                  source.sourceId,
                  [{ ...field, copyNode: field.copyRequired }],
                  commitSeq,
                ),
              )
            }
          >
            {field.copyRequired ? "另存为新节点" : "应用这项修改"}
          </Button>
        </div>
      ))}
      <div className="flex items-center gap-1">
        <Button
          variant="ghost"
          size="xs"
          disabled={busy}
          onClick={() =>
            confirmDiscard
              ? void run(() => discard(source.sourceId))
              : setConfirmDiscard(true)
          }
        >
          {confirmDiscard ? "确认放弃这份草稿" : "放弃草稿"}
        </Button>
        {confirmDiscard && (
          <Button
            variant="ghost"
            size="xs"
            disabled={busy}
            onClick={() => setConfirmDiscard(false)}
          >
            取消
          </Button>
        )}
      </div>
    </section>
  );
}

export function CanvasLocalDrafts({
  sources,
  otherSessions = 0,
  ...actions
}: Actions & { sources: Source[]; otherSessions?: number }) {
  return (
    <Popover>
      <PopoverTrigger asChild>
        <Button size="xs" variant="ghost">
          本机草稿
        </Button>
      </PopoverTrigger>
      <PopoverContent
        align="start"
        aria-label="本机草稿"
        className="max-h-(--radix-popover-content-available-height) overflow-auto"
      >
        {otherSessions > 0 && (
          <p className="text-xs text-muted-foreground">
            其他会话还有 {otherSessions} 份本机草稿。
          </p>
        )}
        {sources.map((source) => (
          <Draft key={source.sourceId} source={source} {...actions} />
        ))}
      </PopoverContent>
    </Popover>
  );
}
