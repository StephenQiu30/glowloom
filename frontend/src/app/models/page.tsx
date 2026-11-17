import { PageLayout } from "@/components/layout/page-layout";
import { GatewayAdmin } from "@/components/gateway/gateway-admin";
export const metadata = { title: "模型管理 · 织光" };
export default function Page() {
  return (
    <PageLayout screen="models">
      <GatewayAdmin />
    </PageLayout>
  );
}
