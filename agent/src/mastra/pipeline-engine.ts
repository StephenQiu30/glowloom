import { readdir, readFile } from 'node:fs/promises';
import path from 'node:path';
import { resourceDirectory } from '../config.js';
import { createStep, createWorkflow } from '@mastra/core/workflows';
import { z } from 'zod';
import { SkillRegistry } from '../skills/loader.js';

export const pipelineApprovalGateSchema = z.object({
  required: z.boolean().default(true),
  stageKey: z.string(),
  quoteTool: z.string().optional(),
  summary: z.string().optional(),
});

export const pipelineStageSchema = z.object({
  id: z.string().min(1),
  name: z.string().min(1),
  role: z.string().min(1),
  skill: z.string().min(1),
  reviewer: z.string().optional(),
  maxReviewRounds: z.number().int().min(1).max(5).default(2),
  approvalGate: pipelineApprovalGateSchema.optional(),
});

export const pipelineDefinitionSchema = z.object({
  id: z.string().min(1),
  version: z.string().default('1.0.0'),
  description: z.string().default(''),
  defaultAspectRatio: z.enum(['9:16', '16:9']).default('9:16'),
  stages: z.array(pipelineStageSchema).min(1),
});

export type PipelineStage = z.infer<typeof pipelineStageSchema>;
export type PipelineDefinition = z.infer<typeof pipelineDefinitionSchema>;

export interface StageExecutionResult {
  stageId: string;
  role: string;
  skill: string;
  skillInstructionsInjected: boolean;
  allowedTools: string[];
  reviewRounds: number;
  decisionId?: string | undefined;
  summary: string;
}

export type DynamicStageExecutor = (params: {
  runId: string;
  projectId: string;
  stage: PipelineStage;
  skillInstructions: string;
  allowedTools: string[];
  decisionId?: string;
  previousOutputs: StageExecutionResult[];
  context: Record<string, unknown>;
}) => Promise<
  Omit<
    StageExecutionResult,
    'stageId' | 'role' | 'skill' | 'skillInstructionsInjected' | 'allowedTools'
  >
>;

export class PipelineRegistry {
  private readonly pipelines = new Map<string, PipelineDefinition>();
  private readonly dirs: string[];

  constructor(dirs?: string[]) {
    const defaultDir = resourceDirectory('pipelines');
    const extraDirs = (process.env.GLOWLOOM_AGENT_PIPELINES_DIRS ?? '')
      .split(',')
      .map((s) => s.trim())
      .filter(Boolean);
    this.dirs = dirs ?? [defaultDir, ...extraDirs];
  }

  async reload(): Promise<PipelineDefinition[]> {
    this.pipelines.clear();
    for (const dir of this.dirs) {
      let entries: string[];
      try {
        entries = await readdir(dir);
      } catch {
        continue;
      }
      for (const file of entries.filter((f) => f.endsWith('.json')).sort()) {
        const raw = await readFile(path.join(dir, file), 'utf8');
        const parsed = pipelineDefinitionSchema.parse(JSON.parse(raw));
        this.pipelines.set(parsed.id, parsed);
      }
    }
    return this.list();
  }

  register(definition: PipelineDefinition): void {
    this.pipelines.set(
      definition.id,
      pipelineDefinitionSchema.parse(definition),
    );
  }

  get(id: string): PipelineDefinition | undefined {
    return this.pipelines.get(id);
  }

  list(): PipelineDefinition[] {
    return Array.from(this.pipelines.values());
  }
}

const dynamicPipelineInputSchema = z.object({
  runId: z.string(),
  projectId: z.string(),
  pipelineId: z.string().optional(),
  inlinePipeline: pipelineDefinitionSchema.optional(),
  requireBillableApprovals: z.boolean().default(true),
  context: z.record(z.string(), z.unknown()).default({}),
});

const stageResultSchema = z.object({
  stageId: z.string(),
  role: z.string(),
  skill: z.string(),
  skillInstructionsInjected: z.boolean(),
  allowedTools: z.array(z.string()),
  reviewRounds: z.number().int().nonnegative(),
  decisionId: z.string().optional(),
  summary: z.string(),
});

const dynamicPipelineOutputSchema = z.object({
  runId: z.string(),
  projectId: z.string(),
  pipelineId: z.string(),
  status: z.enum(['completed', 'rejected']),
  completedStages: z.array(stageResultSchema),
  approvals: z.record(z.string(), z.string()),
  rejectedAtStage: z.string().optional(),
});

const pipelineSuspendSchema = z.object({
  runId: z.string(),
  pipelineId: z.string(),
  stageId: z.string(),
  stageKey: z.string(),
  stageIndex: z.number().int().nonnegative(),
  summary: z.string(),
});

const pipelineResumeSchema = z.object({
  approved: z.boolean(),
  decisionId: z.string(),
  note: z.string().optional(),
});

const pipelineStateSchema = z.object({
  nextStageIndex: z.number().int().nonnegative().default(0),
  completedStages: z.array(stageResultSchema).default([]),
  approvals: z.record(z.string(), z.string()).default({}),
  approvedStageIds: z.record(z.string(), z.string()).default({}),
});

