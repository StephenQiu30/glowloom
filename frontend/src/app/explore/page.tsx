import { ExplorePage } from "./_components/explore-page";
export const metadata = { title: "探索 · 织光" };
export default async function Page({ searchParams }: PageProps<"/explore">) {
  const params = await searchParams;
  return (
    <ExplorePage initialQuery={typeof params.q === "string" ? params.q : ""} />
  );
}
