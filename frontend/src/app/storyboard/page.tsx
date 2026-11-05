import { PageLayout } from "@/components/layout/page-layout";
import { UnavailableState } from "@/components/availability/unavailable";
export const metadata = { title: "分镜 · 织光" };
export default function Page() {
  return (
    <PageLayout screen="storyboard">
      <h1 className="text-2xl">分镜</h1>
      <UnavailableState title="分镜暂未开放" />
    </PageLayout>
  );
}
