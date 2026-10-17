import { BasicLayout } from "@/components/layout/basic-layout";
import { ProvidersPage } from "./_components/providers-page";
export const metadata = { title: "AI 平台接入 · 织光" };
export default function Page() {
  return (
    <BasicLayout screen="providers">
      <ProvidersPage />
    </BasicLayout>
  );
}
