import { ProductShell } from "@/components/layout/product-shell";
import { InspirationGallery } from "@/components/inspiration/gallery";
import type { AuthMode } from "./auth-dialog";

export function AuthEntry({ mode }: { mode: AuthMode }) {
  return (
    <ProductShell screen="home" allowGuest initialAuthMode={mode}>
      <InspirationGallery />
    </ProductShell>
  );
}
