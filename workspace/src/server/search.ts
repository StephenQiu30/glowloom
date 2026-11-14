import { Meilisearch } from 'meilisearch';
import type { SortedResult } from 'fumadocs-core/search';
import type { MeilisearchDocument } from 'fumadocs-core/search/meilisearch';
import { AccessError } from './auth/session';
import { searchIndexName } from '../content/search';

/** Only the website server receives the index-scoped search credential. */
export async function searchKnowledge(
  contentDigest: string,
  query: string,
  category: string,
): Promise<SortedResult[]> {
  if (query.length > 200 || category.length > 80) {
    throw new AccessError('invalid_search', 400);
  }
  if (!query.trim()) {
    return [];
  }
  const host = process.env.GLOWLOOM_WORKSPACE_MEILISEARCH_HOST;
  const apiKey = process.env.GLOWLOOM_WORKSPACE_MEILISEARCH_SEARCH_KEY;
  if (!host || !apiKey) {
    throw new AccessError('search_unavailable', 503);
  }
  try {
    const client = new Meilisearch({ host, apiKey, timeout: 5000 });
    const { hits } = await client
      .index(searchIndexName(contentDigest))
      .search<MeilisearchDocument>(query, {
        limit: 30,
        filter: category ? [`tag = ${JSON.stringify(category)}`] : [],
      });
    const pages = new Set<string>();
    const results: SortedResult[] = [];
    for (const hit of hits) {
      if (!pages.has(hit.url)) {
        pages.add(hit.url);
        results.push({
          id: hit.url,
          type: 'page',
          content: hit.title,
          url: hit.url,
          breadcrumbs: hit.breadcrumbs,
        });
      }
      if (hit.type !== 'page') {
        results.push({
          id: String(hit.id),
          type: hit.type,
          content: hit.content,
          url: hit.heading ? `${hit.url}#${hit.heading}` : hit.url,
          table: hit.table,
        });
      }
    }
    return results;
  } catch {
    // Engine details and credentials never cross the public API boundary.
    throw new AccessError('search_unavailable', 503);
  }
}
