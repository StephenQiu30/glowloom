import { readFile } from 'node:fs/promises';
import path from 'node:path';
import { Mastra } from '@mastra/core';
import { loadConfig } from '../dist/src/config.js';
import { createDatabaseBundle } from '../dist/src/database/connection.js';
import {
  createDynamicPipelineWorkflow,
  PipelineRegistry,
} from '../dist/src/mastra/pipeline-engine.js';
import { PromptRegistry } from '../dist/src/observability/langfuse.js';
import { createAgentLogger } from '../dist/src/observability/logger.js';
import { SkillRegistry, skillWorkspace } from '../dist/src/skills/loader.js';

const [phase, workflowRunId, newRunId, directory] = process.argv.slice(2);
const config = loadConfig({
  GLOWLOOM_AGENT_DB_DSN: process.env.GLOWLOOM_TEST_SKILL_RUNTIME_DB_DSN,
  GLOWLOOM_AGENT_SERVICE_TOKEN: 's'.repeat(32),
  GLOWLOOM_AGENT_BUSINESS_TOKEN: 'b'.repeat(32),
  GLOWLOOM_AGENT_BUSINESS_ORIGIN: 'http://127.0.0.1:9',
  GLOWLOOM_AGENT_SKILLS_DIRS: path.join(directory, 'skills'),
  GLOWLOOM_AGENT_PIPELINES_DIRS: path.join(directory, 'pipelines'),
});
const db = createDatabaseBundle(config.dbDsn, 4);
const prompts = new PromptRegistry(
  undefined,
  createAgentLogger('error'),
  path.join(directory, 'prompts'),
);
try {
  await prompts.validateLocal();
  const skills = new SkillRegistry(config.skillDirectories);
  await skills.reload();
  const pipelines = new PipelineRegistry(config.pipelineDirectories);
  await pipelines.reload(skills);
  const executions = [];
  const workflow = createDynamicPipelineWorkflow({
    pipelines,
    skills,
    prompts,
    stageExecutor: async ({
      stage,
      skill,
      rolePrompt,
      skillInstructions,
      decisionId,
    }) => {
      const workspace = await skillWorkspace([skill]);
      const attachment = await workspace.skills.getAsset(
        skill.name,
        'references/input.txt',
      );
      const summary = {
        stage: stage.id,
        rolePrompt,
        instructions: skillInstructions,
        attachment: attachment.toString('utf8'),
        decisionId,
      };
      executions.push(summary);
      return { reviewRounds: 0, summary: JSON.stringify(summary) };
    },
  });
  const mastra = new Mastra({
    storage: db.mastraStore,
    workflows: { resources: workflow },
  });
  const runtime = mastra.getWorkflow('resources');
  async function start(runId) {
    const run = await runtime.createRun({ runId });
    const output = await run.start({
      inputData: {
        runId,
        projectId: runId,
        pipelineId: 'resource-restart-example',
      },
    });
    return { status: output.status, suspended: output.suspended };
  }

  let output;
  if (phase === 'start') {
    output = await start(workflowRunId);
  } else if (phase === 'restore') {
    const run = await runtime.createRun({ runId: workflowRunId });
    const resumed = await run.resume({
      step: 'execute-declarative-pipeline',
      resumeData: { approved: true, decisionId: 'approve-v1' },
    });
    output = {
      old: { status: resumed.status, result: resumed.result },
      fresh: await start(newRunId),
    };
  } else {
    throw new Error(`Invalid restart test phase: ${phase}`);
  }
  const deployed = JSON.parse(
    await readFile(path.join(directory, 'pipelines', 'restart.json'), 'utf8'),
  );
  process.stdout.write(
    JSON.stringify({
      pid: process.pid,
      deployedVersion: deployed.version,
      output,
      executions,
    }) + '\n',
  );
} finally {
  await Promise.all([prompts.shutdown(), db.close()]);
}
