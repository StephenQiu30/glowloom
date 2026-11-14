import Link from 'next/link';
import { ArrowUpRight, FileText } from 'lucide-react';
import {
  Item,
  ItemActions,
  ItemContent,
  ItemDescription,
  ItemGroup,
  ItemMedia,
  ItemTitle,
} from '@/components/ui/item';

interface DocumentSummary {
  readonly id: string;
  readonly title: string;
  readonly description?: string;
}

/** Renders navigable knowledge entries with shared shadcn item semantics. */
export function DocumentList({
  documents,
}: {
  readonly documents: DocumentSummary[];
}) {
  return (
    <ItemGroup>
      {documents.map((document) => (
        <div key={document.id} role="listitem">
          <Item asChild variant="outline">
            <Link href={`/read/${document.id}/`} title={document.title}>
              <ItemMedia variant="icon">
                <FileText aria-hidden="true" />
              </ItemMedia>
              <ItemContent className="min-w-0">
                <ItemTitle>{document.title}</ItemTitle>
                {document.description && (
                  <ItemDescription>{document.description}</ItemDescription>
                )}
              </ItemContent>
              <ItemActions>
                <ArrowUpRight aria-hidden="true" className="size-4" />
              </ItemActions>
            </Link>
          </Item>
        </div>
      ))}
    </ItemGroup>
  );
}
