import { randomBytes } from 'node:crypto';
import { afterEach, expect, it, vi } from 'vitest';
import { Meilisearch } from 'meilisearch';
import { sync } from 'fumadocs-core/search/meilisearch';
import { searchKnowledge } from '../../src/server/search';
import { searchIndexName } from '../../src/content/search';

const host = process.env.WORKSPACE_MEILISEARCH_TEST_HOST;
const adminKey = process.env.WORKSPACE_MEILISEARCH_TEST_ADMIN_KEY;
afterEach(() => vi.unstubAllEnvs());

it('rejects oversized queries and unconfigured search without exposing engine details', async () => {
  vi.stubEnv('GLOWLOOM_WORKSPACE_MEILISEARCH_HOST', '');
  vi.stubEnv('GLOWLOOM_WORKSPACE_MEILISEARCH_SEARCH_KEY', '');
  await expect(
    searchKnowledge('a'.repeat(64), 'x'.repeat(201), ''),
  ).rejects.toMatchObject({ status: 400 });
  await expect(
    searchKnowledge('a'.repeat(64), '知识', ''),
  ).rejects.toMatchObject({ status: 503, message: 'search_unavailable' });
  await expect(searchKnowledge('a'.repeat(64), ' ', '')).resolves.toEqual([]);
  expect(() => searchIndexName('../other-index')).toThrow(
    'Invalid search content digest',
  );
});

it.skipIf(!host || !adminKey)(
  'syncs and searches real version-scoped Meilisearch with a read-only key',
  async () => {
    const digest = randomBytes(32).toString('hex');
    const indexName = searchIndexName(digest);
    const admin = new Meilisearch({ host: host!, apiKey: adminKey! });
    const key = await admin.createKey({
      actions: ['search'],
      indexes: [indexName],
      expiresAt: null,
    });
    vi.stubEnv('GLOWLOOM_WORKSPACE_MEILISEARCH_HOST', host!);
    vi.stubEnv('GLOWLOOM_WORKSPACE_MEILISEARCH_SEARCH_KEY', key.key);
    try {
      await sync(admin, {
        indexName,
        documents: [
          {
            title: 'Workspace 设计',
            url: '/read/design-43/',
            tag: '设计方案',
            structured: {
              headings: [{ id: '方案', content: '方案' }],
              contents: [
                { heading: '方案', content: 'FR-WS-01 中文知识统一入口' },
              ],
            },
          },
        ],
      });
      for (const query of ['FR-WS-01', '中文知识', 'Workspace']) {
        const result = await searchKnowledge(digest, query, '设计方案');
        expect(
          result.some((item) => item.url.startsWith('/read/design-43/')),
        ).toBe(true);
      }
      expect(await searchKnowledge(digest, '中文知识', '不存在分类')).toEqual(
        [],
      );
      expect(
        await searchKnowledge(digest, '中文知识', '" OR tag = "设计方案'),
      ).toEqual([]);
      const results = await searchKnowledge(digest, '中文知识', '');
      expect(results.some((item) => item.url === '/read/design-43/#方案')).toBe(
        true,
      );
      const reader = new Meilisearch({ host: host!, apiKey: key.key });
      await expect(
        reader
          .index(indexName)
          .addDocuments([{ id: 999, content: 'unauthorized' }]),
      ).rejects.toMatchObject({ response: { status: 403 } });
      await expect(
        searchKnowledge(randomBytes(32).toString('hex'), '知识', ''),
      ).rejects.toMatchObject({ status: 503 });
      vi.stubEnv('GLOWLOOM_WORKSPACE_MEILISEARCH_HOST', 'http://127.0.0.1:1');
      await expect(searchKnowledge(digest, '知识', '')).rejects.toMatchObject({
        message: 'search_unavailable',
      });
    } finally {
      await admin.deleteKey(key.uid);
      await admin.deleteIndex(indexName).waitTask({ timeout: 30000 });
    }
  },
  45000,
);
