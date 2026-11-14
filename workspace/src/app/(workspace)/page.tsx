import Link from 'next/link';
import { ArrowUpRight, BookOpen, Boxes, GitBranch, Search } from 'lucide-react';
import { Button } from '@/components/ui/button';
import {
  Card,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from '@/components/ui/card';
import { Item, ItemContent, ItemGroup, ItemTitle } from '@/components/ui/item';
import { DocumentList } from '@/components/document-list';
import { getSnapshot, categoryNames } from '@/content/runtime';

export default async function Home() {
  const snapshot = await getSnapshot();
  const knowledge = snapshot.documents.filter((document) =>
    document.path.startsWith('content/'),
  );
  const featured = ['requirement-28', 'design-43', 'prd-29', 'plan-38'].flatMap(
    (id) => {
      const document = snapshot.documents.find((item) => item.id === id);
      return document
        ? [
            {
              id,
              title: document.title,
              description: categoryNames[document.category],
            },
          ]
        : [];
    },
  );
  return (
    <div className="page flex flex-col gap-8">
      <div className="page-heading">
        <div>
          <p className="eyebrow">GLOWLOOM WORKSPACE</p>
          <h1>知识库</h1>
          <p>从项目背景到交付验收，找到当前有效的上下文。</p>
        </div>
        <Button asChild variant="outline">
          <Link href="/search/">
            <Search data-icon="inline-start" aria-hidden="true" />
            搜索文档
          </Link>
        </Button>
      </div>
      <div className="grid gap-4 sm:grid-cols-3">
        {[
          {
            label: '项目文档',
            count: knowledge.length,
            unit: '篇',
            icon: BookOpen,
          },
          {
            label: '项目 Skill',
            count: snapshot.documents.filter(
              (item) => item.category === 'skills',
            ).length,
            unit: '项',
            icon: Boxes,
          },
          {
            label: '工作流定义',
            count: snapshot.documents.filter(
              (item) => item.category === 'workflows',
            ).length,
            unit: '套',
            icon: GitBranch,
          },
        ].map(({ label, count, unit, icon: Icon }) => (
          <Card key={label}>
            <CardHeader>
              <CardDescription className="flex items-center gap-2">
                <Icon aria-hidden="true" className="size-4" />
                {label}
              </CardDescription>
              <CardTitle>
                <span className="text-3xl tabular-nums">{count}</span>
                <span className="ml-2 text-sm font-normal text-muted-foreground">
                  {unit}
                </span>
              </CardTitle>
            </CardHeader>
          </Card>
        ))}
      </div>
      <div className="grid items-start gap-8 xl:grid-cols-[minmax(0,1fr)_17rem]">
        <div className="flex min-w-0 flex-col gap-8">
          <section className="flex flex-col gap-4">
            <div className="flex flex-wrap items-center justify-between gap-2">
              <h2 className="section-title">Workspace 设计与交付</h2>
              <Button asChild variant="ghost" size="sm">
                <Link href="/graph/">
                  来源关系
                  <GitBranch data-icon="inline-end" aria-hidden="true" />
                </Link>
              </Button>
            </div>
            <DocumentList documents={featured} />
          </section>
          <section className="flex flex-col gap-4">
            <h2 className="section-title">按分类阅读</h2>
            <div className="grid gap-3 sm:grid-cols-2">
              {[
                'requirement',
                'design',
                'prd',
                'plan',
                'research',
                'templates',
              ].map((category) => {
                const documents = knowledge.filter(
                  (item) => item.category === category,
                );
                const index =
                  documents.find((item) => /\/00-/.test(item.path)) ??
                  documents[0];
                return index ? (
                  <Card key={category}>
                    <CardHeader>
                      <CardTitle>
                        <Link
                          href={`/read/${index.id}/`}
                          className="inline-flex min-h-6 items-center gap-2 hover:underline"
                        >
                          {categoryNames[category]}
                          <ArrowUpRight aria-hidden="true" className="size-4" />
                        </Link>
                      </CardTitle>
                      <CardDescription>
                        {documents.length} 篇文档
                      </CardDescription>
                    </CardHeader>
                  </Card>
                ) : null;
              })}
            </div>
          </section>
        </div>
        <aside className="flex flex-col gap-6">
          <Card>
            <CardHeader>
              <CardTitle>关联工程</CardTitle>
              <CardDescription>从规范进入对应工程</CardDescription>
            </CardHeader>
            <CardContent>
              <ItemGroup>
                {[
                  ['frontend', '产品前端'],
                  ['backend', '业务后端'],
                  ['agent', '创作 Agent'],
                  ['canvas', '画布协作'],
                ].map(([id, label]) => (
                  <div key={id} role="listitem">
                    <Item asChild size="sm">
                      <Link href={`/engineering/#${id}`}>
                        <ItemContent>
                          <ItemTitle>{label}</ItemTitle>
                        </ItemContent>
                        <ArrowUpRight aria-hidden="true" className="size-4" />
                      </Link>
                    </Item>
                  </div>
                ))}
              </ItemGroup>
            </CardContent>
          </Card>
          <Card>
            <CardHeader>
              <CardTitle>一份知识，多种入口</CardTitle>
              <CardDescription>
                页面、搜索、Skill 和交接包引用同一份 Git 原文。
              </CardDescription>
            </CardHeader>
            <CardFooter>
              <Button asChild variant="outline">
                <Link href="/workflows/">
                  查看协作流程
                  <ArrowUpRight data-icon="inline-end" aria-hidden="true" />
                </Link>
              </Button>
            </CardFooter>
          </Card>
        </aside>
      </div>
    </div>
  );
}
