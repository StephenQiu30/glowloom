import { randomUUID } from 'node:crypto';
import { mkdtemp, mkdir, readFile, rm, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import path from 'node:path';
import { Mastra } from '@mastra/core';
import { InMemoryStore } from '@mastra/core/storage';
import { describe, expect, it } from 'vitest';
import { resourceDirectory } from '../src/config.js';
import {
  canvasWorkflowSchema,
  createCanvasWorkflow,
} from '../src/mastra/canvas-workflow.js';
import {
  pinnedSkillSchema,
  SkillRegistry,
  skillWorkspace,
} from '../src/skills/loader.js';

describe('canvas Skill execution', () => {
  it('discovers every packaged stage and reads the original reference, script, and image', async () => {
    const templateFile = path.resolve(
      resourceDirectory('skills'),
      '../../frontend/src/components/canvas/editor/skill-workflow-templates.json',
    );
    const templates = canvasWorkflowSchema.array().parse(
      JSON.parse(await readFile(templateFile, 'utf8')).map(
        (template: { title: string; nodes: Array<{ skill: string }> }) => ({
          id: template.title,
          nodes: template.nodes.map((node) => ({ ...node, id: node.skill })),
        }),
      ),
    );
    const packages = await new SkillRegistry([
      resourceDirectory('skills'),
    ]).discover();
    const stages = new Set(
      templates.flatMap((template) => template.nodes.map((node) => node.skill)),
    );
    const selected = packages.filter((pkg) => stages.has(pkg.name));
    expect(stages.size).toBe(19);
    expect(selected).toHaveLength(19);
    expect(
      selected.reduce(
        (count, pkg) => count + Object.keys(pkg.tree.entries).length,
        0,
      ),
    ).toBe(263);
    const workspace = await skillWorkspace(selected);
    expect(await workspace.skills?.list()).toHaveLength(19);
    expect(await workspace.skills?.get('short-drama-review')).toBeTruthy();
    for (const [name, file] of [
      ['short-drama-review', 'references/stage-contract.md'],
      ['novel-staging', 'scripts/novel-staging.mjs'],
      ['novel-staging', 'assets/report.webp'],
    ] as const) {
      const original = await readFile(
        path.join(resourceDirectory('skills'), name, file),
      );
      expect(
        (await workspace.skills?.getAsset(name, file))?.equals(original),
      ).toBe(true);
    }
  });

  it('discovers a replacement at runtime while a serialized run keeps its original bytes', async () => {
    const root = await mkdtemp(path.join(tmpdir(), 'glowloom-skills-'));
    try {
      const dir = path.join(root, 'test-skill');
      await mkdir(path.join(dir, 'references'), { recursive: true });
      await writeFile(
        path.join(dir, 'SKILL.md'),
        '---\nname: test-skill\ndescription: A test Skill package\n---\n\nUse references/input.md.\n',
      );
      const attachment = path.join(dir, 'references/input.md');
      await writeFile(attachment, 'original');
      const registry = new SkillRegistry([root]);
      const [before] = await registry.discover();
      await writeFile(attachment, 'replacement');
      const [after] = await registry.discover();
      expect(after?.digest).not.toEqual(before?.digest);
      await rm(dir, { recursive: true });
      expect(await registry.discover()).toEqual([]);
      const first = await skillWorkspace([
        pinnedSkillSchema.parse(JSON.parse(JSON.stringify(before))),
      ]);
      const second = await skillWorkspace([
        pinnedSkillSchema.parse(JSON.parse(JSON.stringify(after))),
      ]);
      expect(
        (
          await first.skills?.getAsset('test-skill', 'references/input.md')
        )?.toString(),
      ).toBe('original');
      expect(
        (
          await second.skills?.getAsset('test-skill', 'references/input.md')
        )?.toString(),
      ).toBe('replacement');
    } finally {
      await rm(root, { recursive: true, force: true });
    }
  });

  it('executes edited graph dependencies and rejects cycles before execution', async () => {
    const graph = {
      id: 'group',
      nodes: [
        {
          id: 'review',
          skill: 'short-drama-review',
          title: 'Review',
          task: 'Use the edited input',
          after: ['script', 'reference'],
        },
        {
          id: 'script',
          skill: 'novel-script',
          title: 'Script',
          task: 'Generate a script',
          after: [],
        },
        {
          id: 'reference',
          skill: 'character-refs',
          title: 'Reference',
          task: 'Generate character direction',
          after: [],
        },
      ],
    };
    const storage = new InMemoryStore();
    const workflow = createCanvasWorkflow(async (node, inputs) => ({
      nodeId: node.id,
      title: node.title,
      text: [node.task, ...inputs.map((input) => input.text)].join(' / '),
      skillDigest: 'test',
    }));
    workflow.__registerMastra(new Mastra({ storage }));
    const runId = randomUUID();
    const run = await workflow.createRun({ runId });
    const result = await run.start({ inputData: { runId, graph } });
    expect(result.status).toBe('success');
    if (result.status !== 'success') throw new Error('Canvas workflow failed');
    expect(result.result.results.map((node) => node.nodeId)).toEqual([
      'script',
      'reference',
      'review',
    ]);
    expect(result.result.results[2]?.text).toBe(
      'Use the edited input / Generate a script / Generate character direction',
    );
    graph.nodes[1]?.after.push('review');
    expect(
      canvasWorkflowSchema.safeParse(graph).error?.issues[0]?.message,
    ).toContain('cycle');
  });
});
