import {
  Collapsible,
  CollapsibleTrigger,
  CollapsibleContent,
} from "@/components/ui/collapsible";
import { Button } from "@/components/ui/button";
import { formatBillingMicros } from "@/components/billing/billing-outcome";

/** Renders server-calculated factors. It never calculates or authorizes a quote. */
export function QuoteCostDetail({
  amount,
  detail,
  preview,
}: {
  amount?: number | null;
  detail?: Record<string, unknown> | null;
  preview?: {
    amount_kind?: string;
    confirmable?: boolean;
    unit?: string;
    currency?: string;
  };
}) {
  const known =
    typeof amount === "number" && Number.isSafeInteger(amount) && amount >= 0;
  if (!known)
    return (
      <p role="status" className="text-sm text-muted-foreground">
        {preview?.amount_kind === "unit_only"
          ? "仅有单价，需完整输入与有效规则后才能报价。"
          : "费用待核验，当前没有可确认的报价。"}
      </p>
    );
  const credits = detail?.estimated_credits;
  const factors = detail?.multipliers;
  const namedFactors: Record<string, string | number> = {};
  if (factors && typeof factors === "object" && !Array.isArray(factors)) {
    for (const [key, value] of Object.entries(factors)) {
      if (
        typeof value !== "number" &&
        (typeof value !== "string" || !value.trim())
      )
        continue;
      if (key.startsWith("mode_") && key.length > 5)
        namedFactors[`模式系数（${key.slice(5)}）`] = value;
      if (key.startsWith("resolution_") && key.length > 11)
        namedFactors[`分辨率系数（${key.slice(11)}）`] = value;
    }
  }
  return (
    <section className="min-w-0 space-y-2 text-sm" aria-label="本次费用">
      <p>预留费用：{formatBillingMicros(amount)}</p>
      {typeof credits === "number" &&
        Number.isSafeInteger(credits) &&
        credits >= 0 && <p>预估积分：{credits}（按本次冻结规则向上取整）</p>}
      <p className="text-muted-foreground">
        预留金额不代表最终实扣或费用上限。
      </p>
      <Collapsible className="min-w-0 rounded border p-3">
        <CollapsibleTrigger asChild>
          <Button type="button" variant="ghost">
            费用明细
          </Button>
        </CollapsibleTrigger>
        <CollapsibleContent>
          <dl className="mt-3 grid grid-cols-[auto_minmax(0,1fr)] gap-x-3 gap-y-2 break-words">
            {Object.entries({
              价格版本: detail?.price_version_no,
              计费单位: detail?.unit,
              "单价（原币微单位）": detail?.unit_price_micros,
              数量: detail?.quantity,
              输出数: detail?.outputs,
              币种: detail?.currency,
              冻结汇率: detail?.fx_rate_to_cny,
              加价倍率: detail?.markup_multiplier,
              ...namedFactors,
              参考图数: detail?.reference_image_count,
              免费参考图数: detail?.free_reference_images,
              "参考图单价（原币微单位）": detail?.reference_image_micros,
              参考图计费范围: detail?.reference_scope,
              输入Token预估: detail?.estimated_input_tokens,
              最大输出Token: detail?.max_output_tokens,
              输入千Token单价: detail?.input_unit_price_micros,
              输出千Token单价: detail?.output_unit_price_micros,
            })
              .filter(([, v]) => typeof v === "string" || typeof v === "number")
              .map(([name, value]) => (
                <div className="contents" key={name}>
                  <dt>{name}</dt>
                  <dd>{String(value)}</dd>
                </div>
              ))}
          </dl>
        </CollapsibleContent>
      </Collapsible>
    </section>
  );
}
