import { PageLayout } from "@/components/layout/page-layout";
import { TeamsPage } from "./_components/teams-page";
export const metadata = { title: "我的团队" };
export default function Page() {
  return (
    <PageLayout screen="teams">
      <TeamsPage />
    </PageLayout>
  );
}
