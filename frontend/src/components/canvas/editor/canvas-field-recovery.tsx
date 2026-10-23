"use client";

import { useId, useState } from "react";
import type { RecoveryField } from "@glowloom/canvas-collab/recovery";
import type { RecoverySource } from "@glowloom/canvas-collab/session";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { FieldLabel } from "@/components/ui/field";
import { notifyError } from "@/components/canvas/runtime/components/feedback/error-notification";

const names: Record<string, string> = {
  title: "名称",
  placement: "位置与分组",
  size: "尺寸",
  zIndex: "层级",
  collapsed: "折叠状态",
  parameters: "模型与参数",
  textDocument: "正文",
  fileReferences: "文件引用",
  entity: "节点或连线",
  form: "未应用参数",
};
function display(cell: RecoveryField["after"], field: string) {
  if (!cell.exists) return "删除或清空";
  if (field === "textDocument") {
    const text = (value: unknown): string => {
      if (!value || typeof value !== "object") return "";
      const node = value as {
        text?: string;
        type?: string;
        attrs?: { label?: string };
        content?: unknown[];
      };
      return (
        node.text ??
        (node.type === "mention"
          ? (node.attrs?.label ?? "引用")
          : (node.content?.map(text).join(" ") ?? ""))
      );
    };
    return text((cell.value as { content?: unknown })?.content) || "空白正文";
  }
  return typeof cell.value === "string"
    ? cell.value
    : JSON.stringify(cell.value, null, 2);
}

export function CanvasFieldRecovery({
  source,
  busy,
  apply,
  discard,
}: {
  source: RecoverySource<CanvasContract.CanvasDocumentGraphDTO> & {
    fields: RecoveryField[];
  };
  busy: boolean;
  apply: (id: string, fields: RecoveryField[]) => Promise<void>;
  discard: (id: string) => Promise<void>;
}) {
  const prefix = useId();
  const [selected, setSelected] = useState<Record<string, RecoveryField>>({});
  const values = Object.values(selected);
  const stale = values.some((chosen) => {
    const current = source.fields.find((field) => field.key === chosen.key);
    return (
      !current ||
      current.generation !== chosen.generation ||
      current.targetId !== chosen.targetId ||
      JSON.stringify(current.current) !== JSON.stringify(chosen.current) ||
      JSON.stringify(current.after) !== JSON.stringify(chosen.after)
    );
  });
  return (
    <section aria-label="保留输入逐项处理" className="grid gap-3 border-t pt-3">
      <p className="text-sm">
        保留输入 · 保存版本 {source.confirmed.contentRevision}
      </p>
      {source.fields.length === 0 && (
        <p className="text-sm text-muted-foreground">
          所选输入已处理。原稿可导出，或明确放弃。
        </p>
      )}
      {source.fields.map((field, index) => {
        const label = field.field.startsWith("value:")
          ? field.field.slice(6)
          : (names[field.field] ?? field.field);
        const node = source.sealed.snapshot.graph.nodes.find(
          (node) => node.id === field.id,
        );
        return (
          <div key={field.key} className="grid gap-2 rounded-md border p-2">
            <FieldLabel
              htmlFor={`${prefix}-${index}`}
              className="flex items-start gap-2 text-sm"
            >
              <Checkbox
                id={`${prefix}-${index}`}
                checked={!!selected[field.key]}
                disabled={busy}
                onCheckedChange={(checked) =>
                  setSelected((previous) => {
                    const next = { ...previous };
                    if (checked)
                      next[field.key] = {
                        ...field,
                        copyNode: field.copyRequired,
                      };
                    else delete next[field.key];
                    return next;
                  })
                }
              />
              <span>
                {field.collection === "canvas"
                  ? "画布"
                  : (node?.title ?? "连线")}{" "}
                · {label}
                {field.copyRequired && "（复制为新节点）"}
              </span>
            </FieldLabel>
            <dl className="grid gap-1 text-xs">
              <dt className="text-muted-foreground">当前内容</dt>
              <dd className="max-h-24 overflow-auto break-words whitespace-pre-wrap">
                {display(field.current, field.field)}
              </dd>
              <dt className="text-muted-foreground">保留输入</dt>
              <dd className="max-h-32 overflow-auto break-words whitespace-pre-wrap">
                {display(field.after, field.field)}
              </dd>
            </dl>
          </div>
        );
      })}
      {stale && (
        <p role="status" className="text-sm">
          所选内容或当前值已变化，请重新选择。
        </p>
      )}
      <Button
        disabled={busy || !values.length || stale}
        onClick={() =>
          void apply(source.id, values)
            .then(() => setSelected({}))
            .catch((error) => notifyError(error, "所选输入尚未处理"))
        }
      >
        应用所选输入
      </Button>
      {stale && (
        <Button
          variant="outline"
          disabled={busy}
          onClick={() => setSelected({})}
        >
          刷新对照
        </Button>
      )}
      <Button
        variant="outline"
        disabled={busy}
        onClick={() =>
          void discard(source.id).catch((error) =>
            notifyError(error, "保留稿尚未放弃"),
          )
        }
      >
        放弃这份保留稿
      </Button>
    </section>
  );
}
