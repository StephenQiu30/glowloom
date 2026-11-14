import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import {
  Breadcrumb,
  BreadcrumbItem,
  BreadcrumbLink,
  BreadcrumbList,
  BreadcrumbPage,
  BreadcrumbSeparator,
} from '@/components/ui/breadcrumb';
import { Disclosure } from '@/components/disclosure';
import type { Metadata } from 'next';
import { notFound } from 'next/navigation';
import Link from 'next/link';
import { FileDown, GitCommitHorizontal, Pencil } from 'lucide-react';
import { getSnapshot, categoryNames } from '@/content/runtime';
import { NarrationPlayer } from '@/components/narration-player';

export async function generateStaticParams() {
  return (await getSnapshot()).documents.map((d) => ({ id: d.id }));
}
export const dynamicParams = false;
export async function generateMetadata({
  params,
}: {
  params: Promise<{ id: string }>;
}): Promise<Metadata> {
  const { id } = await params;
  return {
    title:
      (await getSnapshot()).documents.find((d) => d.id === id)?.title ??
      '文档未找到',
  };
}
export default async function Reader({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const { id } = await params;
  const snapshot = await getSnapshot();
  const doc = snapshot.documents.find((d) => d.id === id);
  if (!doc) {
    notFound();
  }
  const first = doc.headings[0];
  const html = doc.html.replace(/^<h1[^>]*>[\s\S]*?<\/h1>\n?/, '');
  return (
    <div className="reader-grid">
      <article
        className="reader-body"
        data-pagefind-body
        data-pagefind-filter={`category:${categoryNames[doc.category] ?? doc.category}`}
      >
        <Breadcrumb className="mb-6" data-pagefind-ignore>
          <BreadcrumbList>
            <BreadcrumbItem>
              <BreadcrumbLink asChild>
                <Link href="/">知识库</Link>
              </BreadcrumbLink>
            </BreadcrumbItem>
            <BreadcrumbSeparator />
            <BreadcrumbItem>
              <BreadcrumbPage>
                {categoryNames[doc.category] ?? doc.category}
              </BreadcrumbPage>
            </BreadcrumbItem>
          </BreadcrumbList>
        </Breadcrumb>
        <h1 className="reader-title" id={first?.id} data-pagefind-meta="title">
          {doc.title}
        </h1>
        <div className="reader-meta" data-pagefind-ignore>
          <Badge variant="outline">
            <GitCommitHorizontal aria-hidden="true" />
            {doc.revision.slice(0, 10)}
          </Badge>
          <Button asChild variant="ghost" size="sm">
            <a
              href={`${process.env.NEXT_PUBLIC_WORKSPACE_BASE_PATH ?? ''}/raw/${id}${doc.path.endsWith('.csv') ? '.csv' : '.md'}`}
              download={doc.path.split('/').at(-1)}
            >
              <FileDown data-icon="inline-start" aria-hidden="true" />
              原文
            </a>
          </Button>
          {process.env.NEXT_PUBLIC_WORKSPACE_MODE === 'online' &&
            doc.path.startsWith('content/') && (
              <Button asChild variant="ghost" size="sm">
                <Link href={`/operations/?document=${id}`}>
                  <Pencil data-icon="inline-start" aria-hidden="true" />
                  提出修改
                </Link>
              </Button>
            )}
        </div>
        <NarrationPlayer
          key={id + doc.revision}
          documentId={id}
          revision={doc.revision}
          chapters={doc.headings
            .filter((h) => h.depth === 2)
            .map((h) => ({
              id: h.id,
              text: h.text,
              blocks: doc.blocks
                .filter((b) => b.heading === h.id)
                .map((b) => b.id),
            }))}
        />
        <div className="mb-6 xl:hidden" data-pagefind-ignore>
          <Disclosure label="本页目录">
            <nav aria-label="本页目录" className="flex flex-col gap-1">
              {doc.headings
                .filter((heading) => heading.depth === 2 || heading.depth === 3)
                .map((heading) => (
                  <a
                    className="py-2 text-sm text-muted-foreground hover:text-foreground"
                    key={heading.id}
                    href={`#${encodeURIComponent(heading.id)}`}
                  >
                    {heading.text}
                  </a>
                ))}
            </nav>
          </Disclosure>
        </div>
        <div className="prose" dangerouslySetInnerHTML={{ __html: html }} />
      </article>
      <aside className="toc" aria-label="本页目录" data-pagefind-ignore>
        <h2>本页目录</h2>
        {doc.headings
          .filter((h) => h.depth === 2 || h.depth === 3)
          .map((h) => (
            <a
              href={`#${encodeURIComponent(h.id)}`}
              data-depth={h.depth}
              key={h.id}
            >
              {h.text}
            </a>
          ))}
      </aside>
    </div>
  );
}
