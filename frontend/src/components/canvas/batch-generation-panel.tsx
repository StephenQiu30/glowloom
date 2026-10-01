"use client";

import { useRef, useState } from "react";
import Link from "next/link";
import { useInfiniteQuery, useQueryClient } from "@tanstack/react-query";
import { ModelParamsForm } from "@/components/catalog/model-params-form";
import { MODELS_KEY, queryModels } from "@/components/catalog/queries";
import {
  QuoteConfirmDialog,
  type QuoteResponse,
} from "@/components/operation/quote-confirm-dialog";
import {
  OPERATIONS_KEY,
  confirmGeneration,
  quoteGeneration,
  type GenerationItem,
  type GenerationSource,
} from "@/components/operation/queries";
import { Button } from "@/components/ui/button";
import { Field, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import type { CanvasNodeData } from "./model";
import type { BatchTableConfig } from "./batch-table";
import { batchGenerationItems } from "./batch-generation";

export function BatchGenerationPanel({
  projectId,
  source,
  retryRowId,
  onRetryCleared,
  config,
  nodes,
  disabled,
  onChange,
  onPersist,
  onBusy,
}: {
  projectId: string;
  source: () => GenerationSource;
  retryRowId?: string;
  onRetryCleared: () => void;
  config: BatchTableConfig;
  nodes: CanvasNodeData[];
  disabled: boolean;
  onChange: (config: BatchTableConfig) => void;
  onPersist: (config: BatchTableConfig) => Promise<boolean>;
  onBusy: (busy: boolean) => void;
}) {
  const [page, setPage] = useState(0);
  const [quoting, setQuoting] = useState(false);
  const [error, setError] = useState("");
  const [quote, setQuote] = useState<QuoteResponse>();
  const [quoteOpen, setQuoteOpen] = useState(false);
  const [confirmed, setConfirmed] = useState(false);
  const pending = useRef<{ items: GenerationItem[]; labels: string[] } | null>(
    null,
  );
  const confirmation = useRef<string | null>(null);
  const cache = useQueryClient();
  const catalogue = useInfiniteQuery({
    queryKey: [...MODELS_KEY, "canvas-batch", projectId, "image"],
    initialPageParam: undefined as string | undefined,
    queryFn: ({ pageParam, signal }) =>
      queryModels(projectId, undefined, signal, pageParam),
    getNextPageParam: (data) => data.next_cursor ?? undefined,
    retry: false,
  });
  const models =
    catalogue.data?.pages
      .flatMap((data) => data.items)
      .filter(
        (model) =>
          model.capability === "image.generate" ||
          model.capability === "image.edit",
      ) ?? [];
  const model = models.find((item) => item.id === config.modelProfileId);
  const pages = Math.max(
    1,
    Math.ceil(config.rows.filter((row) => row.enabled).length / 150),
  );
  const currentPage = Math.min(page, pages - 1);
  const scalarParams = Object.fromEntries(
    Object.entries(config.params).filter(
      (entry): entry is [string, string | number | boolean] =>
        ["string", "number", "boolean"].includes(typeof entry[1]),
    ),
  );
  const busy = disabled || quoting || quoteOpen;
  async function requestQuote(
    params: Record<string, string | number | boolean>,
  ) {
    if (!model?.current_version || busy) return;
    setQuoting(true);
    onBusy(true);
    setError("");
    setConfirmed(false);
    let opened = false;
    try {
      const next = { ...config, params };
      const prepared = batchGenerationItems(
        next,
        nodes,
        {
          key: model.key,
          capability: model.capability,
          modes: model.current_version.modes,
          inputRoles: model.input_roles,
        },
        currentPage,
        retryRowId,
      );
      onChange(next);
      if (!(await onPersist(next)))
        throw new Error("请先确认表格保存成功，再预览费用。");
      const savedSource = source();
      prepared.items = prepared.items.map((item, index) => ({
        ...item,
        source: { ...savedSource, row_id: prepared.rowIds[index] },
      }));
      pending.current = prepared;
      const result = await quoteGeneration(
        projectId,
        prepared.items,
        crypto.randomUUID(),
        prepared.labels,
      );
      confirmation.current = crypto.randomUUID();
      setQuote(result);
      setQuoteOpen(true);
      opened = true;
    } catch (failure) {
      setError(
        failure instanceof Error ? failure.message : "报价失败，请重试。",
      );
    } finally {
      setQuoting(false);
      onBusy(opened);
    }
  }
  async function requote() {
    if (!pending.current || quoting) return;
    setQuoting(true);
    onBusy(true);
    try {
      const current = source();
      pending.current.items = pending.current.items.map((item) => ({
        ...item,
        source: {
          ...current,
          ...(item.source?.row_id ? { row_id: item.source.row_id } : {}),
        },
      }));
      const result = await quoteGeneration(
        projectId,
        pending.current.items,
        crypto.randomUUID(),
        pending.current.labels,
      );
      confirmation.current = crypto.randomUUID();
      setQuote(result);
    } catch {
      setError("费用预览未能刷新，请稍后重试。");
    } finally {
      setQuoting(false);
      onBusy(quoteOpen);
    }
  }
  return (
    <div className="space-y-4 rounded border p-4">
      {retryRowId ? (
        <p className="text-sm">
          预览单行重试费用。
          <Button
            variant="ghost"
            size="sm"
            disabled={busy}
            onClick={onRetryCleared}
          >
            返回整批
          </Button>
        </p>
      ) : null}
      <div className="grid gap-3 sm:grid-cols-[1fr_180px_110px_130px]">
        <Field>
          <FieldLabel>生成模型</FieldLabel>
          <Select
            value={config.modelProfileId ?? ""}
            disabled={busy || catalogue.isPending || catalogue.isError}
            onValueChange={(modelProfileId) => {
              setConfirmed(false);
              onChange({ ...config, modelProfileId, mode: "", params: {} });
            }}
          >
            <SelectTrigger aria-label="批量生成模型">
              <SelectValue placeholder="请选择模型" />
            </SelectTrigger>
            <SelectContent>
              {models.map((item) => (
                <SelectItem
                  key={item.id}
                  value={item.id}
                  disabled={!item.current_version || item.status !== "active"}
                >
                  {item.display_name}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          {catalogue.hasNextPage ? (
            <Button
              size="sm"
              variant="ghost"
              disabled={catalogue.isFetchingNextPage}
              onClick={() => {
                void catalogue.fetchNextPage();
              }}
            >
              加载更多模型
            </Button>
          ) : null}
        </Field>
        <Field>
          <FieldLabel>生成方式</FieldLabel>
          <Select
            value={config.mode}
            disabled={busy || !model?.current_version}
            onValueChange={(mode) => onChange({ ...config, mode, params: {} })}
          >
            <SelectTrigger aria-label="批量生成方式">
              <SelectValue placeholder="请选择方式" />
            </SelectTrigger>
            <SelectContent>
              {model?.current_version?.modes.map((mode) => (
                <SelectItem key={mode} value={mode}>
                  {mode}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </Field>
        <Field>
          <FieldLabel htmlFor="batch-output-count">每行输出</FieldLabel>
          <Input
            id="batch-output-count"
            type="number"
            min={1}
            max={8}
            value={config.outputCount}
            disabled={busy}
            onChange={(event) =>
              onChange({ ...config, outputCount: Number(event.target.value) })
            }
          />
        </Field>
        <Field>
          <FieldLabel>当前批次</FieldLabel>
          <Select
            value={String(currentPage)}
            disabled={busy}
            onValueChange={(value) => setPage(Number(value))}
          >
            <SelectTrigger aria-label="批量当前批次">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {Array.from({ length: pages }, (_, index) => (
                <SelectItem key={index} value={String(index)}>
                  第 {index + 1} / {pages} 批
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </Field>
      </div>
      {catalogue.error ? (
        <div role="alert" className="text-sm">
          模型目录读取失败。
          <Button
            size="sm"
            variant="ghost"
            onClick={() => {
              void catalogue.refetch();
            }}
          >
            重试
          </Button>
        </div>
      ) : null}
      {model?.current_version && config.mode ? (
        <ModelParamsForm
          modelKey={model.key}
          profileVersionId={model.current_version.id}
          mode={config.mode}
          schema={model.current_version.param_schema}
          initialValues={scalarParams}
          disabled={busy || !config.rows.some((row) => row.enabled)}
          submitLabel={
            quoting
              ? "准备费用预览…"
              : retryRowId
                ? "保存参数并预览本行重试费用"
                : "保存参数并预览本批费用"
          }
          onSubmit={(params) => {
            void requestQuote(params);
          }}
        />
      ) : (
        <p className="text-xs text-muted-foreground">
          选择模型和生成方式后设置参数。每批最多 150 行，费用确认后开始生成。
        </p>
      )}
      {error ? (
        <p role="alert" className="text-sm text-destructive">
          {error}
        </p>
      ) : null}
      {confirmed ? (
        <p role="status" className="text-sm">
          本批已确认。
          <Link
            href={`/tasks?project_id=${projectId}`}
            className="ml-2 underline"
          >
            查看项目任务
          </Link>
        </p>
      ) : null}
      {quote ? (
        <QuoteConfirmDialog
          open={quoteOpen}
          onOpenChange={(open) => {
            setQuoteOpen(open);
            onBusy(open);
          }}
          quote={quote}
          onRequote={() => {
            void requote();
          }}
          onConfirm={async (excluded) => {
            onBusy(true);
            try {
              await confirmGeneration(
                projectId,
                quote,
                excluded,
                confirmation.current!,
              );
              setConfirmed(true);
              setQuoteOpen(false);
              await cache.invalidateQueries({
                queryKey: [...OPERATIONS_KEY, projectId],
              });
            } finally {
              onBusy(false);
            }
          }}
        />
      ) : null}
    </div>
  );
}
