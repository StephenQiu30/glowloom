import { ProductShell } from "@/components/layout/product-shell";
import { UnavailableState } from "@/components/availability/unavailable";
export const metadata = { title: "系统健康 · 织光" };
export default function Page() {
  return (
    <ProductShell screen="health">
      <h1 className="text-2xl">系统健康</h1>
      <UnavailableState
        title="系统健康暂未开放"
        description="此功能尚未接入服务。你可以从创作页进入画布，继续编辑和保存草稿。"
      />
    </ProductShell>
  );
}
