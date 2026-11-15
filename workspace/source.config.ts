import { defineConfig } from 'fumadocs-mdx/config';
import { remarkMdxMermaid } from 'fumadocs-core/mdx-plugins';
import rehypeRaw from 'rehype-raw';

export default defineConfig({
  mdxOptions: {
    remarkPlugins: [remarkMdxMermaid],
    remarkImageOptions: false,
    rehypeCodeOptions: {
      themes: { light: 'github-light', dark: 'github-dark' },
      fallbackLanguage: 'text',
    },
    rehypePlugins: [
      [
        rehypeRaw,
        {
          passThrough: [
            'mdxJsxFlowElement',
            'mdxJsxTextElement',
            'mdxFlowExpression',
            'mdxTextExpression',
            'mdxjsEsm',
          ],
        },
      ],
    ],
  },
});
