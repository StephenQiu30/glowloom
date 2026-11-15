import * as fs from 'node:fs/promises';
import { type DocumentRecord, sync } from 'fumadocs-core/search/meilisearch';
import { Meilisearch } from 'meilisearch';

// Next.js writes it under `route-cache` when built with an adapter, like on Vercel
const [file] = await Array.fromAsync(
  fs.glob('.next/server/{app,route-cache/*/*/$}/static.json.body'),
);
const records = JSON.parse(
  await fs.readFile(file, 'utf-8'),
) as DocumentRecord[];

const client = new Meilisearch({
  host: process.env.NEXT_PUBLIC_MEILISEARCH_HOST!,
  apiKey: process.env.MEILISEARCH_ADMIN_KEY!,
});

await sync(client, {
  indexName: 'docs',
  documents: records,
});

console.log(`search updated: ${records.length} records`);
