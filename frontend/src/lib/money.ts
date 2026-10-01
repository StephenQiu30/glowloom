const currency = new Intl.NumberFormat("zh-CN", {
  style: "currency",
  currency: "CNY",
  minimumFractionDigits: 2,
  maximumFractionDigits: 2,
});

export function formatCurrencyMicros(micros: number) {
  if (!Number.isSafeInteger(micros)) return "金额异常";
  const amount = currency.format(Math.abs(micros) / 1_000_000);
  return micros < 0 ? `-${amount}` : amount;
}
