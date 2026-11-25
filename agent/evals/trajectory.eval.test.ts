import { Mastra } from '@mastra/core';
import { createScorer } from '@mastra/core/evals';
import { InMemoryStore } from '@mastra/core/storage';
import { describe, expect, it } from 'vitest';
import { creativeDeliveryWorkflow } from '../src/mastra/workflows.js';
import {
  filterToolsByAllowlist,
  loadDeclarativeSkills,
} from '../src/skills/loader.js';

interface TrajectoryRun {
  steps: Array<{ tool: string; approved?: boolean }>;
  maxAllowedSteps: number;
  allowlist: string[];
}

const trajectoryComplianceScorer = createScorer<TrajectoryRun, TrajectoryRun>({
  id: 'glowloom-trajectory-compliance',
  name: 'Glowloom Trajectory Compliance',
  description:
    'Deterministic CI scorer verifying tool allowlist, step cap, and approval gate ordering',
}).generateScore(({ run }) => {
  const output = run.output;
  if (!output) return 0;
  const allowSet = new Set(output.allowlist);
  const withinStepCap = output.steps.length <= output.maxAllowedSteps;
  const allToolsAllowed = output.steps.every((s) => allowSet.has(s.tool));
  const billableApproved = output.steps
    .filter((s) => s.tool === 'submit_storyboard_generation')
    .every((s) => s.approved === true);

  return withinStepCap && allToolsAllowed && billableApproved ? 1 : 0;
});

describe('Offline CI Trajectory & Constraint Evals (Mastra Scorers)', () => {
  it('passes compliant storyboard production trajectory after HITL approval', async () => {
    const skills = await loadDeclarativeSkills();
    const storyboardSkill = skills.find(
      (s) => s.name === 'storyboard-production',
    );
    expect(storyboardSkill).toBeDefined();

    const mastra = new Mastra({
      storage: new InMemoryStore(),
      workflows: { creativeDeliveryWorkflow },
    });
    const wfRun = await mastra
      .getWorkflow('creativeDeliveryWorkflow')
      .createRun();
    const suspended = await wfRun.start({
      inputData: {
        runId: 'eval-run-1',
        projectId: 'eval-proj-1',
        goal: 'Produce opening 3 shots',
        estimatedCostMicros: 300_000,
        requiresApproval: true,
      },
    });
    expect(suspended.status).toBe('suspended');

    const resumed = await wfRun.resume({
      step: 'approval-gate',
      resumeData: { approved: true, decisionId: 'eval-dec-1' },
    });
    expect(resumed.status).toBe('success');

    const sample: TrajectoryRun = {
      maxAllowedSteps: 8,
      allowlist: storyboardSkill?.allowedTools ?? [],
      steps: [
        { tool: 'read_project_script' },
        { tool: 'read_project_bible' },
        { tool: 'submit_storyboard_generation', approved: true },
      ],
    };

    const scoreResult = await trajectoryComplianceScorer.run({
      input: sample,
      output: sample,
    });
    expect(scoreResult.score).toBe(1);
  });

  it('flags unauthorized tool invocation outside skill allowlist', async () => {
    const allowed = filterToolsByAllowlist(
      {
        read_project_script: true,
        delete_project_data: true,
      },
      ['read_project_script'],
    );
    expect(Object.keys(allowed)).toEqual(['read_project_script']);

    const badTrajectory: TrajectoryRun = {
      maxAllowedSteps: 8,
      allowlist: ['read_project_script'],
      steps: [
        { tool: 'read_project_script' },
        { tool: 'submit_storyboard_generation', approved: false },
      ],
    };

    const scoreResult = await trajectoryComplianceScorer.run({
      input: badTrajectory,
      output: badTrajectory,
    });
    expect(scoreResult.score).toBe(0);
  });
});
