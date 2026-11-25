import { createStep, createWorkflow } from '@mastra/core/workflows';
import { z } from 'zod';

const key = z.string().min(1).max(100);
const nodeSchema = z
  .object({
    id: key,
    skill: z
      .string()
      .regex(/^[a-z0-9]+(-[a-z0-9]+)*$/)
      .max(64),
    title: z.string().trim().min(1).max(160),
    task: z.string().trim().min(1).max(16_000),
    after: z.array(key).max(64),
  })
  .strict();
export const canvasWorkflowSchema = z
  .object({
    id: key,
    nodes: z.array(nodeSchema).min(1).max(64),
    source: z
      .object({
        canvasId: z.uuid(),
        revision: z.string().regex(/^[0-9]+$/),
        graphHash: z.string().regex(/^[a-f0-9]{64}$/),
      })
      .strict()
      .optional(),
  })
  .strict()
  .superRefine((graph, ctx) => {
    try {
      orderCanvasNodes(graph);
    } catch (err) {
      ctx.addIssue({
        code: 'custom',
        message: err instanceof Error ? err.message : String(err),
      });
    }
  });
export type CanvasWorkflow = z.infer<typeof canvasWorkflowSchema>;
export type CanvasStage = z.infer<typeof nodeSchema>;

export function orderCanvasNodes(graph: CanvasWorkflow): CanvasStage[] {
  const nodes = new Map(graph.nodes.map((node) => [node.id, node]));
  if (nodes.size !== graph.nodes.length)
    throw new Error('Duplicate workflow node');
  const visiting = new Set<string>();
  const visited = new Set<string>();
  const ordered: CanvasStage[] = [];
  function visit(id: string): void {
    if (visited.has(id)) return;
    if (visiting.has(id)) throw new Error('Workflow contains a cycle');
    const node = nodes.get(id);
    if (!node) throw new Error(`Workflow dependency missing: ${id}`);
    if (new Set(node.after).size !== node.after.length)
      throw new Error('Duplicate workflow connection');
    visiting.add(id);
    node.after.forEach(visit);
    visiting.delete(id);
    visited.add(id);
    ordered.push(node);
  }
  graph.nodes.forEach((node) => visit(node.id));
  return ordered;
}

const resultSchema = z.object({
  nodeId: key,
  title: z.string(),
  skillDigest: z.string(),
  text: z.string(),
});
export type CanvasStageResult = z.infer<typeof resultSchema>;
const workflowInput = z.object({
  runId: z.uuid(),
  graph: canvasWorkflowSchema,
});
const workflowOutput = z.object({ results: z.array(resultSchema) });
const workflowState = z.object({ results: z.array(resultSchema).default([]) });

export type CanvasStageRunner = (
  stage: CanvasStage,
  upstream: CanvasStageResult[],
  signal: AbortSignal,
) => Promise<CanvasStageResult>;

export function createCanvasWorkflow(executeStage: CanvasStageRunner) {
  const processGraph = createStep({
    id: 'execute-canvas-graph',
    description: 'Execute published canvas Skill nodes in dependency order',
    stateSchema: workflowState,
    inputSchema: workflowInput,
    outputSchema: workflowOutput,
    execute: async ({ inputData, getState, setState, abortSignal }) => {
      const state = await getState();
      const outputs = new Map(
        (state.results ?? []).map((result) => [result.nodeId, result]),
      );
      for (const node of orderCanvasNodes(inputData.graph)) {
        abortSignal.throwIfAborted();
        if (outputs.has(node.id)) continue;
        const inputs = node.after.map((id) => {
          const result = outputs.get(id);
          if (!result) throw new Error(`Missing workflow result: ${id}`);
          return result;
        });
        outputs.set(node.id, await executeStage(node, inputs, abortSignal));
        await setState({ results: [...outputs.values()] });
      }
      return { results: [...outputs.values()] };
    },
  });
  return createWorkflow({
    id: 'canvas-skill-workflow',
    inputSchema: workflowInput,
    outputSchema: workflowOutput,
    stateSchema: workflowState,
  })
    .then(processGraph)
    .commit();
}
