export const billingSections = {
  overview: { label: "总览", title: "积分总览", href: "/billing" },
  transactions: {
    label: "明细",
    title: "积分明细",
    href: "/billing/transactions",
  },
  recharge: { label: "充值", title: "充值积分", href: "/billing/recharge" },
  membership: { label: "会员", title: "会员中心", href: "/billing/membership" },
} as const;
export type BillingSection = keyof typeof billingSections;
