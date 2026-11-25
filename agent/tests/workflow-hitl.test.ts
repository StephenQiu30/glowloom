import { Mastra } from '@mastra/core';
import { InMemoryStore } from '@mastra/core/storage';
import { describe, expect, it } from 'vitest';
import { creativeDeliveryWorkflow } from '../src/mastra/workflows.js';

describe('Mastra creativeDeliveryWorkflow HITL suspend/resume', () => {
  const mastra = new Mastra({
    storage: new InMemoryStore(),
    workflows: { creativeDeliveryWorkflow },
  });

  it('suspends at approval-gate and completes when creator approves', async () => {
    const workflow = mastra.getWorkflow('creativeDeliveryWorkflow');
    const run = await workflow.createRun();
    const initial = await run.start({
      inputData: {
        runId: 'run-hitl-1',
        projectId: 'proj-1',
        goal: 'Generate 6 storyboard frames for scene 1',
        estimatedCostMicros: 500_000,
        requiresApproval: true,
      },
    });

    expect(initial.status).toBe('suspended');

    const resumed = await run.resume({
      step: 'approval-gate',
      resumeData: {
        approved: true,
        decisionId: 'dec-1',
      },
    });

    expect(resumed.status).toBe('success');
    if (resumed.status === 'success') {
      expect(resumed.result.status).toBe('completed');
      expect(resumed.result.decisionId).toBe('dec-1');
    }
  });

  it('records rejection cleanly when creator rejects suspended proposal', async () => {
    const workflow = mastra.getWorkflow('creativeDeliveryWorkflow');
    const run = await workflow.createRun();
    const initial = await run.start({
      inputData: {
        runId: 'run-hitl-2',
        projectId: 'proj-1',
        goal: 'Render full 4K sequence',
        estimatedCostMicros: 2_000_000,
        requiresApproval: true,
      },
    });

    expect(initial.status).toBe('suspended');

    const resumed = await run.resume({
      step: 'approval-gate',
      resumeData: {
        approved: false,
        decisionId: 'dec-2',
        reviewerNote: 'Budget too high',
      },
    });

    expect(resumed.status).toBe('success');
    if (resumed.status === 'success') {
      expect(resumed.result.status).toBe('rejected');
      expect(resumed.result.summary).toBe('Budget too high');
    }
  });
});
