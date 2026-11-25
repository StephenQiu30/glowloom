import { Mastra } from '@mastra/core';
import { InMemoryStore } from '@mastra/core/storage';
import { beforeAll, describe, expect, it } from 'vitest';
import {
  createDynamicPipelineWorkflow,
  PipelineRegistry,
} from '../src/mastra/pipeline-engine.js';
import { SkillRegistry } from '../src/skills/loader.js';

describe('Zero-hardcoding Data-Driven Mastra Pipeline Engine & Dynamic SkillRegistry', () => {
  const skills = new SkillRegistry();
  const pipelines = new PipelineRegistry();
  let mastra: Mastra;

  beforeAll(async () => {
    await skills.reload();
    await pipelines.reload();
    const dynamicPipelineWorkflow = createDynamicPipelineWorkflow({
      pipelines,
      skills,
    });
    mastra = new Mastra({
      storage: new InMemoryStore(),
      workflows: { dynamicPipelineWorkflow },
    });
  });

  it('executes declarative vertical-short-drama.json across all 6 stages and 3 HITL approval gates', async () => {
    const workflow = mastra.getWorkflow('dynamicPipelineWorkflow');
    const run = await workflow.createRun();

    const gate1 = await run.start({
      inputData: {
        runId: 'dyn-run-1',
        projectId: 'proj-drama-1',
        pipelineId: 'vertical-short-drama',
        requireBillableApprovals: true,
        context: { sourceKind: 'novel_text', episodeNumber: 1 },
      },
    });
    expect(gate1.status).toBe('suspended');

    const gate2 = await run.resume({
      step: 'execute-declarative-pipeline',
      resumeData: { approved: true, decisionId: 'dec-assets-1' },
    });
    expect(gate2.status).toBe('suspended');

    const gate3 = await run.resume({
      step: 'execute-declarative-pipeline',
      resumeData: { approved: true, decisionId: 'dec-grid-1' },
    });
    expect(gate3.status).toBe('suspended');

    const done = await run.resume({
      step: 'execute-declarative-pipeline',
      resumeData: { approved: true, decisionId: 'dec-video-1' },
    });

    expect(done.status).toBe('success');
    if (done.status === 'success') {
      expect(done.result.status).toBe('completed');
      expect(done.result.completedStages).toHaveLength(6);
      expect(
        done.result.completedStages.every(
          (s: { skillInstructionsInjected: boolean }) =>
            s.skillInstructionsInjected,
        ),
      ).toBe(true);
      expect(done.result.approvals).toEqual({
        assets_turnaround: 'dec-assets-1',
        storyboard_grid_2x2: 'dec-grid-1',
        video_i2v_audio: 'dec-video-1',
      });
    }
  });

  it('dynamically adds a brand-new Skill and 7th stage at runtime with ZERO engine code changes', async () => {
    skills.register({
      name: 'bgm-emotion-scoring',
      description: 'Dynamically registered BGM scoring skill',
      allowedTools: ['media_estimate_quote', 'media_submit_tts'],
      instructions: 'Select emotional BGM cues aligned with scene beats.',
    });

    const base = pipelines.get('vertical-short-drama');
    expect(base).toBeDefined();
    const customPipeline = {
      ...base!,
      id: 'vertical-short-drama-with-bgm',
      stages: [
        ...base!.stages,
        {
          id: 'score-bgm',
          name: 'Emotional BGM Scoring',
          role: 'production',
          skill: 'bgm-emotion-scoring',
          maxReviewRounds: 1,
          approvalGate: {
            required: true,
            stageKey: 'bgm_scoring',
            summary: 'Approve BGM generation quote',
          },
        },
      ],
    };

    const workflow = mastra.getWorkflow('dynamicPipelineWorkflow');
    const run = await workflow.createRun();
    let state = await run.start({
      inputData: {
        runId: 'dyn-run-custom-7',
        projectId: 'proj-drama-2',
        inlinePipeline: customPipeline,
        requireBillableApprovals: true,
        context: {},
      },
    });

    // Resume all 4 approval gates (3 original + 1 dynamically added BGM stage)
    for (let i = 1; i <= 4; i += 1) {
      expect(state.status).toBe('suspended');
      state = await run.resume({
        step: 'execute-declarative-pipeline',
        resumeData: { approved: true, decisionId: `dec-custom-${i}` },
      });
    }

    expect(state.status).toBe('success');
    if (state.status === 'success') {
      expect(state.result.status).toBe('completed');
      expect(state.result.completedStages).toHaveLength(7);
      const bgmStage = state.result.completedStages[6];
      expect(bgmStage?.stageId).toBe('score-bgm');
      expect(bgmStage?.skill).toBe('bgm-emotion-scoring');
      expect(bgmStage?.skillInstructionsInjected).toBe(true);
      expect(bgmStage?.allowedTools).toEqual([
        'media_estimate_quote',
        'media_submit_tts',
      ]);
      expect(state.result.approvals.bgm_scoring).toBe('dec-custom-4');
    }
  });
});
