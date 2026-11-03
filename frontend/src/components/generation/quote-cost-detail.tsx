import {
  Collapsible,
  CollapsibleTrigger,
  CollapsibleContent,
} from "@/components/ui/collapsible";
import { Button } from "@/components/ui/button";
import { formatBillingMicros } from "@/components/billing/billing-outcome";
import { ChevronDown } from "lucide-react";

const unitLabels: Record<string, string> = {
  per_request: "每次请求",
  per_image: "每张图片",
  per_second: "每秒",
  per_1k_tokens: "每千 Token",
};

/** Renders server-calculated factors. It never calculates or authorizes a quote. */
export function QuoteCostDetail({
  amount,
  detail,
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
        费用待确认
      </p>
    );
  const credits = detail?.estimated_credits;
  const knownCredits =
    typeof credits === "number" &&
    Number.isSafeInteger(credits) &&
    credits >= 0;
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
    <section
      className="flex min-w-0 flex-col gap-2 text-sm"
      aria-label="本次费用"
    >
      <p>
        {knownCredits
          ? `预计消耗 ${credits} 积分`
          : `预计预留 ${formatBillingMicros(amount)}`}
      </p>
      <Collapsible className="min-w-0">
        <CollapsibleTrigger asChild>
          <Button
            type="button"
            variant="ghost"
            size="sm"
            className="group/collapsible"
          >
            费用说明
            <ChevronDown
              data-icon="inline-end"
              className="transition-transform group-data-[state=open]/collapsible:rotate-180"
            />
          </Button>
        </CollapsibleTrigger>
        <CollapsibleContent>
          <div className="flex flex-col gap-3 pt-3">
            {knownCredits && <p>预计预留 {formatBillingMicros(amount)}</p>}
            <p className="text-muted-foreground">
              预留金额不代表最终实扣或费用上限。
              {knownCredits ? "积分按本次报价规则向上取整。" : ""}
            </p>
            <dl className="grid grid-cols-[auto_minmax(0,1fr)] gap-x-3 gap-y-2 break-words">
              {Object.entries({
                价格版本: detail?.price_version_no,
                计费单位:
                  typeof detail?.unit === "string"
                    ? (unitLabels[detail.unit] ?? detail.unit)
                    : undefined,
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
                .filter(
                  ([, v]) => typeof v === "string" || typeof v === "number",
                )
                .map(([name, value]) => (
                  <div className="contents" key={name}>
                    <dt>{name}</dt>
                    <dd>{String(value)}</dd>
                  </div>
                ))}
            </dl>
          </div>
        </CollapsibleContent>
      </Collapsible>
    </section>
  );
}
