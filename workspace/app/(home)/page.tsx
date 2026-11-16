import Link from 'next/link';

export default function HomePage() {
  return (
    <div className="flex flex-col justify-center items-center text-center flex-1 px-4 py-16">
      <h1 className="text-4xl font-extrabold tracking-tight mb-4">
        织光知识库
      </h1>
      <p className="text-lg text-muted-foreground max-w-xl mb-8">
        包含项目需求分析、技术架构设计、产品规格（PRD）、实施计划与研发工作流的线上知识库系统。
      </p>
      <Link
        href="/docs"
        className="inline-flex items-center justify-center rounded-lg bg-primary px-6 py-3 text-sm font-semibold text-primary-foreground shadow transition hover:opacity-90"
      >
        进入文档中心
      </Link>
    </div>
  );
}
