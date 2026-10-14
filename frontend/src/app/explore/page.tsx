import { ProductShell } from "@/components/layout/product-shell";
import { InspirationGallery } from "@/components/inspiration/gallery";
export const metadata = { title: "探索 · 织光" };
export default function Page() {
  return (
    <ProductShell screen="explore" allowGuest>
      <h1 className="sr-only">探索创作灵感</h1>
      <InspirationGallery showUpdates={false} />
    </ProductShell>
  );
}
