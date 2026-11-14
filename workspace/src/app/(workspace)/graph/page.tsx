import { getSnapshot, categoryNames } from "@/content/runtime";
import { Relations } from "@/components/relations";
export const metadata = { title: "知识关系" };
export default async function GraphPage() {
  const { documents } = await getSnapshot();
  const nodes = documents.map((doc) => {
    const references = [...doc.html.matchAll(/href="\/read\/([\w-]+)\//g)].map(
      (m) => m[1],
    );
    const declared = doc.source
      .split("\n")
      .find((s) => /唯一(?:来源|原始需求)/.test(s));
    const sourceIds = declared
      ? documents
          .filter(
            (d) =>
              d.id !== doc.id &&
              (declared.includes(d.path.split("/").at(-1)!) ||
                declared.includes(d.id)),
          )
          .map((d) => d.id)
      : [];
    return {
      id: doc.id,
      title: doc.title,
      category: categoryNames[doc.category] ?? doc.category,
      references: [...new Set(references)],
      sourceIds,
    };
  });
  return (
    <div className="page">
      <div className="page-heading">
        <div>
          <h1>知识关系</h1>
          <p>沿唯一来源查看决策链，普通资料引用单独展示。</p>
        </div>
      </div>
      <Relations nodes={nodes} />
    </div>
  );
}
