import { PublicWork } from "./_components/public-work";

export const metadata = {
  title: "作品 · 织光",
  robots: { index: false, follow: false },
};
export default async function Page({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const { id } = await params;
  return <PublicWork key={id} id={id} />;
}
