import { BillingWallet } from "@/components/billing/billing-wallet";
export const metadata = { title: "积分明细" };
export default function Page() {
  return <BillingWallet section="transactions" />;
}
