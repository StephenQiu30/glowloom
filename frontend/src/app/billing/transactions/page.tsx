import { BillingView } from "@/components/billing/billing-view";
export const metadata = { title: "积分明细" };
export default function Page() {
  return <BillingView section="transactions" />;
}
