"use client";

import { useEffect, useRef, useState } from "react";
import { Dialog } from "radix-ui";

import { Button } from "@/components/ui/button";

import {
  formatQuote,
  useQuote,
  type QuoteItem,
  type QuoteResponse,
} from "./use-quote";

export type { QuoteResponse } from "./use-quote";

type QuoteConfirmDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  quote: QuoteResponse;
  onConfirm: (excludeOperationIds: string[]) => void | Promise<void>;
  onRequote: () => void;
  onForceRegenerate?: (operationId: string) => void;
  previewOnly?: boolean;
};

const rejectionMessages: Record<string, string> = {
  quote_expired: "报价已失效，请重新报价。",
  budget_insufficient: "项目预算已变化，请重新报价。",
  model_unavailable: "模型当前不可用，请重新报价。",
  forbidden: "当前账号不能确认此报价。",
};

const unitPriceNumber = new Intl.NumberFormat("zh-CN", {
  minimumFractionDigits: 2,
  maximumFractionDigits: 6,
});

function formatUnitPrice(micros: number, currency: string) {
  if (
    !Number.isSafeInteger(micros) ||
    micros < 0 ||
    !/^[A-Z]{3}$/.test(currency)
  )
    return "价格异常";
  const amount = unitPriceNumber.format(micros / 1_000_000);
  return currency === "CNY" ? `¥${amount}` : `${currency} ${amount}`;
}

function failureMessage(error: unknown) {
  const code =
    typeof error === "object" && error !== null && "code" in error
      ? error.code
      : undefined;
  return typeof code === "string" && rejectionMessages[code]
    ? rejectionMessages[code]
    : "确认未完成，请刷新报价后重试。";
}

function detailText(item: QuoteItem) {
  const detail = item.quote_detail;
  if (!detail) return "服务端未提供费用拆分。";
  const currency = detail.currency ?? "CNY";
  let breakdown: string;
  if (detail.unit === "per_1k_tokens") {
    const tokenParts: string[] = [];
    if (
      detail.estimated_input_tokens !== undefined &&
      detail.input_unit_price_micros !== undefined
    ) {
      tokenParts.push(
        `输入 ${detail.estimated_input_tokens} token × ${formatUnitPrice(detail.input_unit_price_micros, currency)}/千 token`,
      );
    }
    if (
      detail.max_output_tokens !== undefined &&
      detail.output_unit_price_micros !== undefined
    ) {
      tokenParts.push(
        `输出上限 ${detail.max_output_tokens} token × ${formatUnitPrice(detail.output_unit_price_micros, currency)}/千 token`,
      );
    }
    breakdown = tokenParts.join("；");
  } else {
    const parts: string[] = [];
    if (detail.quantity !== undefined) parts.push(String(detail.quantity));
    if (detail.unit_price_micros !== undefined)
      parts.push(formatUnitPrice(detail.unit_price_micros, currency));
    if (
      (detail.unit === "per_image" || detail.unit === "per_second") &&
      detail.outputs !== undefined
    )
      parts.push(String(detail.outputs));
    breakdown = parts.join(" × ");
  }
  const unit = detail.unit ? `计费单位：${detail.unit}` : "";
  const outputs =
    detail.unit === "per_request" && detail.outputs !== undefined
      ? `输出数：${detail.outputs}（按次计费）`
      : "";
  const multipliers = detail.multipliers
    ? Object.entries(detail.multipliers)
        .map(([name, value]) => `${name} × ${value}`)
        .join("，")
    : "";
  const exchange =
    currency !== "CNY" && detail.fx_rate_to_cny
      ? `汇率 × ${detail.fx_rate_to_cny}（折合人民币 ${formatQuote(item.quote_micros ?? Number.NaN)}）`
      : "";
  return (
    [breakdown, unit, outputs, multipliers, exchange]
      .filter(Boolean)
      .join("；") || "服务端未提供费用拆分。"
  );
}

