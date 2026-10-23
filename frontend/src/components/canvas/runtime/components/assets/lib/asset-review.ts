type Review = {
  policy?: string;
  status?: string;
  stage?: string;
  basis?: string;
};

export function assetReviewLabel(review?: Review): string {
  if (review?.policy === "EXEMPT_PLATFORM_GENERATED") return "平台生成·免审核";
  if (review?.status === "APPROVED")
    return review.basis === "PLATFORM_PRE_REVIEW"
      ? "预审通过·无需火山人像审核"
      : review.basis === "ARK_AIGC_ASSET"
        ? "火山素材审核通过"
        : "审核依据待核实";
  if (review?.status === "REVIEWING")
    return review.stage === "WAITING_ARK"
      ? "预审通过·待火山审核"
      : review.stage === "NEEDS_INFO"
        ? "待补充资料"
        : "待预审核";
  return review?.status === "REJECTED" ? "审核未通过" : "未审核";
}

/** Presentation only; every write and execution still uses server authority. */
export function assetReferenceAllowed(review?: Review): boolean {
  return (
    review?.policy === "EXEMPT_PLATFORM_GENERATED" ||
    (review?.status === "APPROVED" &&
      ["PLATFORM_PRE_REVIEW", "ARK_AIGC_ASSET"].includes(review.basis ?? ""))
  );
}
