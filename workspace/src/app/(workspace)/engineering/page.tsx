import Link from "next/link";
import { FolderGit2, ArrowUpRight } from "lucide-react";
import { getSnapshot } from "@/content/runtime";
export const metadata = { title: "工程引用" };
export default async function EngineeringPage() {
  const { documents } = await getSnapshot();
  return (
    <div className="page">
      <div className="page-heading">
        <div>
          <h1>工程引用</h1>
          <p>业务源码与契约保留在原工程，Workspace 关联知识和验证入口。</p>
        </div>
      </div>
      <div className="collection-grid">
        {[
          {
            id: "frontend",
            name: "产品前端",
            detail: "Next.js、React、shadcn；界面与浏览器接受。",
          },
          {
            id: "backend",
            name: "业务后端",
            detail: "Go、业务 API、唯一数据库 schema 与公共预设。",
          },
          {
            id: "agent",
            name: "创作 Agent",
            detail: "产品会话、模型执行与创作恢复。",
          },
          {
            id: "canvas",
            name: "画布协作",
            detail: "Yjs 实时协作与画布计算。",
          },
        ].map((p) => {
          const rule = documents.find((d) => d.id === `root-${p.id}-agents`);
          return (
            <section id={p.id} className="collection-card" key={p.id}>
              <FolderGit2 />
              <h2>{p.name}</h2>
              <p>{p.detail}</p>
              <footer>
                <code>{p.id}/</code>
                {rule && (
                  <Link
                    href={`/read/${rule.id}/`}
                    aria-label={`阅读${p.name}规范`}
                  >
                    <ArrowUpRight />
                  </Link>
                )}
              </footer>
            </section>
          );
        })}
      </div>
      <h2 className="section-title" style={{ marginTop: 36 }}>
        项目规范
      </h2>
      <div className="document-list">
        {documents
          .filter(
            (d) => d.path.startsWith("../") && !d.path.slice(3).includes("/"),
          )
          .map((d) => (
            <Link className="document-row" href={`/read/${d.id}/`} key={d.id}>
              <div>
                <h3>{d.title}</h3>
                <p>{d.sourcePath}</p>
              </div>
              <ArrowUpRight />
            </Link>
          ))}
      </div>
      <p className="subtle-note" style={{ marginTop: 24 }}>
        命令以各工程包清单与 PROJECT 为准；个人路径在本机配置，不进入共享产物。
      </p>
    </div>
  );
}
