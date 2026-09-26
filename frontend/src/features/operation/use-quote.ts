"use client";

import { useMemo } from "react";

export type QuoteIssue = { code: string; message: string };

export type QuoteItem = {
  operation_id: string | null;
  target_label: string;
  model_key?: string;
  mode?: string;
  quote_micros?: number;
  reused?: boolean;
  region?: string;
  errors: QuoteIssue[];
  quote_detail?: {
    unit?: string;
    quantity?: number;
    unit_price_micros?: number;
    outputs?: number;
    multipliers?: Record<string, number>;
  };
};

export type QuoteResponse = {
  batch_id: string | null;
  expires_at: string;
  items: QuoteItem[];
  total_micros: number;
  available_micros: number;
  confirmable: boolean;
};

const currency = new Intl.NumberFormat("zh-CN", {
  style: "currency",
  currency: "CNY",
  minimumFractionDigits: 2,
  maximumFractionDigits: 2,
});

export function formatQuote(micros: number) {
  if (!Number.isSafeInteger(micros) || micros < 0) return "报价异常";
  return currency.format(micros / 1_000_000);
}

function validMicros(value: number | undefined): value is number {
  return value !== undefined && Number.isSafeInteger(value) && value >= 0;
}

export function useQuote(
  quote: QuoteResponse,
  excludedIndexes: ReadonlySet<number>,
) {
  return useMemo(() => {
    const selected = quote.items.filter(
      (_, index) => !excludedIndexes.has(index),
    );
    const itemMicros = (item: QuoteItem) =>
      validMicros(item.quote_micros) ? item.quote_micros : 0;
    const totalMicros = selected.reduce(
      (total, item) => total + itemMicros(item),
      0,
    );
    const initialTotal = quote.items.reduce(
      (total, item) => total + itemMicros(item),
      0,
    );
    const malformedAmount =
      !validMicros(quote.total_micros) ||
      !validMicros(quote.available_micros) ||
      !Number.isSafeInteger(initialTotal) ||
      initialTotal !== quote.total_micros ||
      quote.items.some(
        (item) => item.operation_id && !validMicros(item.quote_micros),
      );
    const invalid = selected.some(
      (item) => !item.operation_id || item.errors.length > 0,
    );
    const count = selected.length;
    const label =
      count > 1
        ? `批量生成 ${count} 项（约 ${formatQuote(totalMicros)}）`
        : `生成（约 ${formatQuote(totalMicros)}）`;
    const excludedOperationIds = quote.items.flatMap((item, index) =>
      excludedIndexes.has(index) && item.operation_id
        ? [item.operation_id]
        : [],
    );

    return {
      count,
      totalMicros,
      invalid,
      malformedAmount,
      shortfallMicros: Math.max(0, totalMicros - quote.available_micros),
      label,
      excludedOperationIds,
      hasOverseas: selected.some((item) => item.region === "overseas"),
    };
  }, [quote, excludedIndexes]);
}
