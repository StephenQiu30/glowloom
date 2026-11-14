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
import { getDocumentSource, categoryNames } from '@/content/runtime';
import { findNeighbour } from 'fumadocs-core/page-tree';
import {
  DocumentAnchors,
  DocumentOutline,
} from '@/components/document-outline';
import { NarrationPlayer } from '@/components/narration-player';

export async function generateStaticParams() {
  return (await getDocumentSource())
    .generateParams('id')
    .map(({ id }) => ({ id: id[0] }));
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
      (await getDocumentSource()).getPage([id])?.data.title ?? '文档未找到',
  };
}
export default async function Reader({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const { id } = await params;
  const source = await getDocumentSource();
  const page = source.getPage([id]);
  const doc = page?.data.document;
  if (!doc) {
    notFound();
  }
  const first =
    doc.headings.find((heading) => heading.depth === 1) ?? doc.headings[0];
  const html = doc.html.replace(/<h1[^>]*>[\s\S]*?<\/h1>\n?/, '');
  const neighbours = findNeighbour(source.pageTree, page.url);
  return (
    <DocumentAnchors
      headings={doc.headings.filter(
        (heading) => heading.depth === 2 || heading.depth === 3,
      )}
    >
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
          <h1
            className="reader-title"
            id={first?.id}
            data-pagefind-meta="title"
          >
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
              <DocumentOutline />
            </Disclosure>
          </div>
          <div className="prose" dangerouslySetInnerHTML={{ __html: html }} />
          <nav
            aria-label="相邻文档"
            className="mt-10 flex flex-wrap justify-between gap-3"
            data-pagefind-ignore
          >
            {neighbours.previous && (
              <Button asChild variant="outline">
                <Link href={neighbours.previous.url}>
                  上一篇：{neighbours.previous.name}
                </Link>
              </Button>
            )}
            {neighbours.next && (
              <Button asChild variant="outline">
                <Link href={neighbours.next.url}>
                  下一篇：{neighbours.next.name}
                </Link>
              </Button>
            )}
          </nav>
        </article>
        <aside className="toc" aria-label="本页目录" data-pagefind-ignore>
          <h2>本页目录</h2>
          <DocumentOutline />
        </aside>
      </div>
    </DocumentAnchors>
  );
}
