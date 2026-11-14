import { z } from 'zod';
import type { TaskContext } from '@/workflows/definition';
import type { Run } from '@/workflows/run';
import type { Proposal } from '@/server/store';

/** Runtime contracts validate untrusted HTTP JSON before it reaches UI state. */
export const documentContract = z.object({
  id: z.string(),
  title: z.string(),
  revision: z.string(),
  editable: z.boolean(),
});
export const documentsContract = z.array(documentContract);
export const liveDocumentContract = z.object({
  id: z.string(),
  source: z.string(),
  revision: z.string(),
});
export const statusContract = z.object({
  database: z.boolean(),
  bridge: z.boolean(),
  tts: z.boolean(),
  actor: z.string(),
  message: z.string().nullable(),
});
export const skillsContract = z.array(
  z.object({ name: z.string(), enabled: z.boolean(), sourcePath: z.string() }),
);
const contextContract = z.object({
  goal: z.string(),
  project: z.string(),
  repoCommit: z.string(),
  codeDigest: z.string(),
  documents: z.array(
    z.object({ id: z.string(), revision: z.string(), excerpt: z.string() }),
  ),
  skillRevisions: z.record(z.string(), z.string()),
  workflowRevision: z.string(),
  allowedActions: z.array(z.string()),
  expiresAt: z.number(),
  acceptance: z.array(z.string()),
  stageIds: z.array(z.string()),
  digest: z.string(),
}) satisfies z.ZodType<TaskContext>;
const runStatusContract = z.enum([
  'created',
  'ready',
  'running',
  'waiting_input',
  'waiting_external',
  'paused',
  'succeeded',
  'failed',
  'canceled',
  'unknown',
]);
export const runContract = z.object({
  id: z.string(),
  actor: z.string(),
  workflowId: z.string(),
  context: contextContract,
  status: runStatusContract,
  version: z.number(),
  attempt: z.number(),
  stage: z.number(),
  externalHandle: z.string().nullable(),
  acceptedBy: z.string().nullable(),
  reacceptRequired: z.boolean().optional(),
  pausedFrom: runStatusContract.optional(),
  pendingCheck: z
    .object({ key: z.string(), commandId: z.string() })
    .nullable()
    .optional(),
  evidence: z.array(
    z.object({
      kind: z.enum(['command', 'host', 'manual']),
      status: z.enum(['passed', 'failed', 'skipped', 'not_run']),
      inputDigest: z.string(),
      summary: z.string(),
      artifactRef: z.string(),
      stage: z.number().optional(),
    }),
  ),
}) satisfies z.ZodType<Run>;
export const runsContract = z.array(runContract);
export const proposalContract = z.object({
  id: z.string(),
  actor: z.string(),
  documentId: z.string(),
  baseRevision: z.string(),
  text: z.string(),
  baseText: z.string().optional(),
  receipt: z.record(z.string(), z.unknown()).nullable(),
}) satisfies z.ZodType<Proposal>;
export const proposalsContract = z.array(proposalContract);
export type Document = z.infer<typeof documentContract>;
export type Status = z.infer<typeof statusContract>;
export type HostSkill = z.infer<typeof skillsContract>[number];
