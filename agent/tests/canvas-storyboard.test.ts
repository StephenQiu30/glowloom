import { randomUUID } from 'node:crypto';
import type { JsonObject } from '@earendil-works/pi-durable';
import { afterAll, expect, it } from 'vitest';
import {
  BusinessClient,
  type BusinessTool,
  type CallReceipt,
  type Grant,
} from '../src/business/client.js';
import { SessionService } from '../src/sessions/service.js';
import { canonical } from '../src/sessions/manifest.js';
import type { Scope, StoryboardTarget } from '../src/sessions/types.js';
import { authorizedText } from './helpers/conversation-input.js';
import { testPool } from './storage/database.js';

const pool = testPool();
afterAll(() => pool.end());

const read = 'canvas_storyboard_read';
const write = 'canvas_storyboard_write';
const policyMarker = 'Ordinary canvas storyboard';
const storyboardTools: BusinessTool[] = [
  {
    name: read,
    description: 'Read the current ordinary storyboard',
    effect: 'read',
    contract_version: 'v1',
    replay: 'safe',
    parameters: { type: 'object' },
  },
  {
    name: write,
    description: 'Commit the current ordinary storyboard',
    effect: 'write',
    contract_version: 'v1',
    replay: 'safe',
    parameters: { type: 'object' },
  },
];
const mediaTools: BusinessTool[] = [
  'operation_quote',
  'operation_propose',
  'operation_confirm',
].map((name) => ({
  name,
  description: 'Media operation fixture',
  effect: 'write',
  contract_version: 'v1',
  replay: 'safe',
  parameters: { type: 'object' },
}));

type ModelAnswer = string | { name: string; arguments: JsonObject };

class StoryboardBusiness extends BusinessClient {
  readonly modelInputs: unknown[] = [];
  readonly toolCalls: { name: string; input: unknown }[] = [];
  readonly toolResults: JsonObject[] = [];
  readonly answers: ModelAnswer[] = [];

  constructor(
    readonly scope: Scope,
    private readonly canvasKind: string | undefined,
    private readonly descriptors = storyboardTools,
  ) {
    super('http://127.0.0.1:1', 'fixture');
  }

  override async tools() {
    return this.descriptors;
  }
  override async decisions() {
    return {
      current: null,
      authorization_revision: 0,
      barrier: false,
      terminal: false,
    };
  }
  override async grant(
    input: Parameters<BusinessClient['grant']>[0],
  ): Promise<Grant> {
    return {
      ...input,
      grant_id: randomUUID(),
      available_micros: 1000000,
      expires_at: input.lease_until,
      scope: this.scope,
    };
  }
  override async validate(grant: Grant) {
    return grant;
  }
  override async snapshot() {
    if (!this.scope.canvas_id) return {};
    return {
      active_canvas: {
        id: this.scope.canvas_id,
        ...(this.canvasKind ? { kind: this.canvasKind } : {}),
      },
    };
  }
  override async findCall() {
    return undefined;
  }
  override async execute(
    ...args: Parameters<BusinessClient['execute']>
  ): Promise<CallReceipt> {
    const [kind, name, grant, call, input] = args;
    let result: unknown;
    if (kind === 'tool') {
      this.toolCalls.push({ name, input });
      result = this.toolResults.shift();
      if (!result) throw new Error('Unexpected fixture tool invocation');
    } else {
      this.modelInputs.push(input);
      const answer = this.answers.shift();
      if (answer === undefined)
        throw new Error('Unexpected fixture model call');
      result = {
        usage: { input_tokens: 1, output_tokens: 1 },
        model_key: 'fixture',
        ...(typeof answer === 'string'
          ? { output: answer, stop_reason: 'stop' }
          : {
              output: '',
              stop_reason: 'tool_use',
              tool_calls: [{ id: randomUUID(), ...answer }],
            }),
      };
    }
    return {
      receipt_id: randomUUID(),
      root_run_id: grant.root_run_id,
      submission_id: grant.submission_id,
      call,
      status: 'completed',
      result,
      cost_micros: 0,
    };
  }
}

