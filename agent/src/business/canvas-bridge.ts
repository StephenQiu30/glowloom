import { createHash } from 'node:crypto';
import type { BudgetReservation } from './model-middleware.js';

export const MAX_CANVAS_VIEW_BYTES = 39_000;
const MAX_INLINE_FIELD_BYTES = 1_500;
const DEFAULT_WAIT_SLICE_MS = 25_000;
const DEFAULT_MAX_WAIT_MS = 120_000;

export const REBASABLE_CANVAS_WRITES = new Set([
  'canvas_upsert_storyboard_nodes',
  'canvas_node_update',
  'canvas_node_move',
  'canvas_nodes_create',
  'canvas_edge_create',
  'canvas_node_configure',
]);

const NON_REBASABLE_CANVAS_WRITES = new Set(['canvas_document_commit']);

const BILLABLE_TOOLS = new Set([
  'media_submit_image',
  'media_submit_video_i2v',
  'media_submit_tts',
  'media_submit_lipsync',
  'canvas_split_grid_2x2',
  'timeline_export_render',
]);

export class ToolBudgetExhaustedError extends Error {
  readonly code = 'tool_step_budget_exhausted';

  constructor(
    readonly used: number,
    readonly max: number,
  ) {
    super(`Tool step budget exhausted (${used}/${max})`);
    this.name = 'ToolBudgetExhaustedError';
  }
}

export class CanvasConflictCircuitOpenError extends Error {
  readonly code = 'canvas_context_changed';

  constructor() {
    super(
      'canvas_context_changed: 画布已被外部修改，同批剩余写入已暂停，请重新读取最新画布后再决策',
    );
    this.name = 'CanvasConflictCircuitOpenError';
  }
}

export interface McpToolLike {
  execute?: (
    input: Record<string, unknown>,
    context?: unknown,
  ) => Promise<unknown>;
  [key: string]: unknown;
}

export interface TaskProgressUpdate {
  taskIds: string[];
  completed: number;
  total: number;
  status: 'running' | 'completed' | 'failed' | 'yielded_pending';
}

export interface CanvasBridgeOptions {
  runId: string;
  reserveToolStep?: () => Promise<BudgetReservation>;
  onTaskProgress?: (update: TaskProgressUpdate) => Promise<void> | void;
  maxTaskWaitMs?: number;
  waitSliceMs?: number;
  signal?: AbortSignal;
}

type UnknownRecord = Record<string, unknown>;

function asRecord(value: unknown): UnknownRecord | undefined {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
    ? (value as UnknownRecord)
    : undefined;
}

function canonicalToolName(name: string): string {
  return name.replace(/^glowloom_/, '');
}

export function deterministicOperationId(
  runId: string,
  toolName: string,
  args: Record<string, unknown>,
): string {
  const stableEntries = Object.entries(args)
    .filter(([key]) => key !== 'operationId' && key !== 'operation_id')
    .sort(([a], [b]) => a.localeCompare(b));
  const digest = createHash('sha256')
    .update(runId)
    .update(':')
    .update(toolName)
    .update(':')
    .update(JSON.stringify(stableEntries))
    .digest('hex')
    .slice(0, 32);
  return `op_${digest}`;
}

function sanitizeNodeField(
  nodeId: string,
  fieldPath: string,
  value: unknown,
): { value: unknown; complete: boolean } {
  const serialized = JSON.stringify(value);
  if (
    serialized === undefined ||
    Buffer.byteLength(serialized, 'utf8') <= MAX_INLINE_FIELD_BYTES
  ) {
    return { value, complete: true };
  }

  if (typeof value === 'string') {
    return {
      value: {
        preview: value.slice(0, 400),
        totalChars: value.length,
        complete: false,
        readRef: { nodeId, fieldPath },
      },
      complete: false,
    };
  }

  return {
    value: {
      summaryType: Array.isArray(value) ? 'array' : typeof value,
      byteLength: Buffer.byteLength(serialized, 'utf8'),
      complete: false,
      readRef: { nodeId, fieldPath },
    },
    complete: false,
  };
}

