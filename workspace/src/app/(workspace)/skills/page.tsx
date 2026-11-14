import Link from "next/link";
import { Boxes, ArrowUpRight } from "lucide-react";
import { getSnapshot } from "@/content/runtime";
export const metadata = { title: "项目 Skill" };
export default async function SkillsPage() {
  const { documents } = await getSnapshot();
  return (
    <div className="page">
      <div className="page-heading">
        <div>
          <h1>项目 Skill</h1>
          <p>按任务取用能力，先核对适用范围和执行权限。</p>
        </div>
      </div>
      <div className="collection-grid">
        {documents
          .filter((d) => d.category === "skills")
          .map((d) => (
            <Link
              className="collection-card"
              key={d.id}
              href={`/read/${d.id}/`}
            >
              <Boxes />
              <h2>{d.title}</h2>
              <p>
                {d.blocks
                  .find((b) => !b.text.startsWith("name:"))
                  ?.text.slice(0, 100)}
              </p>
              <footer>
                <span>版本 {d.revision.slice(0, 10)}</span>
                <ArrowUpRight />
              </footer>
            </Link>
          ))}
      </div>
      <p className="subtle-note" style={{ marginTop: 24 }}>
        查看定义不会执行脚本。主机发现、依赖和权限在实际任务进入时核对。
      </p>
    </div>
  );
}
