import { PageLayout } from "@/components/layout/page-layout";
import { AiServiceAdmin } from "@/components/ai-service/ai-service-admin";

export const metadata = { title: "平台账务 · 织光" };

export default function Page() {
  return (
    <PageLayout screen="platform/billing">
      <AiServiceAdmin entry="billing" />
    </PageLayout>
  );
}
