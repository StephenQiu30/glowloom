import { BasicLayout } from "@/components/layout/basic-layout";
import { UnavailableState } from "@/components/availability/unavailable";
export const metadata = { title: "分镜 · 织光" };
export default function Page() {
  return (
    <BasicLayout screen="storyboard">
      <h1 className="text-2xl">分镜</h1>
      <UnavailableState
        title="分镜暂未开放"
        description="此功能尚未接入服务。你可以从创作页进入画布，继续编辑和保存草稿。"
      />
    </BasicLayout>
  );
}
