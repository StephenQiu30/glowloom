import { ProductShell } from "@/components/layout/product-shell";
import { ProvidersPage } from "./_components/providers-page";
export const metadata = { title: "AI 平台接入 · 织光" };
export default function Page() {
  return (
    <ProductShell screen="providers">
      <ProvidersPage />
    </ProductShell>
  );
}
