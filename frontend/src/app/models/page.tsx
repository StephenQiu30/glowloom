import { PageLayout } from "@/components/layout/page-layout";
import { ModelsPage } from "./_components/models-page";
export const metadata = { title: "模型管理 · 织光" };
export default async function Page({
  searchParams,
}: {
  searchParams: Promise<{
    provider?: string | string[];
    add?: string;
    model?: string;
  }>;
}) {
  const query = await searchParams;
  const provider = typeof query.provider === "string" ? query.provider : "";
  return (
    <PageLayout screen="models">
      <ModelsPage
        key={JSON.stringify([provider, query.add, query.model])}
        initialProvider={provider}
        openAdd={!!provider && query.add === "1"}
        initialReviewModel={
          typeof query.model === "string" &&
          /^[0-9a-f-]{36}$/i.test(query.model)
            ? query.model
            : undefined
        }
      />
    </PageLayout>
  );
}