async function useConversation(
  canvasId: string | undefined,
  canvasKind: string | undefined,
  exercise: (
    business: StoryboardBusiness,
    submit: (text: string, targets?: StoryboardTarget[]) => Promise<void>,
  ) => Promise<void>,
  descriptors = storyboardTools,
) {
  const scope: Scope = {
    actor_id: randomUUID(),
    org_id: randomUUID(),
    scope_id: randomUUID(),
    project_id: randomUUID(),
    ...(canvasId ? { canvas_id: canvasId } : {}),
  };
  const business = new StoryboardBusiness(scope, canvasKind, descriptors);
  const service = new SessionService(pool, business, { leaseMs: 30000 });
  try {
    const session = await service.createSession({
      ...scope,
      request_id: randomUUID(),
      content_hash: 'a'.repeat(64),
      title: '渡轮分镜',
    });
    await exercise(business, async (prompt, targets) => {
      const current = await service.readSession(session.id, scope);
      if (!current) throw new Error('Missing fixture session');
      const input = authorizedText(prompt);
      const run = await service.createRun(session.id, {
        ...scope,
        request_id: randomUUID(),
        content_hash: input.input_hash,
        expected_execution_revision: current.execution_revision,
        goal: prompt,
        manifest: { model_key: 'fixture' },
        input: {
          ...input,
          ...(targets ? { storyboard_targets: targets } : {}),
        },
      });
      await service.waitForRun(run.root_run_id);
      expect(
        (await service.readRun(run.root_run_id, scope))?.state.status,
      ).toBe('completed');
    });
  } finally {
    await service.close();
  }
}

it('injects the ordinary storyboard rules into Pi and carries a freely combined direction into a tool-driven draft', async () => {
  const canvasId = randomUUID();
  await useConversation(canvasId, undefined, async (business, submit) => {
    const directions =
      '乘客观察强调沉浸，也可以拍船员交班的节奏。推荐乘客观察。';
    business.answers.push(directions, '四镜每镜五秒共二十秒，应保留哪个时长？');
    await submit('想拍清晨第一班渡轮');
    await submit('只讨论，四镜每镜五秒，正好十五秒，哪里有矛盾？');
    expect(business.toolCalls).toEqual([]);
    expect(business.modelInputs[0]).toMatchObject({
      messages: expect.arrayContaining([
        expect.objectContaining({
          role: 'system',
          content: expect.stringContaining(`${policyMarker} ${canvasId}`),
        }),
      ]),
      tools: expect.arrayContaining(
        [{ name: read }, { name: write }].map((item) =>
          expect.objectContaining(item),
        ),
      ),
    });
    const shots = [
      { title: '登船', action: '乘客推开挡风门', duration_ms: 4000 },
      { title: '离岸', action: '透过舷窗看船员解缆', duration_ms: 5000 },
      { title: '过江', action: '贴近乘客目光迎来天光', duration_ms: 6000 },
    ];
    const create = { canvas_id: canvasId, expected_revision: 4, shots };
    const saved = shots.map((shot) => ({
      ...shot,
      id: randomUUID(),
      revision: 1,
    }));
    const board = { id: randomUUID(), shots: saved };
    business.answers.push(
      { name: read, arguments: { canvas_id: canvasId } },
      { name: write, arguments: create },
      '已保存三镜，共十五秒。',
    );
    business.toolResults.push(
      {
        canvas_id: canvasId,
        project_id: business.scope.project_id,
        revision: 4,
      },
      {
        canvas_id: canvasId,
        project_id: business.scope.project_id,
        revision: 5,
        board,
      },
    );
    const selection =
      '用乘客观察的视角，结合船员解缆的节奏，十五秒三镜，请分镜。';
    await submit(selection);
    const transcript = JSON.stringify(business.modelInputs.slice(2));
    expect(transcript).toContain(directions);
    expect(transcript).toContain(selection);
    expect(transcript).toContain(board.id);
    expect(business.toolCalls).toEqual([
      { name: read, input: { canvas_id: canvasId } },
      { name: write, input: create },
    ]);
  });
});

