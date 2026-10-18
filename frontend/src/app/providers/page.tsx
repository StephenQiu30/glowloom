import { BasicLayout } from "@/components/layout/basic-layout";
import { ProvidersPage } from "./_components/providers-page";
export const metadata = { title: "AI 平台接入 · 织光" };
export default async function Page({
  searchParams,
}: {
  searchParams: Promise<{ connection?: string | string[] }>;
}) {
  const query = await searchParams;
  const connection =
    typeof query.connection === "string" ? query.connection : "";
  return (
    <BasicLayout screen="providers">
      <ProvidersPage initialConnection={connection} />
    </BasicLayout>
  );
}
