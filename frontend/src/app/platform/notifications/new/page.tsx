import { PageLayout } from "@/components/layout/page-layout";
import { AnnouncementEditor } from "@/components/notifications/announcement-editor";
export const metadata = { title: "新建通知" };
export default function Page() {
  return (
    <PageLayout screen="platform/notifications">
      <AnnouncementEditor />
    </PageLayout>
  );
}
