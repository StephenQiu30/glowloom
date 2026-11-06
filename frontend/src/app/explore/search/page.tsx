import { ExploreSearchResults } from "./search-results";
export const metadata = { title: "搜索 · 织光" };
export default async function Page({
  searchParams,
}: PageProps<"/explore/search">) {
  const params = await searchParams;
  const query = typeof params.result === "string" ? params.result : "";
  const type =
    typeof params.type === "string" &&
    ["skill", "shortFilm"].includes(params.type)
      ? params.type
      : "inspiration";
  return (
    <ExploreSearchResults key={`${query}/${type}`} query={query} type={type} />
  );
}
