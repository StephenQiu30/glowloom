import { readdir, readFile } from 'node:fs/promises';
import { createHash } from 'node:crypto';
import path from 'node:path';
import { resourceDirectory } from '../config.js';
import { createStep, createWorkflow } from '@mastra/core/workflows';
import { z } from 'zod';
import {
  declarativeSkillSchema,
  pinnedSkillSchema,
  type SkillRegistry,
} from '../skills/loader.js';
import {
  promptByRole,
  promptSnapshotSchema,
  type PromptRegistry,
} from '../observability/langfuse.js';

const roles = z.custom<keyof typeof promptByRole>(
  (value) => typeof value === 'string' && Object.hasOwn(promptByRole, value),
  'Unknown Agent role',
);

export const pipelineApprovalGateSchema = z.object({
  required: z.boolean().default(true),
  stageKey: z.string().min(1),
  quoteTool: z.string().min(1).optional(),
  summary: z.string().optional(),
});

export const pipelineStageSchema = z.object({
  id: z.string().min(1),
  name: z.string().min(1),
  role: roles,
  skill: z.string().min(1),
  reviewer: roles.optional(),
  maxReviewRounds: z.number().int().min(1).max(5).default(2),
  approvalGate: pipelineApprovalGateSchema.optional(),
});

export const pipelineDefinitionSchema = z
  .object({
    id: z.string().min(1),
    version: z.string().min(1).default('1.0.0'),
    description: z.string().default(''),
    defaultAspectRatio: z.enum(['9:16', '16:9']).default('9:16'),
    stages: z.array(pipelineStageSchema).min(1),
  })
  .refine(
    ({ stages }) =>
      new Set(stages.map((stage) => stage.id)).size === stages.length,
    'Duplicate stage ID',
  )
  .refine(({ stages }) => {
    const gates = stages.flatMap((stage) =>
      stage.approvalGate ? [stage.approvalGate.stageKey] : [],
    );
    return new Set(gates).size === gates.length;
  }, 'Duplicate approval stage key');

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
  skill: PinnedStageSkill;
  rolePrompt: string;
  reviewerPrompt?: string;
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

  constructor(dirs: string[] = [resourceDirectory('pipelines')]) {
    this.dirs = dirs;
  }

  async reload(skills?: SkillRegistry): Promise<PipelineDefinition[]> {
    const discovered = new Map<string, PipelineDefinition>();
    for (const dir of this.dirs) {
      const entries = await readdir(dir, { withFileTypes: true });
      for (const entry of entries) {
        if (!entry.name.endsWith('.json')) continue;
        const file = path.join(dir, entry.name);
        try {
          if (!entry.isFile()) throw new Error('Expected a regular JSON file');
          const raw = await readFile(file, 'utf8');
          const parsed = pipelineDefinitionSchema.parse(JSON.parse(raw));
          if (discovered.has(parsed.id))
            throw new Error(`Duplicate pipeline ID: ${parsed.id}`);
          if (skills) {
            for (const stage of parsed.stages) {
              if (!skills.get(stage.skill))
                throw new Error(
                  `Unknown Skill ${stage.skill} in stage ${stage.id}`,
                );
            }
          }
          discovered.set(parsed.id, parsed);
        } catch (cause) {
          throw new Error(`Invalid pipeline file: ${file}: ${String(cause)}`, {
            cause,
          });
        }
      }
    }
    this.pipelines.clear();
    for (const [id, definition] of discovered)
      this.pipelines.set(id, definition);
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

const stageSkillSchema = z.union([
  pinnedSkillSchema,
  declarativeSkillSchema.strict(),
]);
type PinnedStageSkill = z.infer<typeof stageSkillSchema>;
const pipelineResourcesSchema = z.object({
  pipeline: pipelineDefinitionSchema,
  digest: z.string().regex(/^[a-f0-9]{64}$/),
  skills: z.record(z.string(), stageSkillSchema),
  prompts: promptSnapshotSchema,
});

const pipelineStateSchema = z.object({
  resources: pipelineResourcesSchema.optional(),
  nextStageIndex: z.number().int().nonnegative().default(0),
  completedStages: z.array(stageResultSchema).default([]),
  approvals: z.record(z.string(), z.string()).default({}),
  approvedStageIds: z.record(z.string(), z.string()).default({}),
});

/**
 * Builds a single, zero-hardcoding Mastra Workflow engine capable of executing any declarative
 * JSON pipeline with arbitrary stages, dynamic SKILL.md injection, bounded reviewer loops,
 * and durable multi-stage HITL suspend/resume gates.
 */
export function createDynamicPipelineWorkflow(options: {
  pipelines: PipelineRegistry;
  skills: SkillRegistry;
  prompts: PromptRegistry;
  stageExecutor: DynamicStageExecutor;
}) {
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
      let resources = state?.resources;
      if (!resources) {
        if (resumeData)
          throw new Error('Workflow resource snapshot is missing');
        const pipeline =
          inputData.inlinePipeline ??
          (inputData.pipelineId
            ? options.pipelines.get(inputData.pipelineId)
            : undefined);
        if (!pipeline)
          throw new Error(
            `Declarative pipeline not found: ${inputData.pipelineId ?? '(none)'}`,
          );
        const skills = Object.fromEntries(
          [...new Set(pipeline.stages.map((stage) => stage.skill))].map(
            (name) => {
              const skill = options.skills.get(name);
              if (!skill) throw new Error(`Unknown Skill: ${name}`);
              return [name, skill];
            },
          ),
        );
        resources = pipelineResourcesSchema.parse({
          pipeline,
          digest: createHash('sha256')
            .update(JSON.stringify(pipeline))
            .digest('hex'),
          skills,
          prompts: await options.prompts.snapshot(),
        });
      }
      const { pipeline, prompts, skills } = resources;

      let stageIndex = state?.nextStageIndex ?? 0;
      const completedStages: StageExecutionResult[] = [
        ...(state?.completedStages ?? []),
      ];
      const approvals: Record<string, string> = { ...(state?.approvals ?? {}) };
      const approvedStageIds: Record<string, string> = {
        ...(state?.approvedStageIds ?? {}),
      };
      const saveState = () =>
        setState({
          resources,
          nextStageIndex: stageIndex,
          completedStages,
          approvals,
          approvedStageIds,
        });
      saveState();

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
        saveState();
      }

      while (stageIndex < pipeline.stages.length) {
        const stage = pipeline.stages[stageIndex]!;
        const decisionId = approvedStageIds[stage.id];

        if (
          inputData.requireBillableApprovals &&
          stage.approvalGate?.required &&
          !decisionId
        ) {
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

        const skill = skills[stage.skill];
        if (!skill) throw new Error(`Skill snapshot missing: ${stage.skill}`);
        const { instructions: skillInstructions, allowedTools } = skill;
        const result = await options.stageExecutor({
          runId: inputData.runId,
          projectId: inputData.projectId,
          stage,
          skill,
          rolePrompt: prompts[promptByRole[stage.role]].text,
          ...(stage.reviewer
            ? { reviewerPrompt: prompts[promptByRole[stage.reviewer]].text }
            : {}),
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
        saveState();
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
