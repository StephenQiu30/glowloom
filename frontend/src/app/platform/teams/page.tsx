import { BasicLayout } from "@/components/layout/basic-layout";
import { PlatformTeamsPage } from "./_components/platform-teams-page";
export const metadata = { title: "平台团队管理" };
export default function Page() {
  return (
    <BasicLayout screen="platform/teams">
      <PlatformTeamsPage />
    </BasicLayout>
  );
}
