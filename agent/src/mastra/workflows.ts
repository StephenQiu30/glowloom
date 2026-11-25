import { createStep, createWorkflow } from '@mastra/core/workflows';
import { z } from 'zod';

const proposalInputSchema = z.object({
  runId: z.string(),
  projectId: z.string(),
  goal: z.string(),
  estimatedCostMicros: z.number().int().nonnegative().default(0),
  requiresApproval: z.boolean().default(true),
});

const proposalOutputSchema = z.object({
  runId: z.string(),
  projectId: z.string(),
  proposalSummary: z.string(),
  estimatedCostMicros: z.number().int().nonnegative(),
  requiresApproval: z.boolean(),
});

const approvalSuspendSchema = z.object({
  runId: z.string(),
  proposalSummary: z.string(),
  estimatedCostMicros: z.number().int().nonnegative(),
  reason: z.string(),
});

const approvalResumeSchema = z.object({
  approved: z.boolean(),
  decisionId: z.string(),
  reviewerNote: z.string().optional(),
});

const deliveryOutputSchema = z.object({
  runId: z.string(),
  status: z.enum(['completed', 'rejected']),
  decisionId: z.string().optional(),
  summary: z.string(),
});

const prepareProposalStep = createStep({
  id: 'prepare-proposal',
  description: 'Assemble creative production proposal and cost estimate',
  inputSchema: proposalInputSchema,
  outputSchema: proposalOutputSchema,
  execute: async ({ inputData }) => ({
    runId: inputData.runId,
    projectId: inputData.projectId,
    proposalSummary: `Production proposal for: ${inputData.goal}`,
    estimatedCostMicros: inputData.estimatedCostMicros,
    requiresApproval: inputData.requiresApproval,
  }),
});

const approvalGateStep = createStep({
  id: 'approval-gate',
  description: 'Suspend workflow for creator approval before billable execution',
  inputSchema: proposalOutputSchema,
  outputSchema: deliveryOutputSchema,
  suspendSchema: approvalSuspendSchema,
  resumeSchema: approvalResumeSchema,
  execute: async ({ inputData, resumeData, suspend }) => {
    if (!inputData.requiresApproval) {
      return {
        runId: inputData.runId,
        status: 'completed' as const,
        summary: `Auto-executed: ${inputData.proposalSummary}`,
      };
    }

    if (!resumeData) {
      return await suspend({
        runId: inputData.runId,
        proposalSummary: inputData.proposalSummary,
        estimatedCostMicros: inputData.estimatedCostMicros,
        reason: 'creator_approval_required',
      });
    }

    if (!resumeData.approved) {
      return {
        runId: inputData.runId,
        status: 'rejected' as const,
        decisionId: resumeData.decisionId,
        summary: resumeData.reviewerNote ?? 'Rejected by creator',
      };
    }

    return {
      runId: inputData.runId,
      status: 'completed' as const,
      decisionId: resumeData.decisionId,
      summary: `Approved and executed: ${inputData.proposalSummary}`,
    };
  },
});

export const creativeDeliveryWorkflow = createWorkflow({
  id: 'creative-delivery-workflow',
  description:
    'Multi-stage creative delivery workflow with durable HITL approval suspend/resume',
  inputSchema: proposalInputSchema,
  outputSchema: deliveryOutputSchema,
})
  .then(prepareProposalStep)
  .then(approvalGateStep)
  .commit();
