"use client";
import { useEffect, useState } from "react";
import { listCanvasAssetTemplates } from "@/api/canvases";
import { PanelsTopLeft } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { ContentState } from "@/components/layout/content-state";
import { RequestError } from "@/lib/request";
import { readErrorMessage } from "@/lib/read-error";
export type AssetTemplate = {
  key: string;
  kind: "canvas" | "short_drama";
  name: string;
  description: string;
  is_template: true;
  node_count: number;
  requires_model: boolean;
};
export function AssetTemplateList({
  kind,
  disabled,
  onUse,
}: {
  kind: "canvas" | "short_drama";
  disabled: boolean;
  onUse: (template: AssetTemplate) => void;
}) {
  const [items, setItems] = useState<AssetTemplate[]>([]),
    [pending, setPending] = useState(true),
    [error, setError] = useState(""),
    [retry, setRetry] = useState(0);
  useEffect(() => {
    const controller = new AbortController();
    listCanvasAssetTemplates({ kind }, { signal: controller.signal })
      .then((result) => {
        if (
          !Array.isArray(result.items) ||
          result.items.some(
            (item) =>
              !item.key ||
              item.kind !== kind ||
              typeof item.name !== "string" ||
              !item.name.trim() ||
              item.is_template !== true ||
              !Number.isSafeInteger(item.node_count) ||
              item.node_count! < 1 ||
              typeof item.requires_model !== "boolean" ||
              typeof item.description !== "string",
          )
        )
          throw new RequestError("invalid_receipt");
        if (!controller.signal.aborted) {
          setItems(result.items as AssetTemplate[]);
          setError("");
        }
      })
      .catch((reason) => {
        if (!controller.signal.aborted) setError(readErrorMessage(reason));
      })
      .finally(() => {
        if (!controller.signal.aborted) setPending(false);
      });
    return () => controller.abort();
  }, [kind, retry]);
  return (
    <section aria-label="系统模板" className="grid min-w-0 gap-4">
      <h2 className="text-sm font-medium">系统模板</h2>
      {pending ? (
        <ContentState state="loading" title="正在读取模板" />
      ) : error ? (
        <ContentState
          state="error"
          title="暂时无法读取模板"
          description={error}
          actions={
            <Button
              disabled={disabled}
              onClick={() => {
                setPending(true);
                setRetry((value) => value + 1);
              }}
            >
              重试模板
            </Button>
          }
        />
      ) : (
        <div className="grid min-w-0 grid-cols-2 gap-4 lg:grid-cols-3 xl:grid-cols-4">
          {items.map((item) => (
            <article
              key={item.key}
              className="grid min-w-0 gap-3 rounded-lg border p-4"
            >
              <div className="flex items-center justify-between gap-2">
                <PanelsTopLeft className="size-6 text-muted-foreground" />
                <Badge variant="secondary">模板</Badge>
              </div>
              <h3 className="text-sm font-medium break-words">{item.name}</h3>
              <p className="text-xs break-words text-muted-foreground">
                {item.description}
              </p>
              <p className="text-xs text-muted-foreground">
                {item.node_count}个可编辑节点
                {item.requires_model ? " · 需自行选择模型" : ""}
              </p>
              <Button
                variant="secondary"
                disabled={disabled}
                onClick={() => onUse(item)}
              >
                使用模板
              </Button>
            </article>
          ))}
        </div>
      )}
    </section>
  );
}
