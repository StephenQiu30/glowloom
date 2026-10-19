import { PageLayout } from "@/components/layout/page-layout";
import { UnavailableState } from "@/components/availability/unavailable";
export const metadata = { title: "创作统计 · 织光" };
export default function Page() {
  return (
    <PageLayout screen="analytics">
      <h1 className="text-2xl">创作统计</h1>
      <UnavailableState
        title="创作统计暂未开放"
        description="此功能尚未接入服务。你可以从创作页进入画布，继续编辑和保存草稿。"
      />
    </PageLayout>
  );
}
