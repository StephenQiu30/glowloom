/** Quotes are server facts. The client validates completeness and never calculates fees. */
export function quoteExpiresAt(value: string | undefined): number {
  if (!value) return Number.NaN;
  return Date.parse(/(?:Z|[+-]\d\d:\d\d)$/.test(value) ? value : `${value}Z`);
}

export function validateModelQuote(
  quote: CanvasContract.CreditQuoteVO,
  now = Date.now(),
) {
  if (
    !quote.id ||
    !quote.priceVersion ||
    !quote.disclosure?.trim() ||
    !/^(0|[1-9]\d*)$/.test(quote.estimatedPoints ?? "") ||
    !/^(0|[1-9]\d*)$/.test(quote.maximumPoints ?? "") ||
    BigInt(quote.estimatedPoints!) > BigInt(quote.maximumPoints!) ||
    !Number.isFinite(quoteExpiresAt(quote.expiresAt))
  )
    throw new Error("报价信息不完整，请重新读取费用");
  if (quoteExpiresAt(quote.expiresAt) <= now)
    throw new Error("报价已失效，请重新核对生成费用");
  return quote;
}
