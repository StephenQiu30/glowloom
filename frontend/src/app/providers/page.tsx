import { ProvidersWorkspace } from "./_components/providers-workspace";
export const metadata = { title: "AI 服务接入 · 织光" };
export default async function Page({
  searchParams,
}: {
  searchParams: Promise<{
    connection?: string | string[];
    model?: string | string[];
    release?: string | string[];
    mode?: string | string[];
  }>;
}) {
  const query = await searchParams;
  const connection =
    typeof query.connection === "string" ? query.connection : "";
  const value = (v: unknown) => (typeof v === "string" ? v : undefined);
  return (
    <ProvidersWorkspace
      initialConnection={connection}
      initialModel={value(query.model)}
      initialRelease={value(query.release)}
      initialMode={value(query.mode)}
    />
  );
}
