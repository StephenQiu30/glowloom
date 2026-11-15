import { posix } from 'node:path';
import type { ComponentProps } from 'react';
import type { InferPageType } from 'fumadocs-core/source';
import defaultMdxComponents from 'fumadocs-ui/mdx';
import { source } from '@/lib/source';
import { gitConfig } from '@/lib/shared';
import documentIds from '@/content/document-ids.json';

// Existing Markdown uses bare relative filenames as well as ./ and ../ links.
// Resolve them through Fumadocs without rewriting the authored content.
export function createContentLink(page: InferPageType<typeof source>) {
  return function ContentLink({ href, ...props }: ComponentProps<'a'>) {
    const Link = defaultMdxComponents.a;
    if (!href || /^(?:[a-z][a-z\d+.-]*:|\/|#)/i.test(href)) {
      return <Link href={href} {...props} />;
    }

    const relativeHref = href.startsWith('.') ? href : `./${href}`;
    const resolvedHref = source.resolveHref(relativeHref, page);
    if (resolvedHref !== relativeHref) {
      return <Link href={resolvedHref} {...props} />;
    }

    const [file, fragment] = href.split('#', 2);
    const path = posix.normalize(
      posix.join(posix.dirname(page.path), decodeURI(file)),
    );
    const csv = documentIds.find(
      (entry) => entry.path === path && entry.path.endsWith('.csv'),
    );
    const suffix = fragment ? `#${fragment}` : '';
    if (csv) {
      return <Link href={`/raw/${csv.id}.csv${suffix}`} {...props} />;
    }

    const repositoryPath = posix.normalize(`workspace/content/${path}`);
    if (
      !repositoryPath.startsWith('workspace/content/') &&
      !repositoryPath.startsWith('../')
    ) {
      const url = `https://github.com/${gitConfig.user}/${gitConfig.repo}/blob/${gitConfig.branch}/${repositoryPath}${suffix}`;
      return <Link href={url} {...props} />;
    }
    return <Link href={href} {...props} />;
  };
}
