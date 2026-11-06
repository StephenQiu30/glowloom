import { afterAll, expect, it } from 'vitest';
import { randomUUID } from 'node:crypto';
import { SessionService } from '../src/sessions/service.js';
import {
  BusinessClient,
  type CallReceipt,
  type Grant,
} from '../src/business/client.js';
import { testPool } from './storage/database.js';
const scope = {
  actor_id: randomUUID(),
  org_id: randomUUID(),
  scope_id: randomUUID(),
  project_id: randomUUID(),
};
const asset = randomUUID();
const png =
  'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+aXioAAAAASUVORK5CYII=';
class ImageBusiness extends BusinessClient {
  sourceChecks = 0;
  revoked = false;
  async validateInputSources() {
    this.sourceChecks++;
    if (this.revoked) throw new Error('conversation_reference_unavailable');
  }
  modelInput: unknown;
  reads = 0;
  constructor() {
    super('http://127.0.0.1:1', 'offline-fixture');
  }
  override async tools() {
    return [];
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
      scope,
    };
  }
  override async validate(grant: Grant) {
    return grant;
  }
  override async snapshot() {
    return { revision: 1 };
  }
  override async findCall() {
    return undefined;
  }
  override async execute(
    ...args: Parameters<BusinessClient['execute']>
  ): Promise<CallReceipt> {
    const [kind, , grant, call, input] = args;
    let result: unknown;
    if (kind === 'tool') {
      this.reads++;
      result = {
        asset_id: asset,
        revision: 2,
        content: [{ type: 'image', mimeType: 'image/png', data: png }],
      };
    } else {
      this.modelInput = input;
      result = {
        output: '已读取参考图片',
        usage: { input_tokens: 4, output_tokens: 5 },
        stop_reason: 'stop',
        model_key: 'fixture',
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
const pool = testPool();
afterAll(() => pool.end());
it('passes real image content through the official Harness transcript and retains only reference identities in the accepted input', async () => {
  const business = new ImageBusiness();
  const service = new SessionService(pool, business, { leaseMs: 30000 });
  try {
    const session = await service.createSession({
      ...scope,
      request_id: randomUUID(),
      content_hash: 'a'.repeat(64),
      title: '图片',
    });
    const run = await service.createRun(session.id, {
      ...scope,
      request_id: randomUUID(),
      content_hash: 'b'.repeat(64),
      expected_execution_revision: 0,
      goal: 'reference input',
      manifest: { model_key: 'fixture' },
      input: {
        schema_version: 1,
        draft: {},
        input_hash: 'c'.repeat(64),
        plain_text: '',
        prompt: '',
        media_pins: [
          { sourceId: asset, sourceVersion: '2', mediaKind: 'image' },
        ],
      },
    });
    await service.waitForRun(run.root_run_id);
    const snapshot = await service.readRun(run.root_run_id, scope);
    expect(snapshot?.state.status).toBe('completed');
    expect(business.reads).toBe(1);
    expect(JSON.stringify(business.modelInput)).toContain(
      `data:image/png;base64,${png}`,
    );
    expect(snapshot?.input?.plain_text).toBe('');
    expect(JSON.stringify(snapshot?.input)).not.toContain(png);
    expect(
      snapshot?.messages_page.items.map((part) => part.content).join(''),
    ).toContain('已读取参考图片');
  } finally {
    await service.close();
  }
});
it('revalidates old image sources before a later message can reuse the transcript', async () => {
  const business = new ImageBusiness(),
    service = new SessionService(pool, business, { leaseMs: 30000 });
  try {
    const session = await service.createSession({
      ...scope,
      request_id: randomUUID(),
      content_hash: 'd'.repeat(64),
      title: '权限',
    });
    const first = await service.createRun(session.id, {
      ...scope,
      request_id: randomUUID(),
      content_hash: 'e'.repeat(64),
      expected_execution_revision: 0,
      goal: 'reference input',
      manifest: { model_key: 'fixture' },
      input: {
        schema_version: 1,
        draft: { references: [{ id: 'fixture-reference' }] },
        input_hash: 'f'.repeat(64),
        plain_text: '',
        prompt: '',
        media_pins: [
          { sourceId: asset, sourceVersion: '2', mediaKind: 'image' },
        ],
      },
    });
    await service.waitForRun(first.root_run_id);
    expect(
      (await service.readRun(first.root_run_id, scope))?.state.status,
    ).toBe('completed');
    const seen = business.modelInput;
    business.revoked = true;
    const previous = await service.readRun(first.root_run_id, scope);
    const next = await service.createRun(session.id, {
      ...scope,
      request_id: randomUUID(),
      content_hash: '1'.repeat(64),
      expected_execution_revision: previous!.state.execution_revision,
      goal: '继续',
      manifest: { model_key: 'fixture' },
      input: {
        schema_version: 1,
        draft: { references: [] },
        input_hash: '2'.repeat(64),
        plain_text: '继续',
        prompt: '继续',
        media_pins: [],
      },
    });
    await service.waitForRun(next.root_run_id);
    expect(business.sourceChecks).toBeGreaterThan(1);
    expect(business.modelInput).toBe(seen);
    expect((await service.readRun(next.root_run_id, scope))?.state.status).toBe(
      'failed',
    );
  } finally {
    await service.close();
  }
});
