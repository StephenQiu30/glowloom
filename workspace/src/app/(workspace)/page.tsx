import Link from "next/link";
import {
  BookOpen,
  Boxes,
  GitBranch,
  FileText,
  ArrowUpRight,
  Search,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { getSnapshot, categoryNames } from "@/content/runtime";

export default async function Home() {
  const snapshot = await getSnapshot();
  const knowledge = snapshot.documents.filter((d) =>
    d.path.startsWith("content/"),
  );
  const featured = ["requirement-28", "design-43", "prd-29", "plan-38"]
    .map((id) => snapshot.documents.find((d) => d.id === id))
    .filter((d) => !!d);
  return (
    <div className="page">
      <div className="page-heading">
        <div>
          <h1>知识库</h1>
          <p>从项目背景到交付验收，找到当前有效的上下文。</p>
        </div>
        <Button asChild variant="outline">
          <Link href="/search/">
            <Search data-icon="inline-start" />
            搜索文档
          </Link>
        </Button>
      </div>
      <div className="overview-grid">
        {[
          {
            label: "项目文档",
            count: knowledge.length,
            unit: "篇",
            icon: BookOpen,
          },
          {
            label: "项目 Skill",
            count: snapshot.documents.filter((d) => d.category === "skills")
              .length,
            unit: "项",
            icon: Boxes,
          },
          {
            label: "工作流定义",
            count: snapshot.documents.filter((d) => d.category === "workflows")
              .length,
            unit: "套",
            icon: GitBranch,
          },
        ].map(({ label, count, unit, icon: Icon }) => (
          <div className="overview-item" key={label}>
            <p>
              <Icon />
              {label}
            </p>
            <strong>{count}</strong>
            <span>{unit}</span>
          </div>
        ))}
      </div>
      <div className="home-columns">
        <section>
          <h2 className="section-title">
            Workspace 设计与交付<Link href="/graph/">查看来源关系</Link>
          </h2>
          <div className="document-list">
            {featured.map((doc) => (
              <Link
                key={doc.id}
                className="document-row"
                href={`/read/${doc.id}/`}
              >
                <FileText />
                <div>
                  <h3>{doc.title}</h3>
                  <p>
                    {categoryNames[doc.category] ?? doc.category}　
                    {doc.sourcePath}
                  </p>
                </div>
                <ArrowUpRight className="row-arrow" />
              </Link>
            ))}
          </div>
          <h2 className="section-title" style={{ marginTop: 40 }}>
            按分类阅读
          </h2>
          <div className="document-list">
            {[
              "requirement",
              "design",
              "prd",
              "plan",
              "research",
              "templates",
            ].map((category) => {
              const docs = knowledge.filter((d) => d.category === category);
              const index = docs.find((d) => /\/00-/.test(d.path)) ?? docs[0];
              return index ? (
                <Link
                  key={category}
                  className="document-row"
                  href={`/read/${index.id}/`}
                >
                  <BookOpen />
                  <div>
                    <h3>{categoryNames[category]}</h3>
                    <p>{docs.length} 篇文档</p>
                  </div>
                  <ArrowUpRight className="row-arrow" />
                </Link>
              ) : null;
            })}
          </div>
        </section>
        <aside className="home-aside">
          <section className="aside-section">
            <h2 className="section-title">关联工程</h2>
            {[
              ["frontend", "产品前端"],
              ["backend", "业务后端"],
              ["agent", "创作 Agent"],
              ["canvas", "画布协作"],
            ].map(([id, label]) => (
              <Link
                key={id}
                className="engineering-link"
                href={`/engineering/#${id}`}
              >
                <span>{label}</span>
                <code>{id}/</code>
              </Link>
            ))}
          </section>
          <section className="aside-section">
            <h2 className="section-title">一份知识，多种入口</h2>
            <p className="subtle-note">
              页面、搜索、Skill 和任务交接都引用同一份 Git
              原文。查看文档版本后，再进入工程实施。
            </p>
            <Button asChild variant="outline" size="sm">
              <Link href="/workflows/">了解协作流程</Link>
            </Button>
          </section>
        </aside>
      </div>
    </div>
  );
}
