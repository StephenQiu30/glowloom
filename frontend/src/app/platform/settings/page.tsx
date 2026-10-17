import { BasicLayout } from "@/components/layout/basic-layout";
import { PlatformSettingsPage } from "./_components/platform-settings-page";
export const metadata = { title: "平台设置" };
export default function Page() {
  return (
    <BasicLayout screen="platform/settings">
      <PlatformSettingsPage />
    </BasicLayout>
  );
}
