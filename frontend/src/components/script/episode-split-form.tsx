"use client";
import { useId, useState } from "react";
import { useFieldArray, useForm } from "react-hook-form";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  validateReviewedBoundaries,
  type EpisodeBoundary,
  type ScalarSpan,
} from "./review-model";
export type SplitBase = {
  version_id: string;
  expected_revision: number;
  expected_split_revision: number;
  candidate_set_id: string;
};
export type ReviewedSplitBody = SplitBase & {
  boundaries: EpisodeBoundary[];
  preface?: ScalarSpan;
  ack_invalidate: boolean;
};
export function EpisodeSplitForm({
  base,
  initialBoundaries,
  initialPreface,
  charCount,
  locked,
  onDirty,
  onSubmit,
}: {
  base: SplitBase;
  initialBoundaries: EpisodeBoundary[];
  initialPreface?: ScalarSpan;
  charCount: number;
  locked: boolean;
  onDirty: (dirty: boolean) => void;
  onSubmit: (body: ReviewedSplitBody) => void | Promise<void>;
}) {
  const id = useId();
  const [error, setError] = useState<string>();
  const [split, setSplit] = useState("");
  const [hasPreface, setHasPreface] = useState(Boolean(initialPreface));
  const form = useForm<{ boundaries: EpisodeBoundary[]; prefaceEnd: number }>({
    defaultValues: {
      boundaries: initialBoundaries,
      prefaceEnd: initialPreface?.end ?? 0,
    },
  });
  const rows = useFieldArray({ control: form.control, name: "boundaries" });
  const busy = locked || form.formState.isSubmitting;
  function replace(boundaries: EpisodeBoundary[]) {
    rows.replace(
      boundaries.map((item, index) => ({ ...item, seq_no: index + 1 })),
    );
    onDirty(true);
    setError(undefined);
  }
  return (
    <form
      className="space-y-4"
      onChange={() => onDirty(true)}
      onSubmit={form.handleSubmit(async (values) => {
        try {
          const preface = hasPreface
            ? { start: 0, end: values.prefaceEnd }
            : undefined;
          const boundaries = validateReviewedBoundaries(
            values.boundaries,
            preface,
            charCount,
          );
          setError(undefined);
          await onSubmit({
            ...base,
            boundaries,
            ...(preface ? { preface } : {}),
            ack_invalidate: false,
          });
        } catch (cause) {
          setError(
            cause instanceof Error ? cause.message : "分集边界未通过完整校验。",
          );
        }
      })}
    >
      <p className="text-sm">
        规范正文共 {charCount.toLocaleString("zh-CN")} 个 Unicode 字符。区间从 0
        开始，终点不包含在本集内；每个字符必须归入序言或某一集。
      </p>
      <div className="flex items-center gap-2">
        <Checkbox
          id={`${id}-preface`}
          checked={hasPreface}
          disabled={busy}
          onCheckedChange={(value) => {
            setHasPreface(value === true);
            onDirty(true);
          }}
        />
        <Label htmlFor={`${id}-preface`}>正文开头包含独立序言</Label>
      </div>
      {hasPreface && (
        <div className="space-y-1">
          <Label htmlFor={`${id}-preface-end`}>序言正文终点</Label>
          <Input
            id={`${id}-preface-end`}
            type="number"
            min={1}
            max={charCount - 1}
            disabled={busy}
            {...form.register("prefaceEnd", { valueAsNumber: true })}
          />
        </div>
      )}
      <ol className="max-h-[min(60dvh,640px)] space-y-3 overflow-y-auto">
        {rows.fields.map((row, index) => (
          <li key={row.id} className="space-y-2 rounded-lg border p-3">
            <div className="space-y-1">
              <Label htmlFor={`${id}-${index}-title`}>
                第{index + 1}集标题
              </Label>
              <Input
                id={`${id}-${index}-title`}
                disabled={busy}
                {...form.register(`boundaries.${index}.title`)}
              />
            </div>
            <div className="grid gap-3 sm:grid-cols-2">
              <div className="space-y-1">
                <Label htmlFor={`${id}-${index}-start`}>
                  第{index + 1}集正文起点
                </Label>
                <Input
                  id={`${id}-${index}-start`}
                  type="number"
                  min={0}
                  max={charCount}
                  disabled={busy}
                  {...form.register(`boundaries.${index}.span_start`, {
                    valueAsNumber: true,
                  })}
                />
              </div>
              <div className="space-y-1">
                <Label htmlFor={`${id}-${index}-end`}>
                  第{index + 1}集正文终点
                </Label>
                <Input
                  id={`${id}-${index}-end`}
                  type="number"
                  min={1}
                  max={charCount}
                  disabled={busy}
                  {...form.register(`boundaries.${index}.span_end`, {
                    valueAsNumber: true,
                  })}
                />
              </div>
            </div>
            {row.source_lineage_id && (
              <p className="text-xs break-all text-muted-foreground">
                原来源身份：{row.source_lineage_id}
              </p>
            )}
            <Button
              type="button"
              variant="outline"
              disabled={busy || rows.fields.length < 2}
              onClick={() => {
                const current = form.getValues("boundaries");
                const into = index === 0 ? 1 : index - 1;
                const removed = current[index];
                const target = current[into];
                const { source_lineage_id: lineage, ...merged } = target;
                current[into] = {
                  ...merged,
                  span_start: Math.min(target.span_start, removed.span_start),
                  span_end: Math.max(target.span_end, removed.span_end),
                  ...(lineage && lineage === removed.source_lineage_id
                    ? { source_lineage_id: lineage }
                    : {}),
                };
                replace(current.filter((_, position) => position !== index));
              }}
            >
              将第{index + 1}集合并至相邻集
            </Button>
          </li>
        ))}
      </ol>
      {!rows.fields.length && (
        <p>没有可确认的候选。读取正文后可为非空正文明确建立第 1 集。</p>
      )}
      <Label htmlFor={`${id}-split`}>插入新分集的正文位置</Label>
      <div className="flex flex-wrap gap-2">
        <Input
          id={`${id}-split`}
          className="w-40"
          type="number"
          min={1}
          max={charCount - 1}
          disabled={busy}
          value={split}
          onChange={(event) => setSplit(event.target.value)}
        />
        <Button
          type="button"
          variant="outline"
          disabled={busy || rows.fields.length >= 2500}
          onClick={() => {
            const position = Number(split);
            const current = form.getValues("boundaries");
            const index = current.findIndex(
              (boundary) =>
                position > boundary.span_start && position < boundary.span_end,
            );
            if (!split || !Number.isInteger(position) || index < 0) {
              setError("拆分位置必须位于某一现有分集区间内部。");
              return;
            }
            const target = current[index];
            replace([
              ...current.slice(0, index),
              { ...target, span_end: position },
              {
                ...target,
                seq_no: index + 2,
                title: `第 ${index + 2} 集`,
                span_start: position,
              },
              ...current.slice(index + 1),
            ]);
          }}
        >
          在此位置拆分来源区间
        </Button>
      </div>
      {!rows.fields.length && charCount > 0 && (
        <Button
          type="button"
          variant="outline"
          disabled={busy}
          onClick={() =>
            replace([
              {
                seq_no: 1,
                title: "第 1 集",
                span_start: hasPreface ? form.getValues("prefaceEnd") : 0,
                span_end: charCount,
              },
            ])
          }
        >
          以完整正文建立第一集草稿
        </Button>
      )}
      {error && (
        <p role="alert" className="text-sm text-destructive">
          {error}
        </p>
      )}
      <Button
        type="submit"
        disabled={busy || !rows.fields.length || !charCount}
      >
        {form.formState.isSubmitting ? "正在确认完整分集…" : "确认完整分集"}
      </Button>
    </form>
  );
}
