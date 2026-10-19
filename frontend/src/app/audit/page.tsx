import { PageLayout } from "@/components/layout/page-layout";
import { UnavailableState } from "@/components/availability/unavailable";
export const metadata = { title: "审计日志 · 织光" };
export default function Page() {
  return (
    <PageLayout screen="audit">
      <h1 className="text-2xl">审计日志</h1>
      <UnavailableState
        title="审计日志暂未开放"
        description="此功能尚未接入服务。你可以从创作页进入画布，继续编辑和保存草稿。"
      />
    </PageLayout>
  );
}
