'use client';

import { AnchorProvider, TOCItem, useTOC } from 'fumadocs-core/toc';
import type { ReactNode } from 'react';
import { Button } from '@/components/ui/button';
import type { Heading } from '@/content/types';

/** Only heading metadata crosses the boundary; the article remains server-rendered. */
export function DocumentAnchors({
  headings,
  children,
}: {
  readonly headings: Heading[];
  readonly children: ReactNode;
}) {
  return (
    <AnchorProvider
      single
      toc={headings.map((heading) => ({
        title: heading.text,
        depth: heading.depth,
        url: `#${heading.id}`,
      }))}
    >
      {children}
    </AnchorProvider>
  );
}

/** Fumadocs owns anchor observation and selection; shadcn owns presentation. */
export function DocumentOutline() {
  const toc = useTOC();
  return (
    <nav aria-label="本页目录" className="flex flex-col items-stretch gap-1">
      {toc.get().map(({ original: item }) => (
        <Button
          key={item.url}
          variant="ghost"
          size="sm"
          asChild
          className="h-auto justify-start whitespace-normal text-left font-normal text-muted-foreground data-[active=true]:bg-accent data-[active=true]:text-accent-foreground"
        >
          <TOCItem href={item.url} data-depth={item.depth} autoScroll={false}>
            {item.title}
          </TOCItem>
        </Button>
      ))}
    </nav>
  );
}
