import {
  loader,
  type VirtualFile,
  type PageData,
  type MetaData,
} from 'fumadocs-core/source';
import type { Snapshot, Document } from './catalog';
import {
  structure,
  type StructuredData,
} from 'fumadocs-core/mdx-plugins/remark-structure';
import { documentHref } from './identity';

interface WorkspacePage extends PageData {
  readonly document: Document;
  readonly structuredData: () => StructuredData;
}

/** Catalog identities stay authoritative; the framework owns the page tree and lookups. */
export function createDocumentSource(
  snapshot: Pick<Snapshot, 'documents'>,
  categories: Record<string, string>,
) {
  const groups = [
    ...new Set([
      ...Object.keys(categories),
      ...snapshot.documents.map((doc) => doc.category),
    ]),
  ].filter((category) =>
    snapshot.documents.some((doc) => doc.category === category),
  );
  const files: VirtualFile<{ pageData: WorkspacePage; metaData: MetaData }>[] =
    [
      {
        type: 'meta',
        path: 'meta.json',
        data: {
          pages: groups,
        },
      },
    ];
  for (const category of groups) {
    files.push({
      type: 'meta',
      path: `${category}/meta.json`,
      data: {
        title: categories[category] ?? category,
        pages: snapshot.documents
          .filter((doc) => doc.category === category)
          .map((doc) => doc.id),
      },
    });
  }
  for (const document of snapshot.documents) {
    files.push({
      type: 'page',
      path: `${document.category}/${document.id}.md`,
      slugs: [document.id],
      data: {
        title: document.title,
        document,
        structuredData: () => structure(document.source),
      },
    });
  }
  return loader({
    baseUrl: '/read',
    plugins: [
      {
        name: 'workspace-canonical-urls',
        config(config) {
          return { ...config, url: (slugs) => documentHref(slugs[0]) };
        },
      },
    ],
    source: { files },
  });
}
