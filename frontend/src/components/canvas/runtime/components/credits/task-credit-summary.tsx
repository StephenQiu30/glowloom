import { creditFormat } from "./lib/credit-format";

/** A failed generation does not imply a free or already-settled invocation. */
export function TaskCreditSummary({
  billing,
}: {
  billing?: CanvasContract.GenerationBillingVO;
}) {
  if (!billing) return null;
  let text = "正在准备积分";
  if (billing.state === "SETTLED")
    text = `实扣 ${creditFormat(billing.chargedPoints)} 积分`;
  else if (billing.state === "RELEASED")
    text = `未扣费 · 已释放 ${creditFormat(billing.reservedPoints)} 积分`;
  else if (billing.reservedPoints != null)
    text = `${billing.state === "RECONCILING" ? "费用待核对" : "等待结算"} · 已占用 ${creditFormat(billing.reservedPoints)} 积分`;
  return (
    <p className="text-sm text-muted-foreground tabular-nums" role="status">
      {text}
    </p>
  );
}
