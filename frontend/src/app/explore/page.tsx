import { ExplorePage } from "./_components/explore-page";
export const metadata = { title: "探索 · 织光" };
export default async function Page({ searchParams }: PageProps<"/explore">) {
  const params = await searchParams;
  const category =
    typeof params.category === "string" ? params.category : undefined;
  return (
    <ExplorePage
      key={category}
      initialCategory={category}
      initialQuery={typeof params.q === "string" ? params.q : ""}
    />
  );
}
