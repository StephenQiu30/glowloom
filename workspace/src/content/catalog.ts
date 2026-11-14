import { loadReferences, referencePaths } from './references';
import { readFile, realpath, stat } from 'node:fs/promises';
import { execFileSync } from 'node:child_process';
import { dirname, relative, resolve, sep } from 'node:path';
import { unified } from 'unified';
import remarkParse from 'remark-parse';
import { visit } from 'unist-util-visit';
import { toString } from 'mdast-util-to-string';
import type { ParsedDocument } from './types';
import { parseMarkdown } from './parse';
import { digest, documentHref } from './identity';

export interface CatalogEntry {
  id: string;
  path: string;
  visibility: 'public' | 'internal';
  aliases?: string[];
}
export type Document = ParsedDocument &
  CatalogEntry & {
    source: string;
    revision: string;
    category: string;
    sourcePath: string;
  };
export interface Snapshot {
  documents: Document[];
  contentDigest: string;
  repoCommit: string;
  scope: 'public' | 'internal';
  workspaceRoot: string;
  preview: boolean;
}
const rootFiles = referencePaths;

function isExternal(href: string) {
  return /^(?:[a-z][a-z\d+.-]*:|\/\/)/i.test(href);
}
export function rewriteMarkdownLinks(
  source: string,
  rewrite: (url: string) => string | null,
  missing: (label: string, href: string) => string = () => '受限内容',
): string {
  const tree = unified().use(remarkParse).parse(source);
  const edits: { start: number; end: number; value: string }[] = [];
  const definitions = new Map<string, string>();
  visit(tree, 'definition', (node) => {
    definitions.set(node.identifier.toLowerCase(), node.url);
  });
  visit(tree, (node) => {
    if (node.type !== 'linkReference' && node.type !== 'imageReference') {
      return;
    }
    const url = definitions.get(node.identifier.toLowerCase());
    if (
      url &&
      rewrite(url) === null &&
      node.position?.start.offset !== undefined &&
      node.position.end.offset !== undefined
    ) {
      edits.push({
        start: node.position.start.offset,
        end: node.position.end.offset,
        value: missing(toString(node), url),
      });
    }
  });
  visit(tree, (node) => {
    if (
      node.type !== 'link' &&
      node.type !== 'image' &&
      node.type !== 'definition'
    ) {
      return;
    }
    const url = rewrite(node.url);
    const start = node.position?.start.offset;
    const end = node.position?.end.offset;
    if (start === undefined || end === undefined || url === node.url) {
      return;
    }
    const raw = source.slice(start, end);
    const urlStart =
      node.type === 'definition'
        ? raw.indexOf(':') + 1
        : raw.lastIndexOf('](') + 2;
    edits.push({
      start,
      end,
      value:
        url === null
          ? missing(toString(node), node.url)
          : raw.slice(0, urlStart) + raw.slice(urlStart).replace(node.url, url),
    });
  });
  for (const edit of edits.sort((a, b) => b.start - a.start)) {
    source = source.slice(0, edit.start) + edit.value + source.slice(edit.end);
  }
  return source;
}

function workspaceReference(workspaceRoot: string, target: string) {
  const logicalRoot = resolve(dirname(workspaceRoot), 'workspace');
  return target.startsWith(logicalRoot + '/')
    ? resolve(workspaceRoot, relative(logicalRoot, target))
    : target;
}

