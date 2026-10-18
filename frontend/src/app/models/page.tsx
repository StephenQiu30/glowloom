import { BasicLayout } from "@/components/layout/basic-layout";
import { ModelsPage } from "./_components/models-page";
export const metadata = { title: "模型管理 · 织光" };
export default async function Page({
  searchParams,
}: {
  searchParams: Promise<{ provider?: string | string[] }>;
}) {
  const query = await searchParams;
  const provider = typeof query.provider === "string" ? query.provider : "";
  return (
    <BasicLayout screen="models">
      <ModelsPage key={provider} initialProvider={provider} />
    </BasicLayout>
  );
}