const defaultStageExecutor: DynamicStageExecutor = async ({
  stage,
  decisionId,
}) => ({
  reviewRounds: stage.reviewer ? 1 : 0,
  ...(decisionId ? { decisionId } : {}),
  summary: `Executed stage "${stage.id}" via role=${stage.role} skill=${stage.skill}`,
});

/**
 * Builds a single, zero-hardcoding Mastra Workflow engine capable of executing any declarative
 * JSON pipeline with arbitrary stages, dynamic SKILL.md injection, bounded reviewer loops,
 * and durable multi-stage HITL suspend/resume gates.
 */
export function createDynamicPipelineWorkflow(options: {
  pipelines: PipelineRegistry;
  skills: SkillRegistry;
  stageExecutor?: DynamicStageExecutor;
}) {
  const executeStage = options.stageExecutor ?? defaultStageExecutor;

  const runStagesStep = createStep({
    id: 'execute-declarative-pipeline',
    description:
      'Data-driven stage loop executing any declarative pipeline with dynamic Skill injection and HITL suspend/resume',
    stateSchema: pipelineStateSchema,
    inputSchema: dynamicPipelineInputSchema,
    outputSchema: dynamicPipelineOutputSchema,
    suspendSchema: pipelineSuspendSchema,
    resumeSchema: pipelineResumeSchema,
    execute: async ({ inputData, resumeData, suspend, state, setState }) => {
      const pipeline =
        inputData.inlinePipeline ??
        (inputData.pipelineId
          ? options.pipelines.get(inputData.pipelineId)
          : undefined);

      if (!pipeline) {
        throw new Error(
          `Declarative pipeline not found: ${inputData.pipelineId ?? '(none)'}`,
        );
      }

      let stageIndex = state.nextStageIndex ?? 0;
      const completedStages: StageExecutionResult[] = [
        ...(state.completedStages ?? []),
      ];
      const approvals: Record<string, string> = { ...(state.approvals ?? {}) };
      const approvedStageIds: Record<string, string> = {
        ...(state.approvedStageIds ?? {}),
      };

      // Handle incoming resume decision for the currently suspended stage.
      if (resumeData && stageIndex < pipeline.stages.length) {
        const currentStage = pipeline.stages[stageIndex]!;
        if (!resumeData.approved) {
          return {
            runId: inputData.runId,
            projectId: inputData.projectId,
            pipelineId: pipeline.id,
            status: 'rejected' as const,
            completedStages,
            approvals,
            rejectedAtStage: currentStage.id,
          };
        }
        const gateKey = currentStage.approvalGate?.stageKey ?? currentStage.id;
        approvals[gateKey] = resumeData.decisionId;
        approvedStageIds[currentStage.id] = resumeData.decisionId;
        setState({
          nextStageIndex: stageIndex,
          completedStages,
          approvals,
          approvedStageIds,
        });
      }

      while (stageIndex < pipeline.stages.length) {
        const stage = pipeline.stages[stageIndex]!;
        const decisionId = approvedStageIds[stage.id];

        if (
          inputData.requireBillableApprovals &&
          stage.approvalGate?.required &&
          !decisionId
        ) {
          setState({
            nextStageIndex: stageIndex,
            completedStages,
            approvals,
            approvedStageIds,
          });
          return await suspend({
            runId: inputData.runId,
            pipelineId: pipeline.id,
            stageId: stage.id,
            stageKey: stage.approvalGate.stageKey,
            stageIndex,
            summary:
              stage.approvalGate.summary ??
              `Approval required for stage ${stage.name}`,
          });
        }

        const skillInstructions = options.skills.composeInstructions([
          stage.skill,
        ]);
        const allowedTools = options.skills.resolveAllowedTools([stage.skill]);
        const result = await executeStage({
          runId: inputData.runId,
          projectId: inputData.projectId,
          stage,
          skillInstructions,
          allowedTools,
          ...(decisionId ? { decisionId } : {}),
          previousOutputs: completedStages,
          context: inputData.context,
        });

        const boundedRounds = Math.min(
          result.reviewRounds,
          stage.maxReviewRounds,
        );
        completedStages.push({
          stageId: stage.id,
          role: stage.role,
          skill: stage.skill,
          skillInstructionsInjected: skillInstructions.length > 0,
          allowedTools,
          reviewRounds: boundedRounds,
          ...(decisionId ? { decisionId } : {}),
          summary: result.summary,
        });

        stageIndex += 1;
        setState({
          nextStageIndex: stageIndex,
          completedStages,
          approvals,
          approvedStageIds,
        });
      }

      return {
        runId: inputData.runId,
        projectId: inputData.projectId,
        pipelineId: pipeline.id,
        status: 'completed' as const,
        completedStages,
        approvals,
      };
    },
  });

  return createWorkflow({
    id: 'dynamic-pipeline-workflow',
    description:
      'Zero-hardcoding declarative pipeline runner supporting dynamic skills, bounded reviewer loops, and multi-stage HITL approvals',
    stateSchema: pipelineStateSchema,
    inputSchema: dynamicPipelineInputSchema,
    outputSchema: dynamicPipelineOutputSchema,
  })
    .then(runStagesStep)
    .commit();
}
