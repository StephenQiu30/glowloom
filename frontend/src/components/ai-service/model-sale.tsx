"use client";

import { useState } from "react";
import { Button } from "@/components/ui/button";
import {
  FieldDescription,
  FieldGroup,
  FieldLegend,
  FieldSet,
} from "@/components/ui/field";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import {
  previewModelSale,
  publishModelSale,
  publishRoutePurchase,
  type GatewayRoute,
  type GatewayModel,
  type GatewayPrice,
  type SaleUsage,
  type TextRates,
} from "@/lib/ai-service-admin";
import {
  FormDetails,
  InputField,
  micros,
  num,
  priceSummary,
  text,
  type RunMutation,
} from "./admin-fields";

const tokenRates = [
  ["input", "输入"],
  ["output", "输出"],
  ["cache_read", "缓存读取"],
  ["cache_write", "缓存写入"],
  ["reasoning", "推理"],
] as const;

const mediaRates = [
  ["unit_price", "基础单价"],
  ["output_token_price", "输出 Token 单价"],
  ["high_price", "高分辨率单价"],
  ["extra_reference_price", "额外参考图单价"],
] as const;

function rateFields(rates: TextRates, prefix: string) {
  return tokenRates.flatMap(([key, label]) => {
    const value = rates[key];
    return value === undefined
      ? []
      : [
          <InputField
            key={key}
            label={label}
            name={prefix + key}
            value={value}
            inputMode="decimal"
          />,
        ];
  });
}

function readRates(
  form: FormData,
  rates: TextRates,
  prefix: string,
): TextRates {
  return {
    input: text(form, prefix + "input"),
    output: text(form, prefix + "output"),
    cache_read: text(form, prefix + "cache_read"),
    cache_write: text(form, prefix + "cache_write"),
    ...(rates.reasoning === undefined
      ? {}
      : { reasoning: text(form, prefix + "reasoning") }),
  };
}

function readSale(form: FormData, current: GatewayPrice): GatewayPrice {
  const next = {
    ...current,
    multiplier: text(form, "multiplier"),
    discount: text(form, "discount"),
  };
  if (current.text_tariff) {
    const tariff = current.text_tariff;
    next.text_tariff = {
      ...tariff,
      tiers: tariff.tiers.map((tier, i) => ({
        from_input: tier.from_input,
        ...readRates(form, tier, `tier.${i}.`),
      })),
      ...(tariff.off_peak
        ? { off_peak: readRates(form, tariff.off_peak, "off.") }
        : {}),
    };
  } else if (current.media_tariff) {
    next.media_tariff = { ...current.media_tariff };
    for (const [key] of mediaRates) {
      if (current.media_tariff[key] !== undefined)
        next.media_tariff[key] = text(form, key);
    }
  } else {
    for (const key of [
      "input",
      "output",
      "cache_read",
      "cache_write",
      "request",
    ] as const) {
      if (form.has(key)) next[key] = micros(form, key);
    }
  }
  return next;
}

function usageFields(model: GatewayModel): {
  key: keyof SaleUsage;
  label: string;
  sample: string;
  decimal?: boolean;
}[] {
  const tokens = [
    { key: "input", label: "输入 Token", sample: "1000" },
    { key: "output", label: "输出 Token", sample: "1000" },
  ] as const;
  switch (model.price.media_tariff?.meter) {
    case "speech_characters_v1":
      return [{ key: "characters", label: "字符数", sample: "100" }];
    case "original_audio_seconds_v1":
      return [
        {
          key: "original_seconds",
          label: "音频秒数",
          sample: "5",
          decimal: true,
        },
      ];
    case "output_video_seconds_v1":
      return [
        { key: "video_duration_ms", label: "视频毫秒数", sample: "5000" },
      ];
    case "ark_image_pixels_v1":
      return [
        { key: "pixels", label: "像素总数", sample: "1048576" },
        { key: "reference_images", label: "参考图数量", sample: "0" },
      ];
    case "reported_image_amount_v1":
      return [
        {
          key: "provider_amount",
          label: "供应商用量金额（USD）",
          sample: "0.05",
          decimal: true,
        },
      ];
    case "image_tokens_v1":
      return [...tokens];
  }
  if (model.video_metering)
    return [{ key: "output", label: "视频 Token", sample: "108900" }];
  if (model.protocol === "media") return [];
  return model.protocol === "ark_embedding" ? [tokens[0]] : [...tokens];
}

