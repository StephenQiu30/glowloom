import { llms, loader } from 'fumadocs-core/source';
import { lucideIconsPlugin } from 'fumadocs-core/source/lucide-icons';
import { docsRoute } from './shared';
import { defineDocs } from 'fumadocs-mdx/macro';
import documentIds from '@/content/document-ids.json';
import { metaSchema, pageSchema } from 'fumadocs-core/source/schema';

const docs = defineDocs({
  dir: 'content',
  docs: {
    files: ['**/*.md'],
    schema: ({ source, path }) =>
      pageSchema.extend({
        title: pageSchema.shape.title.default(
          source.match(/^#\s+(.+)$/m)?.[1] ??
            path.split('/').at(-1)!.replace(/\.md$/, ''),
        ),
      }),
    postprocess: {
      includeProcessedMarkdown: true,
    },
  },
  meta: {
    files: ['**/meta.json'],
    schema: metaSchema,
  },
});

// See https://fumadocs.dev/docs/headless/source-api for more info
export const source = loader({
  baseUrl: docsRoute,
  source: docs.toFumadocsSource(),
  slugs(file) {
    if (file.path === '00-文档索引.md') {
      return [];
    }
    const entry = documentIds.find((item) => item.path === file.path);
    return entry ? [entry.id] : undefined;
  },
  plugins: [lucideIconsPlugin()],
});

export const docsLlms = llms(source, {
  renderPage: (page) => page.data.getText('raw'),
});
