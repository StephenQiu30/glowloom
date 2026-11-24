import { PageLayout } from "@/components/layout/page-layout";
import { GatewayAdmin } from "@/components/gateway/gateway-admin";
export const metadata = { title: "AI 服务 · 织光" };
export default async function Page({
  searchParams,
}: {
  searchParams: Promise<{ tab?: string | string[] }>;
}) {
  const { tab } = await searchParams;
  return (
    <PageLayout screen="models">
      <GatewayAdmin initialTab={typeof tab === "string" ? tab : undefined} />
    </PageLayout>
  );
}
