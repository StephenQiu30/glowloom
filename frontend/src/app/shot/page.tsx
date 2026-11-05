import { PageLayout } from "@/components/layout/page-layout";
import { UnavailableState } from "@/components/availability/unavailable";
export const metadata = { title: "镜头详情 · 织光" };
export default function Page() {
  return (
    <PageLayout screen="shot">
      <h1 className="text-2xl">镜头详情</h1>
      <UnavailableState title="镜头详情暂未开放" />
    </PageLayout>
  );
}
