import { expect, it } from 'vitest';
import { toDocuments } from 'fumadocs-core/search/meilisearch';
import { parseMarkdown } from '../../src/content/parse';
import { createDocumentSource } from '../../src/content/source';

it('exports stable Fumadocs records including Chinese paragraphs, table IDs and heading targets', async () => {
  const source =
    '# Workspace\n\n## 方案\n\n中文知识。\n\n| 编号 | 验收 |\n| --- | --- |\n| FR-WS-01 | 统一入口 |\n';
  const parsed = await parseMarkdown(source);
  const loader = createDocumentSource(
    {
      documents: [
        {
          ...parsed,
          id: 'design-43',
          path: 'content/design/43.md',
          category: 'design',
          visibility: 'internal',
          source,
          revision: 'frozen',
          sourcePath: 'workspace/content/design/43.md',
        },
      ],
    },
    { design: '设计方案' },
  );
  const records = await toDocuments(loader, {
    tag: (page) => page.data.document.category,
  });
  expect(records).toHaveLength(1);
  expect(records[0].url).toBe('/read/design-43/');
  expect(records[0].tag).toBe('design');
  expect(records[0].structured.headings.map((h) => h.id)).toEqual(
    parsed.headings.map((h) => h.id),
  );
  expect(
    records[0].structured.contents.some((c) => c.content.includes('FR-WS-01')),
  ).toBe(true);
  expect(
    records[0].structured.contents.some((c) => c.content.includes('中文知识')),
  ).toBe(true);
});
