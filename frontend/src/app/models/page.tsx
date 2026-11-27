import { PageLayout } from "@/components/layout/page-layout";
import { AiServiceAdmin } from "@/components/ai-service/ai-service-admin";
export const metadata = { title: "AI 服务 · 织光" };
export default async function Page({
  searchParams,
}: {
  searchParams: Promise<{ tab?: string | string[] }>;
}) {
  const { tab } = await searchParams;
  return (
    <PageLayout screen="models">
      <AiServiceAdmin initialTab={typeof tab === "string" ? tab : undefined} />
    </PageLayout>
  );
}
