import Link from 'next/link';
import { GitBranch, ArrowUpRight } from 'lucide-react';
import { Button } from '@/components/ui/button';
import {
  Card,
  CardHeader,
  CardTitle,
  CardDescription,
  CardContent,
  CardFooter,
} from '@/components/ui/card';
import { Badge } from '@/components/ui/badge';
import { getSnapshot } from '@/content/runtime';
export const metadata = { title: '研发工作流' };
export default async function CollectionPage() {
  const { documents } = await getSnapshot();
  return (
    <div className="page flex flex-col gap-8">
      <div className="page-heading">
        <div>
          <h1>研发工作流</h1>
          <p>明确每阶段的输入、产物、验证与交接。</p>
        </div>
      </div>
      <div className="grid items-stretch gap-4 md:grid-cols-2 xl:grid-cols-3">
        {documents
          .filter((document) => document.category === 'workflows')
          .map((document) => (
            <Card key={document.id}>
              <CardHeader>
                <GitBranch
                  aria-hidden="true"
                  className="mb-2 size-5 text-muted-foreground"
                />
                <CardTitle>{document.title}</CardTitle>
              </CardHeader>
              <CardContent className="flex-1">
                <CardDescription>
                  {document.blocks
                    .find((block) => !block.text.startsWith('name:'))
                    ?.text.slice(0, 120)}
                </CardDescription>
              </CardContent>
              <CardFooter className="flex flex-wrap items-center justify-between gap-3">
                <Badge variant="outline">
                  版本 {document.revision.slice(0, 10)}
                </Badge>
                <Button asChild variant="outline">
                  <Link href={`/read/${document.id}/`}>
                    阅读定义
                    <ArrowUpRight data-icon="inline-end" aria-hidden="true" />
                    <span className="sr-only">{document.title}</span>
                  </Link>
                </Button>
              </CardFooter>
            </Card>
          ))}
      </div>
      <p className="text-sm text-muted-foreground">
        查看定义不会执行脚本。主机、依赖和权限在实际任务进入时核对。
      </p>
    </div>
  );
}
