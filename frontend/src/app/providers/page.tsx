import { PageLayout } from "@/components/layout/page-layout";
import { AiServiceAdmin } from "@/components/ai-service/ai-service-admin";

export const metadata = { title: "渠道 · 织光" };

export default async function Page({
  searchParams,
}: {
  searchParams: Promise<{ provider?: string }>;
}) {
  const { provider } = await searchParams;
  return (
    <PageLayout screen="providers">
      <AiServiceAdmin entry="providers" initialProvider={provider} />
    </PageLayout>
  );
}
