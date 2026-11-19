import { PageLayout } from "@/components/layout/page-layout";
import { UnavailableState } from "@/components/availability/unavailable";
import { PageHeading } from "@/components/layout/page-heading";
export const metadata = { title: "镜头详情 · 织光" };
export default function Page() {
  return (
    <PageLayout screen="shot">
      <PageHeading title="镜头详情" />
      <UnavailableState title="镜头详情暂未开放" />
    </PageLayout>
  );
}