export function ModelPrice({
  model,
  route,
  busy,
  run,
}: {
  model: GatewayModel;
  route?: GatewayRoute;
  busy: boolean;
  run: RunMutation;
}) {
  const [pending, setPending] = useState<GatewayPrice>();
  const [credits, setCredits] = useState<number>();
  const price = route?.purchase ?? model.price;
  const label = route ? "采购价" : "售价";
  const usage = usageFields(model);
  const flatFields =
    model.protocol === "media"
      ? model.video_metering
        ? ([["output", "每百万视频 Token"]] as const)
        : ([["request", "每次调用"]] as const)
      : model.protocol === "ark_embedding"
        ? ([["input", "每百万输入 Token"]] as const)
        : ([
            ["input", "每百万输入 Token"],
            ["output", "每百万输出 Token"],
            ["cache_read", "每百万缓存读取 Token"],
            ["cache_write", "每百万缓存写入 Token"],
            ["request", "每次附加费"],
          ] as const);

  return (
    <section
      aria-labelledby="model-sale-heading"
      className="flex flex-col gap-4"
    >
      <div className="flex flex-col gap-1">
        <h2 id="model-sale-heading" className="font-medium">
          {route ? "采购价" : "销售价"}
        </h2>
        <p className="text-sm text-muted-foreground">
          现行 {priceSummary(price, model.protocol)}
        </p>
      </div>
      <form
        className="flex flex-col gap-4"
        onChange={() => setCredits(undefined)}
        onSubmit={(event) => {
          event.preventDefault();
          const action = (event.nativeEvent as SubmitEvent).submitter;
          const form = new FormData(event.currentTarget);
          if (
            action instanceof HTMLButtonElement &&
            action.value === "preview"
          ) {
            void run(
              async () => {
                const sample: SaleUsage = Object.fromEntries(
                  usage.map(({ key, decimal }) => [
                    key,
                    decimal
                      ? text(form, `usage.${key}`)
                      : num(form, `usage.${key}`),
                  ]),
                );
                const result = await previewModelSale(
                  model,
                  readSale(form, price),
                  sample,
                );
                setCredits(result.credits);
              },
              "",
              false,
            );
          } else {
            void run(
              async () => {
                setPending(readSale(form, price));
              },
              "",
              false,
            );
          }
        }}
      >
        <FieldSet disabled={busy} className="flex min-w-0 flex-col gap-4">
          <FieldGroup className="grid gap-3 sm:grid-cols-2">
            <InputField
              label="倍率"
              name="multiplier"
              value={price.multiplier}
              inputMode="decimal"
            />
            <InputField
              label="折扣"
              name="discount"
              value={price.discount}
              inputMode="decimal"
            />
          </FieldGroup>
          <FormDetails label={`基础费率 · ${price.currency}`}>
            {price.text_tariff ? (
              <FieldGroup>
                {price.text_tariff.tiers.map((tier, i) => (
                  <FieldSet key={tier.from_input}>
                    <FieldLegend>
                      {i === 0 ? "标准输入" : `≥ ${tier.from_input} 输入 Token`}
                    </FieldLegend>
                    <FieldGroup className="grid gap-3 sm:grid-cols-2">
                      {rateFields(tier, `tier.${i}.`)}
                    </FieldGroup>
                  </FieldSet>
                ))}
                {price.text_tariff.off_peak && (
                  <FieldSet>
                    <FieldLegend>空闲时段</FieldLegend>
                    <FieldGroup className="grid gap-3 sm:grid-cols-2">
                      {rateFields(price.text_tariff.off_peak, "off.")}
                    </FieldGroup>
                  </FieldSet>
                )}
              </FieldGroup>
            ) : (
              <FieldGroup className="grid gap-3 sm:grid-cols-2">
                {price.media_tariff
                  ? mediaRates.map(
                      ([key, label]) =>
                        price.media_tariff?.[key] !== undefined && (
                          <InputField
                            key={key}
                            label={label}
                            name={key}
                            value={price.media_tariff[key]}
                            inputMode="decimal"
                          />
                        ),
                    )
                  : flatFields.map(([key, label]) => (
                      <InputField
                        key={key}
                        label={label}
                        name={key}
                        value={String(price[key] / 1_000_000)}
                        inputMode="decimal"
                      />
                    ))}
              </FieldGroup>
            )}
          </FormDetails>
          {!route && (
            <FieldSet>
              <FieldLegend>单次积分预览</FieldLegend>
              {usage.length > 0 && (
                <FieldGroup className="grid gap-3 sm:grid-cols-2">
                  {usage.map(({ key, label, sample, decimal }) => (
                    <InputField
                      key={key}
                      label={label}
                      name={`usage.${key}`}
                      value={sample}
                      inputMode={decimal ? "decimal" : "numeric"}
                      pattern={decimal ? "[0-9]+([.][0-9]+)?" : "[0-9]+"}
                    />
                  ))}
                </FieldGroup>
              )}
              {credits !== undefined && (
                <FieldDescription role="status">
                  预计 {credits} 积分
                </FieldDescription>
              )}
            </FieldSet>
          )}
          <div className="flex flex-wrap gap-2">
            {!route && (
              <Button
                type="submit"
                value="preview"
                variant="outline"
                disabled={busy}
              >
                预览积分
              </Button>
            )}
            <Button type="submit" disabled={busy}>
              发布{label}
            </Button>
          </div>
        </FieldSet>
      </form>
      <AlertDialog
        open={pending !== undefined}
        onOpenChange={(open) => {
          if (!open) setPending(undefined);
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>发布新{label}？</AlertDialogTitle>
            <AlertDialogDescription>
              {route
                ? "新的供应商调用采用新采购价，已记录的采购成本保持原口径。"
                : "新报价立即使用新售价，已确认的任务仍按原价结算。"}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>取消</AlertDialogCancel>
            <AlertDialogAction
              disabled={busy}
              onClick={(event) => {
                event.preventDefault();
                if (!pending) return;
                void run(
                  () =>
                    route
                      ? publishRoutePurchase(route, pending)
                      : publishModelSale(model, pending),
                  `${label}已发布。`,
                ).then((saved) => {
                  if (saved) setPending(undefined);
                });
              }}
            >
              确认发布
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </section>
  );
}
