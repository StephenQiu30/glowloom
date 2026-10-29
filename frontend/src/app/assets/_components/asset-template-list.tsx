"use client";
import { useEffect, useState } from "react";
import { Scan } from "lucide-react";
import { listCanvasAssetTemplates } from "@/api/canvases";
import { AssetCardCover, AssetCardTitle } from "./asset-card-parts";
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
  if (pending) return null;
  if (error)
    return (
      <div className="col-span-full">
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
      </div>
    );
  return (
    <>
      {items.map((item) => (
        <Button
          key={item.key}
          variant="asset-card"
          size="asset-card"
          disabled={disabled}
          aria-label={`使用模板 · ${item.name}`}
          onClick={() => onUse(item)}
        >
          <AssetCardCover>
            <Scan aria-hidden="true" />
            <Badge variant="secondary" className="absolute top-2 left-2">
              模板
            </Badge>
          </AssetCardCover>
          <AssetCardTitle>{item.name}</AssetCardTitle>
        </Button>
      ))}
    </>
  );
}
