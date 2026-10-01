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
import { batchGenerationGroups, mergeBatchQuotes } from "./batch-generation";
import { ApiError } from "@/lib/request";

type QuoteGroup = {
  prepared: ReturnType<typeof batchGenerationGroups>[number];
  quoteKey: string;
  confirmKey: string;
  quote?: QuoteResponse;
  confirmed: boolean;
  uncertain: boolean;
  excluded?: string[];
};

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
  const [quoting, setQuoting] = useState(false);
  const [error, setError] = useState("");
  const [quote, setQuote] = useState<QuoteResponse>();
  const [quoteOpen, setQuoteOpen] = useState(false);
  const [confirmed, setConfirmed] = useState(false);
  const pending = useRef<QuoteGroup[]>([]);
  const [confirmedRows, setConfirmedRows] = useState(0);
  const [uncertain, setUncertain] = useState(false);
  const [quoteProgress, setQuoteProgress] = useState("");
  const activeRequest = useRef(false);
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
  const scalarParams = Object.fromEntries(
    Object.entries(config.params).filter(
      (entry): entry is [string, string | number | boolean] =>
        ["string", "number", "boolean"].includes(typeof entry[1]),
    ),
  );
  const busy = disabled || quoting || quoteOpen || uncertain;
  async function quoteGroups() {
    const remaining = pending.current.filter((group) => !group.confirmed);
    const current = source();
    for (const [index, group] of remaining.entries()) {
      if (group.uncertain)
        throw new Error("上次确认结果尚未核实，请先核验原请求。");
      setQuoteProgress(`正在报价第 ${index + 1} / ${remaining.length} 组`);
      group.prepared.items = group.prepared.items.map((item, rowIndex) => ({
        ...item,
        source: { ...current, row_id: group.prepared.rowIds[rowIndex] },
      }));
      group.quote = await quoteGeneration(
        projectId,
        group.prepared.items,
        group.quoteKey,
        group.prepared.labels,
      );
    }
    setQuote(mergeBatchQuotes(remaining.map((group) => group.quote!)));
    setQuoteOpen(true);
  }
  async function requestQuote(
    params: Record<string, string | number | boolean>,
  ) {
    if (activeRequest.current || !model?.current_version || busy) return;
    activeRequest.current = true;
    setQuoting(true);
    onBusy(true);
    setError("");
    setConfirmed(false);
    setConfirmedRows(0);
    let opened = false;
    try {
      const next = { ...config, params };
      const prepared = batchGenerationGroups(
        next,
        nodes,
        {
          key: model.key,
          capability: model.capability,
          modes: model.current_version.modes,
          inputRoles: model.input_roles,
        },
        retryRowId,
      );
      onChange(next);
      if (!(await onPersist(next)))
        throw new Error("请先确认表格保存成功，再预览费用。");
      pending.current = prepared.map((prepared) => ({
        prepared,
        quoteKey: crypto.randomUUID(),
        confirmKey: crypto.randomUUID(),
        confirmed: false,
        uncertain: false,
      }));
      await quoteGroups();
      opened = true;
    } catch (failure) {
      setError(
        failure instanceof Error ? failure.message : "报价失败，请重试。",
      );
    } finally {
      activeRequest.current = false;
      setQuoting(false);
      setQuoteProgress("");
      onBusy(opened);
    }
  }
  async function requote() {
    if (
      activeRequest.current ||
      !pending.current.length ||
      quoting ||
      uncertain
    )
      return;
    activeRequest.current = true;
    setQuoting(true);
    onBusy(true);
    setError("");
    let opened = false;
    try {
      if (!model?.current_version) throw new Error("请选择有效生成模型。");
      const remainingIds = new Set(
        pending.current
          .filter((group) => !group.confirmed)
          .flatMap((group) => group.prepared.rowIds),
      );
      const all = batchGenerationGroups(
        config,
        nodes,
        {
          key: model.key,
          capability: model.capability,
          modes: model.current_version.modes,
          inputRoles: model.input_roles,
        },
        retryRowId,
      );
      const remaining = all
        .flatMap((group) =>
          group.items.map((item, index) => ({
            item,
            rowId: group.rowIds[index],
            label: group.labels[index],
          })),
        )
        .filter((row) => remainingIds.has(row.rowId));
      if (!remaining.length)
        throw new Error("未确认行已关闭，当前没有需要报价的行。");
      if (!(await onPersist(config)))
        throw new Error("请先确认当前表格配置已保存，再为未确认行报价。");
      const groups: QuoteGroup[] = Array.from(
        { length: Math.ceil(remaining.length / 150) },
        (_, page) => {
          const rows = remaining.slice(page * 150, (page + 1) * 150);
          return {
            prepared: {
              items: rows.map((row) => row.item),
              rowIds: rows.map((row) => row.rowId),
              labels: rows.map((row) => row.label),
            },
            quoteKey: crypto.randomUUID(),
            confirmKey: crypto.randomUUID(),
            confirmed: false,
            uncertain: false,
          };
        },
      );
      pending.current = [
        ...pending.current.filter((group) => group.confirmed),
        ...groups,
      ];
      await quoteGroups();
      opened = true;
    } catch (failure) {
      setError(
        failure instanceof Error ? failure.message : "未确认部分报价未能刷新。",
      );
    } finally {
      activeRequest.current = false;
      setQuoting(false);
      setQuoteProgress("");
      onBusy(opened);
    }
  }
  function acceptedCount() {
    return pending.current
      .filter((group) => group.confirmed)
      .reduce(
        (sum, group) =>
          sum +
          group.quote!.items.filter(
            (item) =>
              item.operation_id && !group.excluded?.includes(item.operation_id),
          ).length,
        0,
      );
  }
  async function confirmGroups(excluded: string[]) {
    onBusy(true);
    try {
      for (const group of pending.current.filter((group) => !group.confirmed)) {
        const ids = new Set(
          group.quote!.items.flatMap((item) =>
            item.operation_id ? [item.operation_id] : [],
          ),
        );
        const omitted = excluded.filter((id) => ids.has(id));
        if (
          !group.quote!.items.some(
            (item) => item.operation_id && !omitted.includes(item.operation_id),
          )
        )
          continue;
        group.excluded = omitted;
        try {
          await confirmGeneration(
            projectId,
            group.quote!,
            omitted,
            group.confirmKey,
          );
          group.confirmed = true;
          setConfirmedRows(acceptedCount());
        } catch (failure) {
          group.uncertain =
            !(failure instanceof ApiError) ||
            failure.status === 0 ||
            failure.status >= 500;
          setUncertain(group.uncertain);
          setQuoteOpen(false);
          setError(
            group.uncertain
              ? `已有 ${acceptedCount()} 行确认。当前组确认结果未知，请核验原幂等请求后继续。`
              : `已有 ${acceptedCount()} 行确认，后续组未确认。已确认任务继续执行；请为未确认部分重新预览费用。`,
          );
          throw failure;
        }
      }
      setConfirmed(true);
      setQuoteOpen(false);
    } finally {
      await cache.invalidateQueries({
        queryKey: [...OPERATIONS_KEY, projectId],
      });
      onBusy(pending.current.some((group) => group.uncertain));
    }
  }
  async function reconcile() {
    if (activeRequest.current || quoting) return;
    activeRequest.current = true;
    setQuoting(true);
    onBusy(true);
    try {
      for (const group of pending.current.filter((group) => group.uncertain)) {
        // Replay the user's original confirmation with exactly the same UUID and exclusions.
        await confirmGeneration(
          projectId,
          group.quote!,
          group.excluded!,
          group.confirmKey,
        );
        group.uncertain = false;
        group.confirmed = true;
      }
      setUncertain(false);
      setConfirmedRows(acceptedCount());
      setError(
        "原确认结果已核实。已确认任务继续执行，可为未确认部分预览费用。",
      );
      if (pending.current.every((group) => group.confirmed)) setConfirmed(true);
    } catch (failure) {
      if (
        failure instanceof ApiError &&
        failure.status >= 400 &&
        failure.status < 500
      ) {
        for (const group of pending.current.filter((group) => group.uncertain))
          group.uncertain = false;
        setUncertain(false);
        setError("服务端确认原请求未成功；可重新预览未确认部分的费用。");
      } else
        setError(
          "原请求的确认结果仍未知，请稍后继续核验。请勿另发重复生成请求。",
        );
    } finally {
      activeRequest.current = false;
      setQuoting(false);
      onBusy(pending.current.some((group) => group.uncertain));
      await cache.invalidateQueries({
        queryKey: [...OPERATIONS_KEY, projectId],
      });
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
        <div className="self-end text-xs text-muted-foreground">
          {config.rows.filter((row) => row.enabled).length} 个启用行 · {pages}{" "}
          组报价
        </div>
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
                : "保存参数并预览全表费用"
          }
          onSubmit={(params) => {
            void requestQuote(params);
          }}
        />
      ) : (
        <p className="text-xs text-muted-foreground">
          选择模型和生成方式后设置参数。全表按每组最多 150
          行报价，统一确认费用后按保存的并发数执行。
        </p>
      )}
      {quoteProgress ? (
        <p role="status" className="text-xs">
          {quoteProgress}
        </p>
      ) : null}
      {!quoteOpen && uncertain ? (
        <Button
          variant="outline"
          disabled={quoting}
          onClick={() => {
            void reconcile();
          }}
        >
          按原幂等请求核验确认结果
        </Button>
      ) : null}
      {!quoteOpen && !uncertain && !confirmed && confirmedRows > 0 ? (
        <Button
          variant="outline"
          disabled={quoting || disabled}
          onClick={() => {
            void requote();
          }}
        >
          为未确认部分预览费用
        </Button>
      ) : null}
      {error ? (
        <p role="alert" className="text-sm text-destructive">
          {error}
        </p>
      ) : null}
      {confirmed ? (
        <p role="status" className="text-sm">
          {confirmedRows} 行已确认。
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
          onConfirm={confirmGroups}
        />
      ) : null}
    </div>
  );
}
