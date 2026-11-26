import type { McpToolLike } from '../src/business/canvas-bridge.js';

export interface SimCanvasNode {
  id: string;
  type: string;
  title: string;
  content: string;
  boundTaskId?: string;
}

export interface SimMediaTask {
  id: string;
  operationId: string;
  nodeId: string;
  status: 'running' | 'completed' | 'failed';
  pollsRemaining: number;
  outputUrl?: string;
}

/**
 * In-memory Go MCP business authority simulator (inspired by BeefTV's sim-go)
 * for zero-cost, zero-HTTP deterministic offline outcome evaluations.
 */
export class SimBusinessAuthority {
  revision = 1;
  readonly nodes = new Map<string, SimCanvasNode>();
  readonly tasksByOpId = new Map<string, SimMediaTask>();
  readonly tasksById = new Map<string, SimMediaTask>();
  readonly opLog: Array<{
    tool: string;
    revision: number;
    operationId?: string;
  }> = [];
  private taskCounter = 0;

  seedNodes(initialNodes: SimCanvasNode[]): void {
    for (const node of initialNodes) {
      this.nodes.set(node.id, { ...node });
    }
  }

  paidTaskCount(): number {
    return this.tasksById.size;
  }

  asMcpToolset(): Record<string, McpToolLike> {
    return {
      canvas_read_graph: {
        execute: async () => {
          this.opLog.push({
            tool: 'canvas_read_graph',
            revision: this.revision,
          });
          return {
            revision: this.revision,
            nodes: [...this.nodes.values()],
            edges: [],
          };
        },
      },
      canvas_node_update: {
        execute: async (input) => {
          const expected = Number(input.expectedRevision);
          if (expected !== this.revision) {
            throw new Error(
              `stale_revision: expected ${expected}, actual ${this.revision}`,
            );
          }
          const nodeId = String(input.nodeId);
          const existing = this.nodes.get(nodeId);
          if (!existing) {
            throw new Error(`node_not_found: ${nodeId}`);
          }
          if (typeof input.title === 'string') existing.title = input.title;
          if (typeof input.content === 'string')
            existing.content = input.content;
          this.revision += 1;
          this.opLog.push({
            tool: 'canvas_node_update',
            revision: this.revision,
          });
          return { ok: true, revision: this.revision, node: { ...existing } };
        },
      },
      media_submit_image: {
        execute: async (input) => {
          const opId = String(input.operationId ?? '');
          const nodeId = String(input.nodeId ?? '');
          if (!opId) {
            throw new Error('operation_id_required');
          }
          const existing = this.tasksByOpId.get(opId);
          if (existing) {
            this.opLog.push({
              tool: 'media_submit_image:replay',
              revision: this.revision,
              operationId: opId,
            });
            return {
              status: existing.status,
              taskId: existing.id,
              taskIds: [existing.id],
            };
          }
          this.taskCounter += 1;
          const task: SimMediaTask = {
            id: `sim-task-${this.taskCounter}`,
            operationId: opId,
            nodeId,
            status: 'running',
            pollsRemaining: Number(input.pollsToComplete ?? 1),
          };
          this.tasksByOpId.set(opId, task);
          this.tasksById.set(task.id, task);
          this.opLog.push({
            tool: 'media_submit_image',
            revision: this.revision,
            operationId: opId,
          });
          return { status: 'running', taskId: task.id };
        },
      },
      task_wait: {
        execute: async (input) => {
          const ids = Array.isArray(input.taskIds)
            ? (input.taskIds as string[])
            : [];
          const tasks = ids.map((id) => {
            const task = this.tasksById.get(id);
            if (!task) return { id, status: 'failed' };
            if (task.status === 'running') {
              task.pollsRemaining -= 1;
              if (task.pollsRemaining <= 0) {
                task.status = 'completed';
                task.outputUrl = `cos://sim/${task.id}.png`;
                const node = this.nodes.get(task.nodeId);
                if (node) node.boundTaskId = task.id;
              }
            }
            return { ...task };
          });
          return { tasks };
        },
      },
    };
  }
}
