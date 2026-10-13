import { ProductShell } from "@/components/layout/product-shell";
import { InspirationGallery } from "@/components/inspiration/gallery";
export const metadata = { title: "探索 · 浮光" };
export default function Page() {
  return (
    <ProductShell screen="explore">
      <h1 className="mb-8 text-2xl">发现创作的更多可能</h1>
      <InspirationGallery showUpdates={false} />
    </ProductShell>
  );
}
