import Link from "next/link";
import { GitBranch, ArrowUpRight } from "lucide-react";
import { getSnapshot } from "@/content/runtime";
export const metadata = { title: "研发工作流" };
export default async function WorkflowsPage() {
  const { documents } = await getSnapshot();
  return (
    <div className="page">
      <div className="page-heading">
        <div>
          <h1>研发工作流</h1>
          <p>明确每阶段的输入、产物、验证与交接。</p>
        </div>
      </div>
      <div className="collection-grid">
        {documents
          .filter((d) => d.category === "workflows")
          .map((d) => (
            <Link
              className="collection-card"
              key={d.id}
              href={`/read/${d.id}/`}
            >
              <GitBranch />
              <h2>{d.title}</h2>
              <p>{d.blocks[0]?.text.slice(0, 100)}</p>
              <footer>
                <span>版本 {d.revision.slice(0, 10)}</span>
                <ArrowUpRight />
              </footer>
            </Link>
          ))}
      </div>
    </div>
  );
}
