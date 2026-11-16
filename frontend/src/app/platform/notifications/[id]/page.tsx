import { PageLayout } from "@/components/layout/page-layout";
import { AnnouncementAdministration } from "@/components/notifications/announcement-administration";
export const metadata = { title: "通知详情" };
export default async function Page({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const { id } = await params;
  return (
    <PageLayout screen="platform/notifications">
      <AnnouncementAdministration key={id} id={id} />
    </PageLayout>
  );
}
