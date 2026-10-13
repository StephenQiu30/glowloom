import { ProductShell } from "@/components/layout/product-shell";
import { UnavailableState } from "@/components/availability/unavailable";
export const metadata = { title: "镜头详情 · 织光" };
export default function Page() {
  return (
    <ProductShell screen="shot">
      <h1 className="text-2xl">镜头详情</h1>
      <UnavailableState
        title="镜头详情暂未开放"
        description="此功能尚未接入服务。你可以从创作页进入画布，继续编辑和保存草稿。"
      />
    </ProductShell>
  );
}
