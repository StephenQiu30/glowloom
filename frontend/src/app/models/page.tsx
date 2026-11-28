import { PageLayout } from "@/components/layout/page-layout";
import { AiServiceAdmin } from "@/components/ai-service/ai-service-admin";

export const metadata = { title: "AI 服务 · 织光" };

export default function Page() {
  return (
    <PageLayout screen="models">
      <AiServiceAdmin />
    </PageLayout>
  );
}
