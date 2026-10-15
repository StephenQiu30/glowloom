import { ProductShell } from "@/components/layout/product-shell";
import { InvitationsPage } from "./_components/invitations-page";
export const metadata = { title: "邀请与授权" };
export default function Page() {
  return (
    <ProductShell screen="invitations" allowGuest>
      <InvitationsPage />
    </ProductShell>
  );
}