it('routes a clear first request directly to a complete board and a follow-up to a stable shot after reordering', async () => {
  const canvasId = randomUUID();
  const shots = [
    { title: '登船', action: '乘客踏上甲板', duration_ms: 4000 },
    { title: '离岸', action: '靠近乘客脚步', duration_ms: 5000 },
    { title: '迎光', action: '透过舷窗看日出', duration_ms: 6000 },
  ];
  const saved = shots.map((shot) => ({
    ...shot,
    id: randomUUID(),
    revision: 1,
  }));
  const boardId = randomUUID();
  const second = saved[1];
  if (!second) throw new Error('Missing fixture shot');
  const target = {
    ordinal: 2,
    board_id: boardId,
    shot_id: second.id,
    shot_revision: second.revision,
  };
  const create = { canvas_id: canvasId, expected_revision: 4, shots };
  const patch = {
    canvas_id: canvasId,
    board_id: boardId,
    expected_revision: 6,
    edit: {
      shot_id: second.id,
      expected_revision: second.revision,
      field: 'action',
      value: '乘客握住湿冷的栏杆',
    },
  };
  await useConversation(
    canvasId,
    'canvas',
    async (business, submit) => {
      const readArgs = { canvas_id: canvasId };
      const canvas = { ...readArgs, project_id: business.scope.project_id };
      business.answers.push(
        { name: read, arguments: readArgs },
        { name: write, arguments: create },
        '已保存三镜，第二镜拍乘客脚步。',
        { name: read, arguments: readArgs },
        { name: write, arguments: patch },
        '已改为拍乘客握住栏杆。',
        { name: read, arguments: readArgs },
        '镜头动作已变化，请确认是否继续修改现在的动作。',
      );
      const reordered = [second, ...saved.filter((shot) => shot !== second)];
      const edited = {
        ...canvas,
        revision: 7,
        board: {
          id: boardId,
          shots: reordered.map((shot) =>
            shot.id === second.id
              ? { ...shot, action: patch.edit.value, revision: 2 }
              : shot,
          ),
        },
      };
      business.toolResults.push(
        { ...canvas, revision: 4 },
        { ...canvas, revision: 5, board: { id: boardId, shots: saved } },
        { ...canvas, revision: 6, board: { id: boardId, shots: reordered } },
        edited,
        edited,
      );
      await submit('乘客视角、十五秒、三镜的渡轮短片，请直接分镜。');
      await submit('刚才第二镜别拍脚步，改拍握住栏杆的手。', [target]);
      await submit('把之前的第二镜改成拉响汽笛。', [target]);
      expect(business.modelInputs[0]).toMatchObject({
        messages: expect.arrayContaining([
          expect.objectContaining({
            role: 'system',
            content: expect.stringContaining(
              'confirms that exact current quote',
            ),
          }),
        ]),
      });
      expect(business.modelInputs.at(-1)).toMatchObject({
        messages: expect.arrayContaining([
          expect.objectContaining({
            role: 'system',
            content: expect.stringContaining(canonical([target])),
          }),
          expect.objectContaining({
            role: 'system',
            content: expect.stringContaining(
              'wait for the user to confirm the current target',
            ),
          }),
        ]),
      });
      expect(business.toolCalls).toEqual([
        { name: read, input: readArgs },
        { name: write, input: create },
        { name: read, input: readArgs },
        { name: write, input: patch },
        { name: read, input: readArgs },
      ]);
      expect(JSON.stringify(business.modelInputs.at(-1))).toContain(
        patch.edit.value,
      );
    },
    [...storyboardTools, ...mediaTools],
  );
});

it.each([
  ['short-drama canvas', randomUUID(), 'short_drama', storyboardTools],
  ['project chat', undefined, 'canvas', storyboardTools],
  ['missing write tool', randomUUID(), 'canvas', storyboardTools.slice(0, 1)],
] as const)(
  'leaves the prompt unchanged for %s',
  async (_, canvasId, kind, tools) => {
    await useConversation(
      canvasId,
      kind,
      async (business, submit) => {
        business.answers.push('已分析');
        await submit('讨论创意');
        expect(business.modelInputs).toHaveLength(1);
        expect(JSON.stringify(business.modelInputs[0])).not.toContain(
          policyMarker,
        );
      },
      tools,
    );
  },
);