/**
 * Projects a raw canvas graph payload into a bounded (< 40KB UTF-8)
 * model-facing view with explicit `complete: false` drilldown references.
 */
export function projectCanvasReadView(
  rawCanvas: unknown,
  maxBytes = MAX_CANVAS_VIEW_BYTES,
): UnknownRecord {
  const record = asRecord(rawCanvas);
  if (!record) {
    return { complete: true, data: rawCanvas };
  }

  const revision =
    typeof record.revision === 'number'
      ? record.revision
      : typeof record.version === 'number'
        ? record.version
        : 0;
  const rawNodes = Array.isArray(record.nodes) ? record.nodes : [];
  const rawEdges = Array.isArray(record.edges)
    ? record.edges
    : Array.isArray(record.connections)
      ? record.connections
      : [];

  const nodeIndex = rawNodes.slice(0, 500).map((item, idx) => {
    const node = asRecord(item);
    return {
      id: typeof node?.id === 'string' ? node.id : `node_${idx}`,
      type: typeof node?.type === 'string' ? node.type : 'unknown',
      title: typeof node?.title === 'string' ? node.title.slice(0, 80) : '',
    };
  });

  const nodesPage: UnknownRecord[] = [];
  let fieldsComplete = true;
  let recordsComplete = true;

  const baseOverhead = Buffer.byteLength(
    JSON.stringify({
      revision,
      totalNodes: rawNodes.length,
      totalEdges: rawEdges.length,
      nodeIndex,
      nodes: [],
      edges: [],
      recordsComplete: true,
      complete: true,
    }),
    'utf8',
  );
  let usedBytes = baseOverhead;

  for (let i = 0; i < rawNodes.length; i += 1) {
    const node = asRecord(rawNodes[i]);
    if (!node) continue;
    const nodeId = typeof node.id === 'string' ? node.id : `node_${i}`;
    const projectedNode: UnknownRecord = {};
    let nodeFieldsComplete = true;

    for (const [key, val] of Object.entries(node)) {
      const sanitized = sanitizeNodeField(nodeId, `/${key}`, val);
      projectedNode[key] = sanitized.value;
      if (!sanitized.complete) {
        nodeFieldsComplete = false;
        fieldsComplete = false;
      }
    }
    if (!nodeFieldsComplete) {
      projectedNode.complete = false;
    }

    const nodeBytes = Buffer.byteLength(JSON.stringify(projectedNode), 'utf8');
    if (usedBytes + nodeBytes > maxBytes - 4_000) {
      recordsComplete = false;
      break;
    }
    nodesPage.push(projectedNode);
    usedBytes += nodeBytes;
  }

  const edgesPage: unknown[] = [];
  let edgesComplete = true;
  for (const edge of rawEdges) {
    const edgeBytes = Buffer.byteLength(JSON.stringify(edge), 'utf8');
    if (usedBytes + edgeBytes > maxBytes) {
      edgesComplete = false;
      break;
    }
    edgesPage.push(edge);
    usedBytes += edgeBytes;
  }

  return {
    revision,
    totalNodes: rawNodes.length,
    totalEdges: rawEdges.length,
    nodeIndex,
    nodes: nodesPage,
    edges: edgesPage,
    recordsComplete,
    edgesComplete,
    complete: recordsComplete && edgesComplete && fieldsComplete,
    ...(recordsComplete ? {} : { nextOffset: nodesPage.length }),
  };
}

function isCanvasConflictError(error: unknown): boolean {
  const message = error instanceof Error ? error.message : String(error);
  return /stale_revision|canvas_conflict|revision_mismatch|optimistic_lock/i.test(
    message,
  );
}

function extractRevision(payload: unknown): number | undefined {
  const record = asRecord(payload);
  if (!record) return undefined;
  if (typeof record.revision === 'number') return record.revision;
  if (typeof record.newRevision === 'number') return record.newRevision;
  if (typeof record.version === 'number') return record.version;
  return undefined;
}

