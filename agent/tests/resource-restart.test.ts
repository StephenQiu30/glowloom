import { execFile } from 'node:child_process';
import { randomUUID } from 'node:crypto';
import { mkdir, mkdtemp, rm, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { promisify } from 'node:util';
import { expect, it } from 'vitest';
import { promptByRole } from '../src/observability/langfuse.js';
import { PipelineRegistry } from '../src/mastra/pipeline-engine.js';

const exec = promisify(execFile);
const dsn = process.env.GLOWLOOM_TEST_SKILL_RUNTIME_DB_DSN;
const worker = fileURLToPath(
  new URL('./resource-restart-worker.mjs', import.meta.url),
);

function definition(version: string) {
  const stage = { role: 'production', skill: 'restart-fixture' };
  return {
    id: 'resource-restart-example',
    version,
    stages: [
      { ...stage, id: `prepare-${version}`, name: 'Prepare' },
      {
        ...stage,
        id: `publish-${version}`,
        name: 'Publish',
        approvalGate: {
          stageKey: `gate-${version}`,
          summary: `Approval for ${version}`,
        },
      },
    ],
  };
}

it('rejects missing roots, duplicate workflow IDs, and unknown Skill references', async () => {
  const root = await mkdtemp(path.join(tmpdir(), 'agent-invalid-resources-'));
  try {
    const file = path.join(root, 'a.json');
    await writeFile(file, JSON.stringify(definition('v1')));
    await expect(
      new PipelineRegistry([path.join(root, 'missing')]).reload(),
    ).rejects.toThrow('missing');
    const { SkillRegistry } = await import('../src/skills/loader.js');
    await expect(
      new PipelineRegistry([root]).reload(new SkillRegistry()),
    ).rejects.toThrow('Unknown Skill restart-fixture');
    await writeFile(
      path.join(root, 'b.json'),
      JSON.stringify(definition('v2')),
    );
    await expect(new PipelineRegistry([root]).reload()).rejects.toThrow(
      'Duplicate pipeline ID',
    );
  } finally {
    await rm(root, { force: true, recursive: true });
  }
});

it.skipIf(!dsn)(
  'resumes the original workflow, Skill attachment, and Prompt after resource publication and process restart',
  async () => {
    const database = new URL(dsn ?? '');
    if (
      database.hostname !== '127.0.0.1' ||
      database.username !== 'glowloom_agent' ||
      !database.pathname.endsWith('_test')
    ) {
      throw new Error('Expected a restricted, isolated test database');
    }
    const root = await mkdtemp(path.join(tmpdir(), 'agent-resource-restart-'));
    const skills = path.join(root, 'skills', 'restart-fixture');
    const pipelines = path.join(root, 'pipelines');
    const prompts = path.join(root, 'prompts');
    const oldRunId = randomUUID();
    const newRunId = randomUUID();
    async function publish(version: string) {
      await Promise.all([
        writeFile(
          path.join(pipelines, 'restart.json'),
          JSON.stringify(definition(version)),
        ),
        writeFile(
          path.join(skills, 'SKILL.md'),
          `---\nname: restart-fixture\ndescription: Restart fixture\n---\n\ninstructions ${version}\n`,
        ),
        writeFile(
          path.join(skills, 'references', 'input.txt'),
          `attachment ${version}`,
        ),
        ...Object.values(promptByRole).map((name) =>
          writeFile(path.join(prompts, `${name}.md`), `${name} ${version}`),
        ),
      ]);
    }
    async function runWorker(phase: string) {
      const { stdout } = await exec(
        process.execPath,
        [worker, phase, oldRunId, newRunId, root],
        {
          cwd: root,
          timeout: 15_000,
        },
      );
      return JSON.parse(stdout.trim().split('\n').at(-1) ?? '');
    }
    try {
      await Promise.all([
        mkdir(path.join(skills, 'references'), { recursive: true }),
        mkdir(pipelines),
        mkdir(prompts),
      ]);
      await publish('v1');
      const before = await runWorker('start');
      expect(before.deployedVersion).toBe('v1');
      expect(before.output.status).toBe('suspended');
      expect(before.executions).toMatchObject([
        { stage: 'prepare-v1', rolePrompt: 'production v1' },
      ]);

      await publish('v2');
      const after = await runWorker('restore');
      expect(after.pid).not.toBe(before.pid);
      expect(after.deployedVersion).toBe('v2');
      expect(after.output.old).toMatchObject({
        status: 'success',
        result: {
          status: 'completed',
          approvals: { 'gate-v1': 'approve-v1' },
          completedStages: [
            { stageId: 'prepare-v1' },
            { stageId: 'publish-v1' },
          ],
        },
      });
      expect(after.output.fresh.status).toBe('suspended');
      expect(after.executions).toEqual([
        {
          stage: 'publish-v1',
          rolePrompt: 'production v1',
          instructions: expect.stringContaining('v1'),
          attachment: 'attachment v1',
          decisionId: 'approve-v1',
        },
        {
          stage: 'prepare-v2',
          rolePrompt: 'production v2',
          instructions: expect.stringContaining('v2'),
          attachment: 'attachment v2',
        },
      ]);
    } finally {
      await rm(root, { force: true, recursive: true });
    }
  },
  35_000,
);
