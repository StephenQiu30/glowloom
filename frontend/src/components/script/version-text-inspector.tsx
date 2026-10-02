"use client";
import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { useForm } from "react-hook-form";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { versionSourceText } from "./version-queries";
import { reviewScopeKey } from "./review-queries";
import type { ScriptScope } from "./source-intent";
import type { ScriptVersionSummary } from "./review-model";
export function VersionTextInspector({
  scope,
  version,
  start = 0,
  end = version.char_count,
}: {
  scope: ScriptScope;
  version: ScriptVersionSummary;
  start?: number;
  end?: number;
}) {
  const [range, setRange] = useState<{ from: number; to: number }>();
  const [error, setError] = useState<string>();
  const form = useForm({
    defaultValues: { from: start, to: Math.min(end, start + 65536) },
  });
  const text = useQuery({
    queryKey: [
      ...reviewScopeKey(scope),
      "version-text",
      version.id,
      range?.from,
      range?.to,
    ],
    queryFn: ({ signal }) =>
      versionSourceText(scope, version, range!.from, range!.to, signal),
    enabled: Boolean(range),
    staleTime: 0,
    gcTime: 0,
  });
  return (
    <section
      className="space-y-3 rounded-lg border p-3"
      aria-label="规范正文片段阅读"
    >
      <p className="text-sm">
        原文区间 [{start}, {end})，每次最多 65,536 个 Unicode
        字符。选择片段后才读取私有正文；序言可在整版阅读。
      </p>
      <form
        className="flex flex-wrap items-end gap-3"
        onSubmit={form.handleSubmit(({ from, to }) => {
          if (
            !Number.isInteger(from) ||
            !Number.isInteger(to) ||
            from < start ||
            to > end ||
            to <= from ||
            to - from > 65536
          ) {
            setError("请选择本区间内最多 65,536 个字符的非空片段。");
            return;
          }
          setError(undefined);
          setRange({ from, to });
        })}
      >
        <label className="min-w-24 flex-1 text-sm">
          阅读起点
          <Input
            type="number"
            {...form.register("from", { valueAsNumber: true })}
          />
        </label>
        <label className="min-w-24 flex-1 text-sm">
          阅读终点
          <Input
            type="number"
            {...form.register("to", { valueAsNumber: true })}
          />
        </label>
        <Button
          type="submit"
          variant="outline"
          disabled={text.isFetching || end <= start}
        >
          读取所选规范正文
        </Button>
      </form>
      {error && <p role="alert">{error}</p>}
      {text.isFetching && <p role="status">正在核验并读取不可变原文…</p>}
      {text.error && (
        <>
          <p role="alert">{text.error.message}</p>
          <Button
            type="button"
            variant="outline"
            onClick={() => void text.refetch()}
          >
            重试读取同一片段
          </Button>
        </>
      )}
      {text.data && (
        <>
          <p className="text-xs break-all">
            整版正文 SHA：{text.data.content_hash}；区间 [{text.data.span_start}
            , {text.data.span_end})。
          </p>
          <pre
            className="max-h-72 overflow-auto break-words whitespace-pre-wrap"
            tabIndex={0}
          >
            {text.data.text}
          </pre>
        </>
      )}
    </section>
  );
}
