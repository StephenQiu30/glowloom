import { Suspense } from "react";
import { PageLayout } from "@/components/layout/page-layout";
import { NotificationInboxRoute } from "@/components/notifications/notification-inbox-route";

export const metadata = { title: "通知中心" };
export default function Page() {
  return (
    <PageLayout screen="notifications">
      <Suspense>
        <NotificationInboxRoute />
      </Suspense>
    </PageLayout>
  );
}
