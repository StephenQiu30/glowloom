import { PageLayout } from "@/components/layout/page-layout";
import { PlatformTeamsPage } from "./_components/platform-teams-page";
export const metadata = { title: "平台团队管理" };
export default function Page() {
  return (
    <PageLayout screen="platform/teams">
      <PlatformTeamsPage />
    </PageLayout>
  );
}
