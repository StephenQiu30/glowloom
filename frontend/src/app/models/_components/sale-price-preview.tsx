"use client";

import { useEffect, useRef, useState } from "react";
import { previewAdminSalePrice } from "@/api/settings";
import { Button } from "@/components/ui/button";
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible";
import { Label } from "@/components/ui/label";
import { Input } from "@/components/ui/input";
import { catalogMessage } from "@/components/catalog/mutation";
import { priceRule } from "./model-setup";

export function saleDiscountFactor(value: FormDataEntryValue | null) {
  const text = String(value ?? "10");
  if (
    !/^(?:10(?:\.0{1,5})?|[0-9](?:\.\d{1,5})?)$/.test(text) ||
    Number(text) <= 0
  )
    throw new Error("销售折扣须大于 0 且不超过 10 折。");
  return (Number(text) / 10).toFixed(6).replace(/0+$/, "").replace(/\.$/, "");
}

const money = (micros: number) =>
  (micros / 1_000_000).toLocaleString("zh-CN", { maximumFractionDigits: 6 });

export function SalePricePreview({
  unit,
  currency,
}: {
  unit: string;
  currency: string;
}) {
  const [result, setResult] = useState<Required<API.SalePreview> | null>(null);
  const [error, setError] = useState("");
  const root = useRef<HTMLDivElement>(null);
  const pending = useRef<AbortController | null>(null);
  useEffect(() => {
    const form = root.current?.closest("form");
    const reset = () => {
      pending.current?.abort();
      setResult(null);
      setError("");
    };
    form?.addEventListener("input", reset);
    return () => {
      pending.current?.abort();
      form?.removeEventListener("input", reset);
    };
  }, []);
  async function preview(form: HTMLFormElement | null) {
    if (!form) return;
    pending.current?.abort();
    const controller = new AbortController();
    pending.current = controller;
    setError("");
    setResult(null);
    try {
      const f = new FormData(form);
      const sample = (key: string, fallback = 0) =>
        Number(f.get(key) ?? fallback);
      const result = await previewAdminSalePrice(
        {
          unit: unit as API.PriceUnit,
          currency,
          fx_rate_to_cny:
            currency === "CNY" ? undefined : String(f.get("fx_rate") ?? ""),
          rule: priceRule(f),
          markup_multiplier: String(f.get("markup_multiplier") ?? "1"),
          discount_factor: saleDiscountFactor(f.get("discount_tenths")),
          output_count: sample("sample_count", 1),
          duration_ms: sample("sample_duration") * 1000,
          characters: sample("sample_chars"),
          input_tokens: sample("sample_input"),
          output_tokens: sample("sample_output"),
          reference_images: sample("sample_refs"),
          mode: String(f.get("sample_mode") ?? ""),
          resolution: String(f.get("sample_resolution") ?? ""),
        },
        { signal: controller.signal },
      );
      const { procurement_micros, regular_credits, sale_micros, sale_credits } =
        result;
      if (
        procurement_micros == null ||
        regular_credits == null ||
        sale_micros == null ||
        sale_credits == null ||
        ![procurement_micros, regular_credits, sale_micros, sale_credits].every(
          Number.isSafeInteger,
        )
      )
        throw new Error("试算结果不完整，请核对价目。");
      if (!controller.signal.aborted)
        setResult({
          procurement_micros,
          regular_credits,
          sale_micros,
          sale_credits,
        });
    } catch (e) {
      if (!controller.signal.aborted)
        setError(e instanceof Error ? e.message : catalogMessage(e));
    }
  }
  const fields: [string, string, number][] = [["sample_refs", "参考图数", 0]];
  if (unit === "per_image" || unit === "per_second")
    fields.unshift(["sample_count", "输出数量", 1]);
  if (unit === "per_second") fields.push(["sample_duration", "计费秒数", 5]);
  if (unit === "per_1k_chars") fields.push(["sample_chars", "计费字符", 1000]);
  if (unit === "per_1k_tokens")
    fields.push(
      ["sample_input", "输入 Token", 0],
      ["sample_output", "输出 Token", 1000],
    );
  return (
    <Collapsible ref={root}>
      <CollapsibleTrigger asChild>
        <Button type="button" variant="outline" size="sm">
          价格试算
        </Button>
      </CollapsibleTrigger>
      <CollapsibleContent>
        <div className="mt-3 grid gap-3 sm:grid-cols-2">
          {fields.map(([name, label, value]) => (
            <Label key={name} className="text-sm">
              {label}
              <Input
                name={name}
                type="number"
                min="0"
                step="any"
                defaultValue={value}
              />
            </Label>
          ))}
          <Label className="text-sm">
            计费模式
            <Input name="sample_mode" placeholder="按价目填写" />
          </Label>
          <Label className="text-sm">
            分辨率
            <Input name="sample_resolution" placeholder="如 720p" />
          </Label>
        </div>
        <Button
          className="mt-3"
          type="button"
          size="sm"
          variant="outline"
          onClick={(e) => void preview(e.currentTarget.form)}
        >
          试算示例用量
        </Button>
        {error && <p role="alert">{error}</p>}
        {result && (
          <p role="status" className="mt-2 text-sm">
            预计采购 ¥{money(result.procurement_micros)} · 折前{" "}
            {result.regular_credits} 积分 · 折后 {result.sale_credits} 积分 ·
            预计价差 ¥{money(result.sale_micros - result.procurement_micros)}
          </p>
        )}
      </CollapsibleContent>
    </Collapsible>
  );
}
