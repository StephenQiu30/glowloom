import { PageLayout } from "@/components/layout/page-layout";
import { TeamPage } from "./_components/team-page";
export const metadata = { title: "团队管理" };
export default async function Page({
  params,
}: {
  params: Promise<{ teamId: string }>;
}) {
  const { teamId } = await params;
  return (
    <PageLayout screen="teams">
      <TeamPage id={teamId} />
    </PageLayout>
  );
}
