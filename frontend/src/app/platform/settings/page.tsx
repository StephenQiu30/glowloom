import { PageLayout } from "@/components/layout/page-layout";
import { PlatformSettingsPage } from "./_components/platform-settings-page";
export const metadata = { title: "平台设置" };
export default function Page() {
  return (
    <PageLayout screen="platform/settings">
      <PlatformSettingsPage />
    </PageLayout>
  );
}
