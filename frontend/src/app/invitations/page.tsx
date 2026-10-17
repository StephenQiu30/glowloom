import { BasicLayout } from "@/components/layout/basic-layout";
import { InvitationsPage } from "./_components/invitations-page";
export const metadata = { title: "邀请与授权" };
export default function Page() {
  return (
    <BasicLayout screen="invitations" allowGuest>
      <InvitationsPage />
    </BasicLayout>
  );
}