function extractPendingTaskIds(payload: unknown): string[] {
  const record = asRecord(payload);
  if (!record) return [];
  const status =
    typeof record.status === 'string' ? record.status.toLowerCase() : '';
  if (status && !['queued', 'pending', 'running'].includes(status)) {
    return [];
  }
  if (Array.isArray(record.taskIds)) {
    return record.taskIds.filter((id): id is string => typeof id === 'string');
  }
  if (typeof record.taskId === 'string') {
    return [record.taskId];
  }
  if (typeof record.task_id === 'string') {
    return [record.task_id];
  }
  return [];
}

function isTerminalTask(task: UnknownRecord): {
  done: boolean;
  failed: boolean;
} {
  const status = String(task.status ?? task.state ?? '').toLowerCase();
  if (status === 'completed' || status === 'succeeded' || status === 'ready') {
    return { done: true, failed: false };
  }
  if (status === 'failed' || status === 'cancelled' || status === 'canceled') {
    return { done: true, failed: true };
  }
  return { done: false, failed: false };
}

/**
 * Wraps an `@mastra/mcp` toolset in memory (zero hand-written HTTP/fetch):
 * - Enforces pre-dispatch durable tool-step budget
 * - Projects large canvas reads under 40KB UTF-8 with explicit `complete: false`
 * - Rebases same-turn local canvas writes onto the latest known revision and
 *   trips a circuit breaker on external concurrent conflicts
 * - Injects deterministic `operationId` on billable media submissions
 * - Deterministically awaits async media tasks via MCP `task_wait` /
 *   `media_get_task` with progress events and graceful Yield on timeout
 */