function QuoteDialogContent({
  open,
  onOpenChange,
  quote,
  onConfirm,
  onRequote,
  onForceRegenerate,
  previewOnly = false,
}: QuoteConfirmDialogProps) {
  const [excludedIndexes, setExcludedIndexes] = useState<ReadonlySet<number>>(
    () => new Set(),
  );
  const [now, setNow] = useState(() => Date.now());
  const [pending, setPending] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const submitting = useRef(false);
  const cancelButton = useRef<HTMLButtonElement>(null);
  const summary = useQuote(quote, excludedIndexes);
  const expiresAt = Date.parse(quote.expires_at);
  const secondsLeft = Number.isFinite(expiresAt)
    ? Math.max(0, Math.ceil((expiresAt - now) / 1000))
    : 0;
  const expired = secondsLeft === 0;
  const canConfirm =
    !previewOnly &&
    summary.count > 0 &&
    !summary.invalid &&
    !summary.malformedAmount &&
    summary.shortfallMicros === 0 &&
    !expired &&
    !pending &&
    !error &&
    (quote.confirmable || excludedIndexes.size > 0);

  useEffect(() => {
    if (!open) return;
    const timer = window.setInterval(() => setNow(Date.now()), 1_000);
    return () => window.clearInterval(timer);
  }, [open]);

  function toggleExcluded(index: number) {
    setExcludedIndexes((current) => {
      const next = new Set(current);
      if (next.has(index)) next.delete(index);
      else next.add(index);
      return next;
    });
  }

  async function confirm() {
    if (!canConfirm || submitting.current) return;
    submitting.current = true;
    setPending(true);
    setError(null);
    try {
      await onConfirm(summary.excludedOperationIds);
    } catch (failure) {
      setError(failureMessage(failure));
    } finally {
      submitting.current = false;
      setPending(false);
    }
  }

  return (
    <Dialog.Root
      open={open}
      onOpenChange={(next) => {
        if (!submitting.current) onOpenChange(next);
      }}
    >
      <Dialog.Portal>
        <Dialog.Overlay className="fixed inset-0 z-50 bg-black/55" />
        <Dialog.Content
          className="fixed top-1/2 left-1/2 z-50 flex max-h-[min(88dvh,48rem)] w-[calc(100vw-2rem)] max-w-2xl -translate-x-1/2 -translate-y-1/2 flex-col overflow-hidden rounded-2xl border bg-background text-foreground shadow-2xl outline-none"
          onOpenAutoFocus={(event) => {
            event.preventDefault();
            cancelButton.current?.focus();
          }}
          onEscapeKeyDown={(event) => {
            if (submitting.current) event.preventDefault();
          }}
          onPointerDownOutside={(event) => {
            if (submitting.current) event.preventDefault();
          }}
        >
          <div className="border-b border-border px-5 py-5 sm:px-6">
            <Dialog.Title className="text-lg font-semibold tracking-tight">
              确认生成报价
            </Dialog.Title>
            <Dialog.Description className="mt-1 text-sm text-muted-foreground">
              {previewOnly
                ? "前端 PoC · 样例报价，仅预览费用和决策流程，尚未接入生成服务。"
                : "请核对费用和服务区域。只有点击下方生成按钮才会提交确认。"}
            </Dialog.Description>
          </div>

          <div className="min-h-0 flex-1 space-y-5 overflow-y-auto px-5 py-5 sm:px-6">
            <div className="flex flex-wrap items-center justify-between gap-2 rounded-xl bg-muted/60 px-4 py-3 text-sm">
              <span className="text-muted-foreground">报价有效期</span>
              <span className="font-medium tabular-nums" aria-live="off">
                {expired
                  ? "已过期"
                  : `${Math.floor(secondsLeft / 60)}:${String(secondsLeft % 60).padStart(2, "0")}`}
              </span>
            </div>

            {summary.hasOverseas && (
              <p className="rounded-xl border border-amber-500/40 bg-amber-500/10 px-4 py-3 text-sm text-amber-900 dark:text-amber-200">
                数据出境提示：所选项目将由境外区域的模型服务处理，请确认素材和授权范围。
              </p>
            )}

            <ul className="space-y-3" aria-label="报价项目">
              {quote.items.map((item, index) => {
                const excluded = excludedIndexes.has(index);
                const invalid = !item.operation_id || item.errors.length > 0;
                return (
                  <li
                    key={item.operation_id ?? `${item.target_label}-${index}`}
                    className={`rounded-xl border p-4 ${excluded ? "opacity-60" : ""}`}
                  >
                    <div className="flex flex-wrap items-start justify-between gap-3">
                      <div className="min-w-0 flex-1 space-y-1">
                        <p className="font-medium">{item.target_label}</p>
                        {item.model_key && (
                          <p className="text-sm break-all text-muted-foreground">
                            {item.model_key}
                          </p>
                        )}
                        {item.mode && (
                          <p className="text-xs text-muted-foreground">
                            模式：{item.mode}
                          </p>
                        )}
                        {item.region && (
                          <p className="text-xs text-muted-foreground">
                            服务区域：
                            {item.region === "overseas"
                              ? "境外"
                              : item.region === "domestic"
                                ? "国内"
                                : item.region}
                          </p>
                        )}
                      </div>
                      <p className="shrink-0 font-semibold tabular-nums">
                        {item.quote_micros === undefined
                          ? "无法报价"
                          : formatQuote(item.quote_micros)}
                      </p>
                    </div>
                    {item.reused && (
                      <p className="mt-3 text-sm text-muted-foreground">
                        复用已有结果，¥0
                      </p>
                    )}
                    {item.errors.length > 0 && (
                      <ul className="mt-3 space-y-1 text-sm text-destructive">
                        {item.errors.map((issue, issueIndex) => (
                          <li key={`${issue.code}-${issueIndex}`}>
                            {issue.message}
                          </li>
                        ))}
                      </ul>
                    )}
                    {item.quote_detail && (
                      <details className="mt-3 text-sm">
                        <summary className="cursor-pointer text-muted-foreground hover:text-foreground">
                          费用明细
                        </summary>
                        <p className="mt-2 break-words text-muted-foreground">
                          {detailText(item)}
                        </p>
                      </details>
                    )}
                    <div className="mt-3 flex flex-wrap gap-2">
                      <Button
                        type="button"
                        size="sm"
                        variant="outline"
                        disabled={pending}
                        onClick={() => toggleExcluded(index)}
                        aria-label={`${excluded ? "恢复" : "剔除"} ${item.target_label}`}
                      >
                        {excluded ? "恢复" : "剔除"}
                      </Button>
                      {item.reused &&
                        item.operation_id &&
                        onForceRegenerate && (
                          <Button
                            type="button"
                            size="sm"
                            variant="ghost"
                            disabled={pending || previewOnly}
                            onClick={() =>
                              onForceRegenerate(item.operation_id!)
                            }
                            aria-label={`强制重新生成 ${item.target_label}`}
                          >
                            强制重新生成
                          </Button>
                        )}
                      {invalid && !excluded && (
                        <span className="self-center text-xs text-destructive">
                          此项需剔除或重新报价
                        </span>
                      )}
                    </div>
                  </li>
                );
              })}
            </ul>
          </div>

          <div className="space-y-3 border-t border-border px-5 py-4 sm:px-6">
            <dl className="space-y-1 text-sm">
              <div className="flex justify-between gap-4">
                <dt className="text-muted-foreground">合计</dt>
                <dd className="font-semibold tabular-nums">
                  {formatQuote(summary.totalMicros)}
                </dd>
              </div>
              <div className="flex justify-between gap-4">
                <dt className="text-muted-foreground">剩余预算</dt>
                <dd className="tabular-nums">
                  {formatQuote(quote.available_micros)}
                </dd>
              </div>
            </dl>
            {summary.shortfallMicros > 0 && (
              <p className="text-sm text-destructive" role="status">
                预算不足，还差 {formatQuote(summary.shortfallMicros)}
                。可剔除项目后确认。
              </p>
            )}
            {summary.malformedAmount && (
              <p className="text-sm text-destructive" role="status">
                报价金额不一致，请重新报价。
              </p>
            )}
            {expired && (
              <p className="text-sm text-destructive" role="status">
                报价已过期，请重新报价。
              </p>
            )}
            {error && (
              <p className="text-sm text-destructive" role="alert">
                {error}
              </p>
            )}
            {!quote.confirmable &&
              excludedIndexes.size === 0 &&
              summary.shortfallMicros === 0 &&
              !summary.invalid &&
              !summary.malformedAmount &&
              !expired && (
                <p className="text-sm text-destructive" role="status">
                  当前报价不可确认，请重新报价。
                </p>
              )}
            <div className="flex flex-wrap justify-end gap-2">
              <Button
                ref={cancelButton}
                type="button"
                variant="outline"
                disabled={pending}
                onClick={() => onOpenChange(false)}
              >
                取消
              </Button>
              <Button
                type="button"
                variant="secondary"
                disabled={pending}
                onClick={onRequote}
              >
                重新报价
              </Button>
              <Button
                type="button"
                disabled={!canConfirm}
                onKeyDown={(event) => {
                  if (event.key === "Enter") event.preventDefault();
                }}
                onClick={confirm}
              >
                {previewOnly
                  ? "生成服务待接入"
                  : pending
                    ? "确认中…"
                    : summary.label}
              </Button>
            </div>
          </div>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  );
}

export function QuoteConfirmDialog(props: QuoteConfirmDialogProps) {
  return (
    <QuoteDialogContent
      key={`${props.quote.batch_id ?? "single"}-${props.quote.expires_at}`}
      {...props}
    />
  );
}
