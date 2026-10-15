import { ProductShell } from "@/components/layout/product-shell";
import { PlatformSettingsPage } from "./_components/platform-settings-page";
export const metadata = { title: "平台设置" };
export default function Page() {
  return (
    <ProductShell screen="platform/settings">
      <PlatformSettingsPage />
    </ProductShell>
  );
}
