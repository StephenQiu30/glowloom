import { ProductShell } from "@/components/layout/product-shell";
import { UnavailableState } from "@/components/availability/unavailable";
export const metadata = { title: "模型管理 · 浮光" };
export default function Page() {
  return (
    <ProductShell screen="models">
      <h1 className="text-2xl">模型管理</h1>
      <UnavailableState
        title="模型管理暂未开放"
        description="此功能尚未接入服务。你可以从创作页进入画布，继续编辑和保存草稿。"
      />
    </ProductShell>
  );
}
