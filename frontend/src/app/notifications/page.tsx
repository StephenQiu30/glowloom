import { Suspense } from "react";
import { PageLayout } from "@/components/layout/page-layout";
import { NotificationInbox } from "@/components/notifications/notification-inbox";

export const metadata = { title: "通知中心" };
export default function Page() {
  return (
    <PageLayout screen="notifications">
      <Suspense>
        <NotificationInbox />
      </Suspense>
    </PageLayout>
  );
}
