import { ProductShell } from "@/components/layout/product-shell";
import { TeamsPage } from "./_components/teams-page";
export const metadata = { title: "我的团队" };
export default function Page() {
  return (
    <ProductShell screen="teams">
      <TeamsPage />
    </ProductShell>
  );
}
