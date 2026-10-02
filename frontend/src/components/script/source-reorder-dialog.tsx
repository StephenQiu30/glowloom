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
import { SourceList } from "./source-list";
import { moveSourcePosition, reviewSourceOrder } from "./source-order";
import type { SourceCommand, SourceSummary } from "./source-model";
type ReorderBody = Extract<SourceCommand, { action: "reorder" }>["body"];
export function SourceReorderDialog({
  items,
  base,
  locked,
  onClose,
  onSubmit,
  onDirty,
  feedback,
  latestItems,
  onAcceptLatest,
}: {
  items: SourceSummary[];
  base: { expected_revision: number; base_version_id?: string };
  locked: boolean;
  onClose: () => void;
  onSubmit: (body: ReorderBody) => void | Promise<void>;
  onDirty: (dirty: boolean) => void;
  feedback?: ReactNode;
  latestItems?: SourceSummary[];
  onAcceptLatest?: () => void;
}) {
  const id = useId();
  const [currentItems, setCurrentItems] = useState(items);
  const [order, setOrder] = useState(() =>
    items.map((item) => item.source_lineage_id),
  );
  const [selected, setSelected] = useState(items[0]?.source_lineage_id);
  const [position, setPosition] = useState("1");
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);
  const [discard, setDiscard] = useState(false);
  const blocked = locked || submitting;
  const changed = order.some(
    (lineage, index) => lineage !== currentItems[index]?.source_lineage_id,
  );
  const lookup = new Map(
    currentItems.map((item) => [item.source_lineage_id, item]),
  );
  const latestLookup = new Map(
    latestItems?.map((item) => [item.source_lineage_id, item]),
  );
  const review = latestItems
    ? reviewSourceOrder(
        order,
        latestItems.map((item) => item.source_lineage_id),
      )
    : null;
  const ordered = order.map((lineage, index) => ({
    ...lookup.get(lineage)!,
    position: index,
  }));
  function close() {
    if (blocked) return;
    if (changed) {
      setDiscard(true);
      return;
    }
    onClose();
  }
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) close();
      }}
    >
      <DialogContent
        showCloseButton={!blocked}
        className="max-h-[calc(100dvh-2rem)] overflow-y-auto sm:max-w-2xl"
        onEscapeKeyDown={(event) => {
          if (blocked || changed) {
            event.preventDefault();
            close();
          }
        }}
        onInteractOutside={(event) => {
          if (blocked || changed) {
            event.preventDefault();
            close();
          }
        }}
      >
        <DialogHeader>
          <DialogTitle>调整完整来源顺序</DialogTitle>
          <DialogDescription>
            已读取同一不可变版本的全部{" "}
            {currentItems.length.toLocaleString("zh-CN")}{" "}
            个来源。确认前仅调整本页草稿；保存一次提交完整身份集合与当前版本。
          </DialogDescription>
        </DialogHeader>
        <SourceList
          items={ordered}
          selected={selected}
          locked={blocked}
          nextPage={false}
          loadingNext={false}
          onSelect={(lineage) => {
            setSelected(lineage);
            setPosition(String(order.indexOf(lineage) + 1));
          }}
          onNext={() => {}}
        />
        <Label htmlFor={id}>选中来源移至第几个位置</Label>
        <div className="flex gap-2">
          <Input
            id={id}
            inputMode="numeric"
            value={position}
            disabled={blocked || !selected}
            onChange={(event) => setPosition(event.target.value)}
          />
          <Button
            type="button"
            variant="outline"
            disabled={blocked || !selected}
            onClick={() => {
              try {
                if (!selected) return;
                const next = moveSourcePosition(
                  order,
                  selected,
                  Number(position) - 1,
                );
                setOrder(next);
                setError(null);
                setDiscard(false);
                onDirty(true);
              } catch {
                setError(`请输入 1 至 ${currentItems.length} 的完整来源位置。`);
              }
            }}
          >
            应用位置草稿
          </Button>
        </div>
        {error && (
          <p role="alert" className="text-sm text-destructive">
            {error}
          </p>
        )}
        {discard && (
          <p role="alert" className="text-sm text-destructive">
            来源顺序草稿未保存，请明确放弃后关闭。
          </p>
        )}
        {feedback}
        {review && latestItems && onAcceptLatest && (
          <section
            className="space-y-2 rounded-lg border p-3"
            aria-label="审阅最新来源集合"
          >
            <p>
              最新集合共 {latestItems.length}{" "}
              个来源。仍存在的来源保留当前顺序草稿，新增来源按最新顺序放在末尾。
            </p>
            {review.removed.map((lineage) => (
              <p key={lineage} className="text-sm break-all">
                已移除来源：{lookup.get(lineage)?.title} · {lineage}
              </p>
            ))}
            {review.added.map((lineage) => (
              <p key={lineage} className="text-sm break-all">
                新增来源：{latestLookup.get(lineage)?.title} · {lineage}
              </p>
            ))}
            <Button
              type="button"
              variant="outline"
              disabled={submitting}
              onClick={() => {
                setOrder(review.order);
                setCurrentItems(latestItems);
                setSelected(review.order[0]);
                setPosition("1");
                setError(null);
                onDirty(true);
                onAcceptLatest();
              }}
            >
              采用最新集合并保留可用顺序草稿
            </Button>
          </section>
        )}
        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            disabled={blocked}
            onClick={close}
          >
            关闭顺序调整
          </Button>
          {discard && (
            <Button
              type="button"
              variant="destructive"
              disabled={blocked}
              onClick={() => {
                onDirty(false);
                onClose();
              }}
            >
              放弃顺序草稿并关闭
            </Button>
          )}
          <Button
            type="button"
            disabled={blocked || !changed}
            onClick={async () => {
              setSubmitting(true);
              try {
                await onSubmit({ ...base, source_lineage_ids: order });
              } finally {
                setSubmitting(false);
              }
            }}
          >
            {submitting ? "正在确认顺序…" : "保存完整来源顺序"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
