import { AssetWorkspace } from "./_components/asset-workspace";

export const metadata = { title: "资产" };
export default async function Page({
  searchParams,
}: {
  searchParams: Promise<{ view?: string }>;
}) {
  const { view } = await searchParams;
  const initialView =
    view === "canvas" || view === "short_drama" || view === "reference"
      ? view
      : "history";
  return <AssetWorkspace key={initialView} initialView={initialView} />;
}
