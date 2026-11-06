import { HomePage } from "./_components/home-page";

export const metadata = { title: "创作 · 织光" };

export default async function Page({
  searchParams,
}: {
  searchParams: Promise<{ mode?: string | string[] }>;
}) {
  const { mode } = await searchParams;
  return (
    <HomePage
      initialType={mode === "video" || mode === "avatar" ? mode : "image"}
    />
  );
}
