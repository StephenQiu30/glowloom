import { PageLayout } from "@/components/layout/page-layout";
import { AnnouncementEditing } from "@/components/notifications/announcement-editing";
export const metadata = { title: "编辑通知" };
export default async function Page({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const { id } = await params;
  return (
    <PageLayout screen="platform/notifications">
      <AnnouncementEditing key={id} id={id} />
    </PageLayout>
  );
}
