import { BillingView } from "@/components/billing/billing-view";
export const metadata = { title: "会员中心" };
export default function Page() {
  return <BillingView section="membership" />;
}
