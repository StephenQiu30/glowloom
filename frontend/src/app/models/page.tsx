import { ProductShell } from "@/components/layout/product-shell";
import { ModelsPage } from "./_components/models-page";
export const metadata = { title: "模型管理 · 织光" };
export default function Page() {
  return (
    <ProductShell screen="models">
      <ModelsPage />
    </ProductShell>
  );
}
