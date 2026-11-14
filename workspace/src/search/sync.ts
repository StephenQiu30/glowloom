import { readFile } from 'node:fs/promises';
import { resolve } from 'node:path';
import { Meilisearch } from 'meilisearch';
import { sync } from 'fumadocs-core/search/meilisearch';
import { z } from 'zod';
import { verifyRelease } from '../build/release';
import { searchIndexName } from '../content/search';

const recordsContract = z.array(
  z.object({
    title: z.string(),
    url: z.string(),
    description: z.string().optional(),
    breadcrumbs: z.array(z.string()).optional(),
    tag: z.union([z.string(), z.array(z.string())]).optional(),
    locale: z.string().optional(),
    structured: z.object({
      headings: z.array(z.object({ id: z.string(), content: z.string() })),
      contents: z.array(
        z.object({
          content: z.string(),
          heading: z.string().optional(),
          table: z.string().optional(),
        }),
      ),
    }),
  }),
);

/** Explicit publication step; neither ordinary builds nor website startup write the index. */
export async function synchronizeSearch(workspaceRoot: string) {
  const host = process.env.GLOWLOOM_WORKSPACE_MEILISEARCH_HOST;
  const apiKey = process.env.GLOWLOOM_WORKSPACE_MEILISEARCH_ADMIN_KEY;
  if (!host || !apiKey) {
    throw new Error(
      'Configure Meilisearch host and the publication-only admin key',
    );
  }
  const { releaseId } = z
    .object({ releaseId: z.string().regex(/^[\w-]+$/) })
    .parse(
      JSON.parse(
        await readFile(
          resolve(workspaceRoot, 'dist/online/current.json'),
          'utf8',
        ),
      ),
    );
  const root = resolve(workspaceRoot, 'dist/online', releaseId);
  const release = await verifyRelease(root, 'online');
  const indexName = searchIndexName(release.contentDigest);
  const documents = recordsContract.parse(
    JSON.parse(await readFile(resolve(root, 'search-documents.json'), 'utf8')),
  );
  const client = new Meilisearch({ host, apiKey, timeout: 30000 });
  await sync(client, {
    indexName,
    documents: documents.map((document) => ({
      ...document,
      structured: {
        ...document.structured,
        contents: document.structured.contents.map((item) => ({
          ...item,
          heading: item.heading,
        })),
      },
    })),
  });
  return {
    indexName,
    documents: documents.length,
    contentDigest: release.contentDigest,
  };
}
