import Link from 'next/link';
import { ArrowUpRight, FolderGit2 } from 'lucide-react';
import { Button } from '@/components/ui/button';
import {
  Card,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from '@/components/ui/card';
import { DocumentList } from '@/components/document-list';
import { getSnapshot } from '@/content/runtime';
export const metadata = { title: '工程引用' };
export default async function EngineeringPage() {
  const { documents } = await getSnapshot();
  return (
    <div className="page flex flex-col gap-8">
      <div className="page-heading">
        <div>
          <h1>工程引用</h1>
          <p>业务源码与契约保留在原工程，Workspace 关联知识和验证入口。</p>
        </div>
      </div>
      <div className="grid gap-4 md:grid-cols-2">
        {[
          {
            id: 'frontend',
            name: '产品前端',
            detail: 'Next.js、React、shadcn；界面与浏览器验收。',
          },
          {
            id: 'backend',
            name: '业务后端',
            detail: 'Go、业务 API、唯一数据库 schema 与公共预设。',
          },
          {
            id: 'agent',
            name: '创作 Agent',
            detail: '产品会话、模型执行与创作恢复。',
          },
          {
            id: 'canvas',
            name: '画布协作',
            detail: 'Yjs 实时协作与画布计算。',
          },
        ].map((project) => {
          const rule = documents.find(
            (document) => document.id === `root-${project.id}-agents`,
          );
          return (
            <Card id={project.id} key={project.id}>
              <CardHeader>
                <FolderGit2
                  aria-hidden="true"
                  className="mb-2 size-5 text-muted-foreground"
                />
                <CardTitle>{project.name}</CardTitle>
                <CardDescription>{project.detail}</CardDescription>
              </CardHeader>
              <CardContent>
                <code className="text-xs text-muted-foreground">
                  {project.id}/
                </code>
              </CardContent>
              <CardFooter>
                {rule && (
                  <Button asChild variant="outline">
                    <Link href={`/read/${rule.id}/`}>
                      阅读{project.name}规范
                      <ArrowUpRight data-icon="inline-end" aria-hidden="true" />
                    </Link>
                  </Button>
                )}
              </CardFooter>
            </Card>
          );
        })}
      </div>
      <section className="flex flex-col gap-4">
        <h2 className="section-title">项目规范</h2>
        <DocumentList
          documents={documents
            .filter(
              (document) =>
                document.path.startsWith('../') &&
                !document.path.slice(3).includes('/'),
            )
            .map(({ id, title, sourcePath }) => ({
              id,
              title,
              description: sourcePath,
            }))}
        />
      </section>
      <p className="text-sm text-muted-foreground">
        命令以各工程包清单与 PROJECT 为准；个人路径不进入共享产物。
      </p>
    </div>
  );
}
