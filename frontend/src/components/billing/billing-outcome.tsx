import { Button } from "@/components/ui/button";

export function formatBillingMicros(amount?: number | null) {
  return typeof amount === "number" &&
    Number.isSafeInteger(amount) &&
    amount >= 0
    ? `¥${(amount / 1_000_000).toLocaleString("zh-CN", { maximumFractionDigits: 6 })}`
    : "待核验";
}
const chargeLabels: Record<string, string> = {
  unknown: "费用状态待核验",
  pending: "费用仍在核对",
  settled: "已记录实扣",
  released: "预留已释放",
  refund_confirmed: "退款已入账",
};
const deliveryLabels: Record<string, string> = {
  pending: "交付状态待核对",
  ready: "结果已就绪",
  recoverable: "可继续取回原结果",
  unrecoverable: "原结果确认无法取回",
};
export function BillingOutcomeView({
  outcome,
  onRecover,
  pending = false,
}: {
  outcome?: API.BillingOutcome;
  onRecover?: () => void;
  pending?: boolean;
}) {
  return (
    <section
      aria-label="费用与交付"
      className="min-w-0 space-y-2 rounded border p-3 text-sm"
    >
      <p>
        {chargeLabels[outcome?.charge_state ?? "unknown"] ??
          chargeLabels.unknown}{" "}
        ·{" "}
        {deliveryLabels[outcome?.delivery_state ?? "pending"] ??
          deliveryLabels.pending}
      </p>
      <dl className="flex flex-wrap gap-x-4 gap-y-1">
        {outcome?.reserved_micros != null && (
          <div>当前预留：{formatBillingMicros(outcome.reserved_micros)}</div>
        )}
        {outcome?.charged_micros != null && (
          <div>实扣：{formatBillingMicros(outcome.charged_micros)}</div>
        )}
        {outcome?.released_micros != null && (
          <div>预留释放：{formatBillingMicros(outcome.released_micros)}</div>
        )}
        {outcome?.refunded_micros != null && (
          <div>已入账退款：{formatBillingMicros(outcome.refunded_micros)}</div>
        )}
      </dl>
      {outcome?.reconciliation_required && (
        <p>费用仍需核查。请保留原任务，不要重复消费。</p>
      )}
      {outcome?.provider_settlement_state &&
        outcome.provider_settlement_state !== "unknown" && (
          <p>
            上游结算观察：
            {(
              {
                pending: "待确认",
                settled: "上游已结算",
                released: "上游未收费",
                refunded: "上游报告退款，平台入账仍需核查",
              } as Record<string, string>
            )[outcome.provider_settlement_state] ?? "待核验"}
          </p>
        )}
      {outcome?.delivery_state === "recoverable" && onRecover && (
        <Button variant="outline" disabled={pending} onClick={onRecover}>
          {pending ? "正在请求核查…" : "继续取回原任务"}
        </Button>
      )}
    </section>
  );
}

// Error explanations do not establish execution or refund evidence.
export function billingFailureMessage(code?: string) {
  const explanations: Record<string, string> = {
    provider_insufficient_balance:
      "上游账号额度不足，请联系模型连接管理员核查。",
    provider_payment_required: "上游要求核对付款状态，具体原因仍需核查。",
    provider_project_limit_exceeded: "上游项目已达到用量限制。",
    provider_organization_limit_exceeded: "上游组织已达到用量限制。",
    provider_insufficient_quota: "上游用量额度受限，请联系连接管理员。",
    provider_rate_limited: "上游请求速率受限，请继续查询原任务。",
    provider_request_limited: "上游暂时限制请求，具体限额原因待核查。",
    provider_auth_failed: "上游凭据无法使用，请联系连接管理员核查原连接。",
    provider_permission_denied: "原连接没有此模型的访问权限。",
  };
  return code && explanations[code]
    ? explanations[code]
    : "生成未能完成。请先核对原任务的费用与交付状态。";
}