export function wrapMcpToolsetWithHarness<T extends Record<string, unknown>>(
  rawTools: T,
  options: CanvasBridgeOptions,
): T {
  let knownRevision: number | undefined;
  let externalConflict = false;

  const entries = Object.entries(rawTools) as Array<[string, McpToolLike]>;

  const taskWaitTool = entries.find(([name]) => {
    const canon = canonicalToolName(name);
    return canon === 'task_wait' || canon === 'media_wait_tasks';
  })?.[1];

  const taskGetTool = entries.find(([name]) => {
    const canon = canonicalToolName(name);
    return canon === 'media_get_task' || canon === 'task_get';
  })?.[1];

  async function awaitSubmittedTasks(
    taskIds: string[],
    initialResult: unknown,
  ): Promise<unknown> {
    if (taskIds.length === 0) return initialResult;
    if (!taskWaitTool?.execute && !taskGetTool?.execute) {
      return initialResult;
    }

    const maxWaitMs = options.maxTaskWaitMs ?? DEFAULT_MAX_WAIT_MS;
    const sliceMs = Math.min(
      DEFAULT_WAIT_SLICE_MS,
      options.waitSliceMs ?? DEFAULT_WAIT_SLICE_MS,
    );
    const deadline = Date.now() + maxWaitMs;
    const observed = new Map<string, UnknownRecord>();

    while (Date.now() < deadline) {
      options.signal?.throwIfAborted();
      let batchTasks: UnknownRecord[] = [];

      if (taskWaitTool?.execute) {
        const remainingMs = Math.max(100, deadline - Date.now());
        const waitResponse = await taskWaitTool.execute({
          taskIds,
          timeoutMs: Math.min(sliceMs, remainingMs),
        });
        const rec = asRecord(waitResponse);
        if (Array.isArray(rec?.tasks)) {
          batchTasks = rec.tasks
            .map(asRecord)
            .filter((item): item is UnknownRecord => Boolean(item));
        }
      } else if (taskGetTool?.execute) {
        const fetched = await Promise.all(
          taskIds.map((taskId) => taskGetTool.execute!({ taskId })),
        );
        batchTasks = fetched
          .map(asRecord)
          .filter((item): item is UnknownRecord => Boolean(item));
      }

      for (const task of batchTasks) {
        const id =
          typeof task.id === 'string'
            ? task.id
            : typeof task.taskId === 'string'
              ? task.taskId
              : typeof task.task_id === 'string'
                ? task.task_id
                : undefined;
        if (id) observed.set(id, task);
      }

      let completed = 0;
      let failed = 0;
      for (const id of taskIds) {
        const task = observed.get(id);
        if (!task) continue;
        const state = isTerminalTask(task);
        if (state.done) {
          if (state.failed) failed += 1;
          else completed += 1;
        }
      }

      await options.onTaskProgress?.({
        taskIds,
        completed,
        total: taskIds.length,
        status:
          failed > 0
            ? 'failed'
            : completed === taskIds.length
              ? 'completed'
              : 'running',
      });

      if (completed + failed === taskIds.length) {
        return {
          status: failed > 0 ? 'failed' : 'completed',
          taskIds,
          tasks: taskIds.map((id) => observed.get(id)),
        };
      }

      if (!taskWaitTool?.execute) {
        await new Promise((resolve) => setTimeout(resolve, 1_000));
      }
    }

    await options.onTaskProgress?.({
      taskIds,
      completed: [...observed.values()].filter(
        (task) => isTerminalTask(task).done && !isTerminalTask(task).failed,
      ).length,
      total: taskIds.length,
      status: 'yielded_pending',
    });

    return {
      status: 'yielded_pending',
      taskIds,
      tasks: taskIds.map((id) => observed.get(id) ?? { id, status: 'running' }),
      message:
        '异步媒体任务仍在后台生成中，已安全让出本轮回合，请待任务完成后继续。',
    };
  }

  const wrapped: Record<string, unknown> = {};

  for (const [toolName, tool] of entries) {
    if (typeof tool?.execute !== 'function') {
      wrapped[toolName] = tool;
      continue;
    }

    const canon = canonicalToolName(toolName);
    const originalExecute = tool.execute.bind(tool);
    const isCanvasRead =
      canon === 'canvas_read_graph' || canon === 'canvas_get';
    const isCanvasWrite =
      REBASABLE_CANVAS_WRITES.has(canon) ||
      NON_REBASABLE_CANVAS_WRITES.has(canon);
    const isBillable = BILLABLE_TOOLS.has(canon);

    wrapped[toolName] = {
      ...tool,
      execute: async (input: Record<string, unknown>, context?: unknown) => {
        if (options.reserveToolStep) {
          const reservation = await options.reserveToolStep();
          if (!reservation.allowed) {
            throw new ToolBudgetExhaustedError(
              reservation.used,
              reservation.max,
            );
          }
        }

        if (isCanvasWrite && externalConflict) {
          throw new CanvasConflictCircuitOpenError();
        }

        let effectiveInput = { ...input };

        if (
          REBASABLE_CANVAS_WRITES.has(canon) &&
          knownRevision !== undefined &&
          typeof effectiveInput.expectedRevision === 'number' &&
          effectiveInput.expectedRevision < knownRevision
        ) {
          effectiveInput.expectedRevision = knownRevision;
        }

        if (
          isBillable &&
          typeof effectiveInput.operationId !== 'string' &&
          typeof effectiveInput.operation_id !== 'string'
        ) {
          effectiveInput = {
            ...effectiveInput,
            operationId: deterministicOperationId(
              options.runId,
              canon,
              effectiveInput,
            ),
          };
        }

        let result: unknown;
        try {
          result = await originalExecute(effectiveInput, context);
        } catch (error: unknown) {
          if (isCanvasWrite && isCanvasConflictError(error)) {
            externalConflict = true;
          }
          throw error;
        }

        const nextRevision = extractRevision(result);
        if (nextRevision !== undefined) {
          knownRevision = nextRevision;
        }

        if (isCanvasRead) {
          externalConflict = false;
          return projectCanvasReadView(result);
        }

        if (isBillable) {
          const pendingTaskIds = extractPendingTaskIds(result);
          if (pendingTaskIds.length > 0) {
            return awaitSubmittedTasks(pendingTaskIds, result);
          }
        }

        return result;
      },
    };
  }

  return wrapped as T;
}
