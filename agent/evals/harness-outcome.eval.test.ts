import { describe, expect, it } from 'vitest';
import {
  MAX_CANVAS_VIEW_BYTES,
  wrapMcpToolsetWithHarness,
} from '../src/business/canvas-bridge.js';
import { SimBusinessAuthority } from './sim-business.js';

describe('Offline Harness Outcome & Efficiency Evals (Mock MCP Authority)', () => {
  it('L1: locates and updates a single target node inside a 300-node canvas within <40KB read budget', async () => {
    const sim = new SimBusinessAuthority();
    sim.seedNodes(
      Array.from({ length: 300 }, (_, idx) => ({
        id: `n_${idx + 1}`,
        type: 'text',
        title: `备注 ${idx + 1}`,
        content: `原始内容 ${idx + 1}`,
      })),
    );

    const tools = wrapMcpToolsetWithHarness(sim.asMcpToolset(), {
      runId: 'eval-l1',
    });

    const view = (await tools.canvas_read_graph!.execute!({})) as {
      revision: number;
      nodeIndex: Array<{ id: string; title: string }>;
    };
    expect(Buffer.byteLength(JSON.stringify(view), 'utf8')).toBeLessThan(
      MAX_CANVAS_VIEW_BYTES,
    );

    const target = view.nodeIndex.find((item) => item.title === '备注 137');
    expect(target?.id).toBe('n_137');

    await tools.canvas_node_update!.execute!({
      nodeId: target!.id,
      expectedRevision: view.revision,
      content: '已按导演意见更新',
    });

    expect(sim.nodes.get('n_137')?.content).toBe('已按导演意见更新');
    expect(sim.nodes.get('n_136')?.content).toBe('原始内容 136');
    expect(sim.opLog.length).toBeLessThanOrEqual(2);
  });

  it('R7: applies 4 same-turn edits without losing updates via local write-chain rebasing', async () => {
    const sim = new SimBusinessAuthority();
    sim.seedNodes([
      { id: 's1', type: 'text', title: '镜1', content: '旧1' },
      { id: 's2', type: 'text', title: '镜2', content: '旧2' },
      { id: 's3', type: 'text', title: '镜3', content: '旧3' },
      { id: 's4', type: 'text', title: '镜4', content: '旧4' },
    ]);

    const tools = wrapMcpToolsetWithHarness(sim.asMcpToolset(), {
      runId: 'eval-r7',
    });

    const view = (await tools.canvas_read_graph!.execute!({})) as {
      revision: number;
    };
    for (const id of ['s1', 's2', 's3', 's4']) {
      await tools.canvas_node_update!.execute!({
        nodeId: id,
        expectedRevision: view.revision,
        content: `新-${id}`,
      });
    }

    for (const id of ['s1', 's2', 's3', 's4']) {
      expect(sim.nodes.get(id)?.content).toBe(`新-${id}`);
    }
    expect(sim.revision).toBe(5);
  });

  it('R1/R3: deterministically awaits async image generation and prevents duplicate paid charges on crash replay', async () => {
    const sim = new SimBusinessAuthority();
    sim.seedNodes([
      { id: 'shot_1', type: 'image', title: '镜1首帧', content: '' },
    ]);

    const runOnce = () => {
      const tools = wrapMcpToolsetWithHarness(sim.asMcpToolset(), {
        runId: 'eval-r1-replay',
        maxTaskWaitMs: 5_000,
        waitSliceMs: 50,
      });
      return tools.media_submit_image!.execute!({
        nodeId: 'shot_1',
        prompt: 'cyberpunk alley close-up',
      });
    };

    const first = (await runOnce()) as { status: string; taskIds: string[] };
    const replayed = (await runOnce()) as { status: string; taskIds: string[] };

    expect(first.status).toBe('completed');
    expect(replayed.status).toBe('completed');
    expect(replayed.taskIds).toEqual(first.taskIds);
    expect(sim.paidTaskCount()).toBe(1);
    expect(sim.nodes.get('shot_1')?.boundTaskId).toBe(first.taskIds[0]);
  });
});
