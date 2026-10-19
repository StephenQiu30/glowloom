import { PageLayout } from "@/components/layout/page-layout";
import { InvitationsPage } from "./_components/invitations-page";
export const metadata = { title: "邀请与授权" };
export default function Page() {
  return (
    <PageLayout screen="invitations">
      <InvitationsPage />
    </PageLayout>
  );
}
