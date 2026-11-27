import { AiServiceAdmin } from "@/components/ai-service/ai-service-admin";
import { PageLayout } from "@/components/layout/page-layout";

export const metadata = { title: "模型版本 · 织光" };

export default async function Page({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const { id } = await params;
  return (
    <PageLayout screen="models">
      <AiServiceAdmin entry="models" entityID={id} />
    </PageLayout>
  );
}
