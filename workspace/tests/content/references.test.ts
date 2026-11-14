import { mkdtemp, mkdir, writeFile, rm, cp } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { expect, it } from 'vitest';
import { loadSnapshot, validateLinks } from '../../src/content/catalog';
import {
  loadReferences,
  synchronizeReferences,
} from '../../src/content/references';

it('builds imported project rules without sibling sources and detects tampered knowledge inputs', async () => {
  const root = await mkdtemp(join(tmpdir(), 'workspace-reference-'));
  try {
    const project = join(root, 'project');
    const workspace = join(project, 'workspace');
    const standalone = join(root, 'standalone');
    await mkdir(join(workspace, 'content'), { recursive: true });
    await writeFile(
      join(project, 'AGENTS.md'),
      '# 规则\n\n[正文](workspace/content/a.md)\n',
    );
    await writeFile(
      join(workspace, 'content/a.md'),
      '# 正文\n\n[规则](../../AGENTS.md)\n',
    );
    await writeFile(
      join(workspace, 'catalog.json'),
      JSON.stringify([
        { id: 'rules', path: '../AGENTS.md', visibility: 'internal' },
        { id: 'document', path: 'content/a.md', visibility: 'internal' },
      ]),
    );
    await synchronizeReferences(workspace, project);
    await cp(workspace, standalone, { recursive: true });
    const snapshot = await loadSnapshot({
      workspaceRoot: standalone,
      scope: 'internal',
    });
    expect(
      snapshot.documents.find((document) => document.id === 'rules')?.source,
    ).toContain('# 规则');
    expect(await validateLinks(snapshot)).toEqual([]);
    await writeFile(join(project, 'AGENTS.md'), '# 当前规则\n');
    const live = await loadSnapshot({
      workspaceRoot: workspace,
      scope: 'internal',
      referenceMode: 'live',
    });
    expect(
      live.documents.find((document) => document.id === 'rules')?.source,
    ).toContain('当前规则');
    const references = await loadReferences(standalone);
    if (!references) {
      throw new Error('Expected imported references');
    }
    references.entries.rules.source = 'tampered';
    await writeFile(
      join(standalone, '.local/knowledge/references.json'),
      JSON.stringify(references),
    );
    await expect(
      loadSnapshot({ workspaceRoot: standalone, scope: 'internal' }),
    ).rejects.toThrow('Invalid knowledge reference');
  } finally {
    await rm(root, { recursive: true, force: true });
  }
});
