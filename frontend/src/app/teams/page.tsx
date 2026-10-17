import { BasicLayout } from "@/components/layout/basic-layout";
import { TeamsPage } from "./_components/teams-page";
export const metadata = { title: "我的团队" };
export default function Page() {
  return (
    <BasicLayout screen="teams">
      <TeamsPage />
    </BasicLayout>
  );
}
