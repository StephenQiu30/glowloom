import { PageLayout } from "@/components/layout/page-layout";
import { UnavailableState } from "@/components/availability/unavailable";
import { PageHeader } from "@/components/layout/page-header";
export const metadata = { title: "镜头详情 · 织光" };
export default function Page() {
  return (
    <PageLayout screen="shot">
      <PageHeader title="镜头详情" />
      <UnavailableState title="镜头详情暂未开放" />
    </PageLayout>
  );
}
