import { createMDX } from 'fumadocs-mdx/next';
import { readFileSync } from 'node:fs';

const documents = JSON.parse(
  readFileSync(new URL('./content/document-ids.json', import.meta.url), 'utf8'),
);
const external = JSON.parse(
  readFileSync(
    new URL('./lib/external-documents.json', import.meta.url),
    'utf8',
  ),
);
const repo = 'https://github.com/StephenQiu30/glowloom/blob/main/';
const withMDX = createMDX();

export default withMDX({
  reactStrictMode: true,
  experimental: { cpus: 2 },
  async redirects() {
    const redirects = [];
    for (const entry of documents) {
      const destination = entry.path.endsWith('.csv')
        ? `/raw/${entry.id}.csv`
        : entry.path === '00-文档索引.md'
          ? '/read'
          : `/read/${entry.id}`;
      for (const alias of entry.aliases) {
        redirects.push({
          source: encodeURI(`/${alias}`),
          destination,
          permanent: true,
        });
      }
      if (entry.legacyPath) {
        redirects.push(
          {
            source: encodeURI(`/${entry.legacyPath}`),
            destination,
            permanent: true,
          },
          {
            source: encodeURI(`/read/${entry.legacyPath}`),
            destination,
            permanent: true,
          },
        );
      }
      if (entry.path.endsWith('.csv') || entry.path === '00-文档索引.md') {
        redirects.push({
          source: `/read/${entry.id}`,
          destination,
          permanent: true,
        });
      }
      if (!entry.path.endsWith('.csv')) {
        redirects.push({
          source: `/raw/${entry.id}.md`,
          destination:
            entry.path === '00-文档索引.md'
              ? '/read.md'
              : `/read/${entry.id}.md`,
          permanent: true,
        });
      }
    }
    for (const entry of external) {
      const path = entry.path.startsWith('../')
        ? entry.path.slice(3)
        : 'workspace/' + entry.path;
      redirects.push({
        source: `/read/${entry.id}`,
        destination: repo + path,
        permanent: true,
      });
      redirects.push({
        source: `/raw/${entry.id}.md`,
        destination: `https://raw.githubusercontent.com/StephenQiu30/glowloom/main/${path}`,
        permanent: true,
      });
    }
    redirects.push({
      source: '/backend/:path*',
      destination: repo + 'backend/:path*',
      permanent: true,
    });
    for (const name of [
      'AGENTS.md',
      'PROJECT.md',
      'DESIGN.md',
      'README.md',
      'BACKLOG.md',
    ]) {
      redirects.push({
        source: '/' + name,
        destination: repo + name,
        permanent: true,
      });
    }
    return redirects;
  },
  async rewrites() {
    return [
      { source: '/read.md', destination: '/llms.mdx/read.md' },
      { source: '/read/:path*.md', destination: '/llms.mdx/read/:path*.md' },
      { source: '/read/:path*.txt', destination: '/llms.mdx/read/:path*.txt' },
    ];
  },
});
