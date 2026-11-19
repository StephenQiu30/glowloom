import { GatewayAdmin } from "@/components/gateway/gateway-admin";
import { PageLayout } from "@/components/layout/page-layout";

export const metadata = { title: "供应商路由 · 织光" };

export default async function Page({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const { id } = await params;
  return (
    <PageLayout screen="providers">
      <GatewayAdmin entry="providers" entityID={id} />
    </PageLayout>
  );
}
