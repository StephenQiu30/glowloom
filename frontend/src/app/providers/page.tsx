import { ProvidersWorkspace } from "./_components/providers-workspace";
export const metadata = { title: "AI 服务接入 · 织光" };
export default async function Page({
  searchParams,
}: {
  searchParams: Promise<{ connection?: string | string[] }>;
}) {
  const query = await searchParams;
  const connection =
    typeof query.connection === "string" ? query.connection : "";
  return <ProvidersWorkspace initialConnection={connection} />;
}
