import { AssetWorkspace } from "./_components/asset-workspace";
import { readAssetLocation } from "./_components/asset-location";

export const metadata = { title: "资产" };
export default async function Page({
  searchParams,
}: {
  searchParams: Promise<Record<string, string | string[] | undefined>>;
}) {
  const params = new URLSearchParams();
  for (const [key, value] of Object.entries(await searchParams)) {
    if (typeof value === "string") params.set(key, value);
  }
  const initialLocation = readAssetLocation(params);
  return (
    <AssetWorkspace
      key={JSON.stringify(initialLocation)}
      initialLocation={initialLocation}
    />
  );
}
