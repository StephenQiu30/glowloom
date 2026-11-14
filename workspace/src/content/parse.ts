import { digest } from './identity';
import { createMarkdownRenderer } from 'fumadocs-core/content/md';
import { remarkHeading } from 'fumadocs-core/mdx-plugins/remark-heading';
import { renderToStaticMarkup } from 'react-dom/server';
import { toString } from 'mdast-util-to-string';
import type { Root } from 'mdast';
import type { Transformer } from 'unified';
import remarkGfm from 'remark-gfm';
import rehypeRaw from 'rehype-raw';
import rehypeSanitize, { defaultSchema } from 'rehype-sanitize';
import { visit } from 'unist-util-visit';
import type { ParsedDocument } from './types';

export async function parseMarkdown(
  source: string,
  resolveHref: (href: string) => string = (href) => href,
): Promise<ParsedDocument> {
  const result: ParsedDocument = {
    title: '',
    html: '',
    headings: [],
    blocks: [],
    links: [],
    diagrams: [],
  };
  const occurrences = new Map<string, number>();
  let heading = '';
  function workspaceMetadata(): Transformer<Root> {
    return function collect(tree, file) {
      result.headings = (file.data.toc ?? []).map((item) => ({
        id: item.url.slice(1),
        text: String(item.title),
        depth: item.depth,
      }));
      result.title = result.headings[0]?.text ?? '';
      visit(tree, (node) => {
        if (node.type === 'heading') {
          heading = String(node.data?.hProperties?.id ?? '');
        }
        if (node.type === 'paragraph') {
          const text = toString(node).trim();
          if (!text) {
            return;
          }
          const fingerprint = digest(text).slice(0, 14);
          const occurrence = occurrences.get(fingerprint) ?? 0;
          occurrences.set(fingerprint, occurrence + 1);
          const id = `block-${fingerprint}-${occurrence}`;
          node.data = {
            ...node.data,
            hProperties: { id, 'data-block-id': id },
          };
          result.blocks.push({
            id,
            text,
            heading,
            start: node.position?.start.offset ?? 0,
            end: node.position?.end.offset ?? 0,
          });
        }
        if (
          node.type === 'link' ||
          node.type === 'image' ||
          node.type === 'definition'
        ) {
          result.links.push(node.url);
          node.url = resolveHref(node.url);
        }
        if (node.type === 'code' && node.lang === 'mermaid') {
          const id = digest(node.value).slice(0, 20);
          result.diagrams.push({ id, source: node.value });
          Object.assign(node, {
            type: 'html',
            value: `<figure data-diagram="${id}"></figure>`,
          });
        }
      });
    };
  }
  // Fumadocs processes ordinary Markdown: authored JS/JSX is never compiled.
  // Only workspace-specific links, audio block identities and diagram references extend it.
  const renderer = createMarkdownRenderer({
    remarkPlugins: [
      remarkGfm,
      [remarkHeading, { customId: false }],
      workspaceMetadata,
    ],
    remarkRehypeOptions: { allowDangerousHtml: true },
    rehypePlugins: [
      rehypeRaw,
      [
        rehypeSanitize,
        {
          ...defaultSchema,
          clobberPrefix: '',
          tagNames: [...(defaultSchema.tagNames ?? []), 'figure'],
          attributes: {
            ...defaultSchema.attributes,
            '*': [...(defaultSchema.attributes?.['*'] ?? []), 'id'],
            p: ['id', 'dataBlockId'],
            figure: ['dataDiagram'],
          },
        },
      ],
    ],
  });
  result.html = renderToStaticMarkup(
    await renderer.MarkdownServer({ children: source }),
  );
  return result;
}
