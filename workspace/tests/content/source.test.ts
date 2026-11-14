import { expect, it } from 'vitest';
import { flattenTree, findNeighbour } from 'fumadocs-core/page-tree';
import type { Document } from '../../src/content/catalog';
import { createDocumentSource } from '../../src/content/source';

function document(id: string, category: string): Document {
  return {
    id,
    category,
    path: `content/${category}/旧文件名.md`,
    visibility: 'internal',
    title: `文档 ${id}`,
    source: '# 原文',
    revision: 'original',
    sourcePath: 'workspace/content/旧文件名.md',
    html: '',
    headings: [],
    blocks: [],
    links: [],
    diagrams: [],
    aliases: ['docs/旧文件名.md'],
  };
}

it('uses catalog identities and canonical links instead of deriving routes from filenames', () => {
  const doc = document('design-43', 'design');
  const source = createDocumentSource(
    { documents: [doc] },
    { design: '设计方案' },
  );
  expect(source.getPage(['design-43'])?.url).toBe('/read/design-43/');
  expect(
    source.getPageByHref('/read/design-43/#方案')?.page.data.document,
  ).toBe(doc);
  expect(source.generateParams('id')).toEqual([{ id: ['design-43'] }]);
  expect(doc.path).toBe('content/design/旧文件名.md');
  expect(doc.aliases).toEqual(['docs/旧文件名.md']);
});

it('includes unregistered categories and keeps framework neighbour navigation within that source', () => {
  const documents = [
    document('design-43', 'design'),
    document('extra', 'COS配置说明'),
  ];
  const source = createDocumentSource({ documents }, { design: '设计方案' });
  const pages = flattenTree(source.pageTree.children);
  expect(pages.map((page) => page.url)).toEqual([
    '/read/design-43/',
    '/read/extra/',
  ]);
  expect(source.pageTree.children.map((node) => node.name)).toEqual([
    '设计方案',
    'COS配置说明',
  ]);
  expect(findNeighbour(source.pageTree, '/read/design-43/').next?.url).toBe(
    '/read/extra/',
  );
});

it('never adds documents outside the already filtered input, including an empty public source', () => {
  const source = createDocumentSource(
    { documents: [] },
    { design: '设计方案' },
  );
  expect(source.getPages()).toEqual([]);
  expect(source.pageTree.children).toEqual([]);
  expect(source.getPage(['design-43'])).toBeUndefined();
});