export async function loadSnapshot(options: {
  workspaceRoot: string;
  scope: 'public' | 'internal';
  referenceMode?: 'live' | 'snapshot';
}): Promise<Snapshot> {
  const workspaceRoot = await realpath(options.workspaceRoot);
  const repoRoot = dirname(workspaceRoot);
  const references =
    options.referenceMode === 'live'
      ? null
      : await loadReferences(workspaceRoot);
  const catalog = JSON.parse(
    await readFile(resolve(workspaceRoot, 'catalog.json'), 'utf8'),
  ) as CatalogEntry[];
  const ids = new Set<string>();
  const paths = new Map<string, CatalogEntry>();
  for (const entry of catalog) {
    if (!entry.id || !/^[\w-]+$/.test(entry.id) || ids.has(entry.id)) {
      throw new Error(`Duplicate or invalid document identity: ${entry.id}`);
    }
    if (!['public', 'internal'].includes(entry.visibility)) {
      throw new Error('Invalid visibility');
    }
    ids.add(entry.id);
    const path = resolve(workspaceRoot, entry.path);
    const repoPath = entry.path.startsWith('../')
      ? entry.path.slice(3)
      : 'workspace/' + entry.path;
    if (!(
      path.startsWith(workspaceRoot + '/content/') ||
      path.startsWith(workspaceRoot + '/skills/') ||
      path.startsWith(workspaceRoot + '/workflows/') ||
      rootFiles.has(repoPath)
    )) {
      throw new Error(`Disallowed path: ${entry.path}`);
    }
    if (paths.has(path)) {
      throw new Error(`Duplicate path: ${entry.path}`);
    }
    paths.set(path, entry);
  }
  const visible = catalog.filter(
    (e) => options.scope === 'internal' || e.visibility === 'public',
  );
  function canRead(entry: CatalogEntry) {
    return options.scope === 'internal' || entry.visibility === 'public';
  }
  const documents: Document[] = [];
  for (const entry of visible) {
    const path = resolve(workspaceRoot, entry.path);
    const reference = entry.path.startsWith('../')
      ? references?.entries[entry.id]
      : undefined;
    if (reference && reference.path !== entry.path.slice(3)) {
      throw new Error('Knowledge reference identity mismatch');
    }
    if (!reference && (await realpath(path)) !== path) {
      throw new Error(`Symlink path escape: ${entry.path}`);
    }
    let source = reference?.source ?? (await readFile(path, 'utf8'));
    function targetEntry(href: string) {
      if (isExternal(href) || href.startsWith('#')) {
        return undefined;
      }
      return paths.get(
        workspaceReference(
          workspaceRoot,
          resolve(dirname(path), decodeURIComponent(href.split(/[?#]/)[0])),
        ),
      );
    }
    if (options.scope === 'public') {
      const publicTree = unified().use(remarkParse).parse(source);
      visit(publicTree, 'html', (node) => {
        if (
          !/^\s*(?:<a\s+id=(?:"[^"]+"|'[^']+')\s*>\s*<\/a>\s*)+$/.test(
            node.value,
          )
        ) {
          throw new Error('public HTML requires explicit review: ' + entry.id);
        }
      });
      source = rewriteMarkdownLinks(source, (href) => {
        const target = targetEntry(href);
        return target && !canRead(target) ? null : href;
      });
    }
    const parsed = await parseMarkdown(source, (href) => {
      if (isExternal(href) || href.startsWith('#')) {
        return href;
      }
      const target = targetEntry(href);
      const anchor = decodeURIComponent(href.split('#').slice(1).join('#'));
      if (target) {
        return canRead(target)
          ? documentHref(target.id, anchor)
          : '#restricted';
      }
      // Engineering references are explicit references, never arbitrary source downloads.
      const engineeringPath = relative(
        repoRoot,
        workspaceReference(
          workspaceRoot,
          resolve(dirname(path), decodeURIComponent(href.split(/[?#]/)[0])),
        ),
      )
        .split(sep)
        .join('/');
      return `/engineering/?path=${encodeURIComponent(engineeringPath)}`;
    });
    if (entry.path.endsWith('.csv')) {
      const csv = await parseMarkdown(
        `# ${entry.path.split('/').at(-1)}\n\n\`\`\`csv\n${source}\n\`\`\``,
      );
      Object.assign(parsed, csv);
    }
    documents.push({
      ...entry,
      ...parsed,
      source,
      revision: digest(source),
      category: entry.path.startsWith('content/')
        ? entry.path.split('/')[1].replace(/\.md$/, '')
        : entry.path.startsWith('skills/') ||
            entry.path.startsWith('workflows/')
          ? entry.path.split('/')[0]
          : '工程规范',
      sourcePath: entry.path.startsWith('../')
        ? entry.path.slice(3)
        : 'workspace/' + entry.path,
    });
  }
  let repoCommit = references?.repoCommit ?? 'unversioned';
  let preview = true;
  try {
    repoCommit = execFileSync('git', ['rev-parse', 'HEAD'], {
      cwd: repoRoot,
      stdio: ['ignore', 'pipe', 'ignore'],
    })
      .toString()
      .trim();
    preview = !!execFileSync(
      'git',
      ['status', '--porcelain', '--untracked-files=normal'],
      { cwd: repoRoot, stdio: ['ignore', 'pipe', 'ignore'] },
    )
      .toString()
      .trim();
  } catch {
    /* Filesystem fixtures and exported source trees have no Git metadata. */
  }
  const snapshot = {
    documents,
    contentDigest: digest(
      JSON.stringify(
        documents.map((d) => [
          d.id,
          d.path,
          d.category,
          d.revision,
          d.visibility,
          d.aliases,
        ]),
      ),
    ),
    repoCommit,
    scope: options.scope,
    preview,
  } as Snapshot;
  Object.defineProperty(snapshot, 'workspaceRoot', {
    value: workspaceRoot,
    enumerable: false,
  });
  return snapshot;
}

export async function validateLinks(
  snapshot: Snapshot,
): Promise<{ documentId: string; target: string; reason: string }[]> {
  const errors: { documentId: string; target: string; reason: string }[] = [];
  const references = await loadReferences(snapshot.workspaceRoot);
  const resources = new Set(references?.resources ?? []);
  const docs = new Map(
    snapshot.documents.map((d) => [resolve(snapshot.workspaceRoot, d.path), d]),
  );
  for (const doc of snapshot.documents) {
    for (const target of doc.links) {
      if (isExternal(target)) {
        continue;
      }
      const [pathname, anchor] = target.split('#');
      if (decodeURIComponent(pathname).startsWith('/')) {
        errors.push({
          documentId: doc.id,
          target,
          reason: 'nonportable_local_link',
        });
        continue;
      }
      const resolvedPath = pathname
        ? resolve(
            dirname(resolve(snapshot.workspaceRoot, doc.path)),
            decodeURIComponent(pathname.split('?')[0]),
          )
        : resolve(snapshot.workspaceRoot, doc.path);
      const path = workspaceReference(snapshot.workspaceRoot, resolvedPath);
      let exists =
        docs.has(path) ||
        resources.has(
          relative(dirname(snapshot.workspaceRoot), path).split(sep).join('/'),
        );
      try {
        const info = await stat(path);
        exists = exists || info.isFile() || info.isDirectory();
      } catch {
        /* Report broken links with their source. */
      }
      if (!exists) {
        errors.push({ documentId: doc.id, target, reason: 'file_missing' });
      } else if (anchor && docs.has(path)) {
        const html = docs.get(path)!.html;
        const id = decodeURIComponent(anchor);
        if (!html.includes(`id="${id.replaceAll('"', '&quot;')}"`)) {
          errors.push({ documentId: doc.id, target, reason: 'anchor_missing' });
        }
      }
    }
  }
  return errors;
}
