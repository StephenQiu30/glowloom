import { PageLayout } from "@/components/layout/page-layout";
import { GatewayAdmin } from "@/components/gateway/gateway-admin";
export const metadata = { title: "AI 服务接入 · 织光" };
export default function Page() {
  return (
    <PageLayout screen="providers">
      <GatewayAdmin />
    </PageLayout>
  );
}
