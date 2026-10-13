import { ProductShell } from "@/components/layout/product-shell";
import { UnavailableState } from "@/components/availability/unavailable";
export const metadata = { title: "供应商凭据 · 浮光" };
export default function Page() {
  return (
    <ProductShell screen="providers">
      <h1 className="text-2xl">供应商凭据</h1>
      <UnavailableState
        title="供应商凭据暂未开放"
        description="此功能尚未接入服务。你可以从创作页进入画布，继续编辑和保存草稿。"
      />
    </ProductShell>
  );
}
