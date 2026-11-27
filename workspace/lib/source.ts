import { llms, loader } from 'fumadocs-core/source';
import { lucideIconsPlugin } from 'fumadocs-core/source/lucide-icons';
import { docsRoute } from './shared';
import { defineDocs } from 'fumadocs-mdx/macro';
import { metaSchema, pageSchema } from 'fumadocs-core/source/schema';

export function numberedDocumentTitle(filePath: string) {
  return /(?:^|\/)content\/[^/]+\/(\d{2}-.+)\.mdx?$/.exec(filePath)?.[1];
}

const docs = defineDocs({
  dir: '.',
  docs: {
    files: [
      'ARCHITECTURE.md',
      'PROJECT.md',
      'content/**/*.md',
      'content/**/*.mdx',
    ],
    schema: ({ source, path }) =>
      pageSchema.extend({
        title: pageSchema.shape.title
          .default(
            source.match(/^#\s+(.+)$/m)?.[1] ??
              path
                .split('/')
                .at(-1)!
                .replace(/\.mdx?$/, ''),
          )
          .transform((title) => numberedDocumentTitle(path) ?? title),
        sourceHeading: pageSchema.shape.title
          .optional()
          .transform(() => source.match(/^#\s+(.+)$/m)?.[1]),
        archived: pageSchema.shape.full.default(false),
      }),
    postprocess: {
      includeProcessedMarkdown: true,
    },
  },
  meta: {
    files: ['meta.json', 'content/**/meta.json'],
    schema: metaSchema,
  },
});

// See https://fumadocs.dev/docs/headless/source-api for more info
export const source = loader({
  baseUrl: docsRoute,
  source: docs.toFumadocsSource(),
  slugs(file, next) {
    if (file.path === 'ARCHITECTURE.md') return ['architecture'];
    if (file.path === 'PROJECT.md') return ['project'];
    return next().slice(1);
  },
  plugins: [lucideIconsPlugin()],
});

export const searchableSource: typeof source = {
  ...source,
  getPages(language?: string) {
    return source.getPages(language).filter((page) => !page.data.archived);
  },
};

export const docsLlms = llms(searchableSource, {
  renderPage: async (page) => `# ${page.data.title} (${page.url})

${await page.data.getText('processed')}`,
});
