import { ProductShell } from "@/components/layout/product-shell";
import { PlatformTeamsPage } from "./_components/platform-teams-page";
export const metadata = { title: "平台团队管理" };
export default function Page() {
  return (
    <ProductShell screen="platform/teams">
      <PlatformTeamsPage />
    </ProductShell>
  );
}
