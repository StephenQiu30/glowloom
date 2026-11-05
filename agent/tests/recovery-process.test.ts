import { authorizedText } from './helpers/conversation-input.js';
import { randomUUID } from 'node:crypto';
import { fork, type ChildProcess } from 'node:child_process';
import { createServer, type ServerResponse } from 'node:http';
import { fileURLToPath } from 'node:url';
import { once } from 'node:events';
import { expect, it } from 'vitest';
import {
  BusinessClient,
  type CallReceipt,
} from '../src/integrations/go-business-api/client.js';
import { SessionStore } from '../src/sessions/persistence/store.js';
import { freezeManifest, type BusinessTool } from '../src/sessions/manifest.js';
import { reconcileInterrupted } from '../src/sessions/reconciliation.js';
import { SessionRuntime } from '../src/sessions/runtime.js';
import { testPool } from './storage/database.js';

// Parent HTTP fixture models Go's independent committed call ledger; Agent storage and processes are real.
const tool: BusinessTool = {
  name: 'write_resource',
  description: 'Write one resource',
  parameters: { type: 'object', properties: {}, additionalProperties: false },
  contract_version: '1',
  effect: 'write',
  replay: 'safe',
};
function deadline<T>(work: Promise<T>, ms = 12000): Promise<T> {
  return new Promise((resolve, reject) => {
    const timer = setTimeout(
      () => reject(Error('Recovery fixture deadline exceeded')),
      ms,
    );
    work.then(
      (x) => {
        clearTimeout(timer);
        resolve(x);
      },
      (e) => {
        clearTimeout(timer);
        reject(e);
      },
    );
  });
}
async function fixture(
  mode: 'model' | 'tool' | 'owned' | 'plan',
  outcome: 'final' | 'unknown' | 'tool_calls' = 'final',
) {
  const pool = testPool(),
    store = new SessionStore(pool);
  const scope = {
    actor_id: randomUUID(),
    org_id: randomUUID(),
    scope_id: randomUUID(),
    project_id: randomUUID(),
  };
  const session = await store.createSession({
    ...scope,
    request_id: randomUUID(),
    content_hash: 'a'.repeat(64),
    title: 'SIGKILL recovery',
  });
  const run = await store.admitRun(
    session.id,
    {
      input: authorizedText('Complete the task once'),
      ...scope,
      request_id: randomUUID(),
      content_hash: 'b'.repeat(64),
      goal: 'Complete the task once',
      expected_execution_revision: 0,
      manifest: { model_key: 'fixture' },
    },
    freezeManifest({ model_key: 'fixture' }, [tool]),
  );
  const ledger = new Map<string, CallReceipt>();
  const sends: string[] = [];
  const lookups: string[] = [];
  let held: ServerResponse | undefined;
  let reached!: () => void;
  const committed = new Promise<void>((resolve) => {
    reached = resolve;
  });
  const server = createServer(async (req, res) => {
    let raw = '';
    for await (const part of req) raw += String(part);
    const body = raw ? JSON.parse(raw) : undefined;
    const path = new URL(req.url!, 'http://fixture').pathname.replace(
      '/internal/agent-business',
      '',
    );
    const send = (value: unknown) => {
      res.setHeader('content-type', 'application/json');
      res.end(JSON.stringify(value));
    };
    if (path.endsWith('/decisions'))
      return send({
        current: null,
        authorization_revision: 0,
        barrier: false,
        terminal: false,
      });
    if (path === '/grants')
      return send({
        ...body,
        grant_id: randomUUID(),
        available_micros: 1000000,
        expires_at: body.lease_until,
        scope,
      });
    if (path === '/grants/validate') return send(body);
    if (path === '/snapshot') return send({ revision: 1 });
    if (path.includes('/calls/')) {
      const id = decodeURIComponent(path.split('/').at(-1)!);
      lookups.push(id);
      const saved = ledger.get(id);
      if (saved) return send(saved);
      res.statusCode = 404;
      return send({ code: 'not_found' });
    }
    if (path === '/models/stream' || path.endsWith('/execute')) {
      const kind = path === '/models/stream' ? 'model' : 'tool';
      sends.push(body.call.call_id);
      const result =
        kind === 'tool'
          ? { output: 'Resource committed', revision: 1 }
          : {
              output: 'Original provider answer',
              usage: { input_tokens: 1, output_tokens: 1 },
              model_key: 'fixture',
              stop_reason:
                mode !== 'model' && ledger.size === 0 ? 'tool_use' : 'stop',
              ...(mode !== 'model' && ledger.size === 0
                ? {
                    tool_calls: [
                      mode === 'plan'
                        ? {
                            id: 'plan-once',
                            name: 'plan_update',
                            arguments: { items: [] },
                          }
                        : mode === 'owned'
                          ? {
                              id: 'delegate-once',
                              name: 'delegate',
                              arguments: {
                                role: 'script',
                                task: 'Analyze the source',
                              },
                            }
                          : {
                              id: 'write-once',
                              name: 'write_resource',
                              arguments: {},
                            },
                    ],
                  }
                : {}),
            };
      const receipt: CallReceipt = {
        receipt_id: randomUUID(),
        root_run_id: body.grant.root_run_id,
        submission_id: body.grant.submission_id,
        call: body.call,
        status: outcome === 'unknown' ? 'unknown' : 'completed',
        result:
          outcome === 'tool_calls'
            ? {
                ...result,
                stop_reason: 'tool_use',
                tool_calls: [
                  {
                    id: 'not-yet-executed',
                    name: 'write_resource',
                    arguments: {},
                  },
                ],
              }
            : result,
        cost_micros: 1,
      };
      ledger.set(body.call.call_id, receipt);
      if (
        kind === mode ||
        (mode === 'plan' && kind === 'model' && ledger.size === 2)
      ) {
        held = res;
        reached();
        return;
      }
      return send(receipt);
    }
    res.statusCode = 404;
    send({ code: 'fixture_unknown_path' });
  });
  server.listen(0, '127.0.0.1');
  await once(server, 'listening');
  const address = server.address();
  if (!address || typeof address === 'string')
    throw Error('Missing fixture address');
  const origin = `http://127.0.0.1:${address.port}`;
  let child: ChildProcess | undefined;
  return {
    pool,
    scope,
    session,
    run,
    ledger,
    sends,
    lookups,
    origin,
    committed,
    spawn(window?: string) {
      child = fork(
        fileURLToPath(new URL('./helpers/recovery-child.ts', import.meta.url)),
        [],
        {
          execArgv: ['--import', 'tsx'],
          env: {
            ...process.env,
            RECOVERY_RUN: run.id,
            RECOVERY_ORIGIN: origin,
            ...(window ? { RECOVERY_WINDOW: window } : {}),
          },
          stdio: ['ignore', 'pipe', 'pipe', 'ipc'],
        },
      );
      let stderr = '';
      child.stderr?.on('data', (x) => (stderr += String(x)));
      child.on('exit', (code, signal) => {
        if (code && signal !== 'SIGKILL') process.stderr.write(stderr);
      });
      return child;
    },
    async cleanup() {
      if (child && !child.killed) child.kill('SIGKILL');
      held?.destroy();
      server.closeAllConnections();
      await new Promise<void>((resolve) => server.close(() => resolve()));
      await pool.end();
    },
  };
}

it.each([
  { mode: 'model', outcome: 'final', status: 'completed' },
  { mode: 'model', outcome: 'unknown', status: 'needs_reconciliation' },
  { mode: 'model', outcome: 'tool_calls', status: 'needs_reconciliation' },
  { mode: 'tool', outcome: 'final', status: 'completed' },
  { mode: 'plan', outcome: 'final', status: 'completed' },
] as const)(
  'recovers original $mode receipt ($outcome) after actual SIGKILL between Go commit and Pi result',
  async ({ mode, outcome, status }) => {
    const f = await fixture(mode, outcome);
    let runtime: SessionRuntime | undefined;
    try {
      const child = f.spawn();
      await deadline(f.committed);
      const exit = once(child, 'exit');
      child.kill('SIGKILL');
      expect((await exit)[1]).toBe('SIGKILL');
      const original = [...f.ledger.values()].at(-1)!;
      const sentBefore = [...f.sends];
      const lookupsBefore = f.lookups.length;
      // No completed Pi result can exist: the parent never sent the committed Go fixture receipt.
      const rows = await f.pool.query<{ body: string }>(
        "SELECT body FROM agent.runtime_records WHERE session_id=$1 AND record_type='entry'",
        [f.session.id],
      );
      expect(JSON.stringify(rows.rows)).not.toContain(original.receipt_id);
      await deadline(
        (async () => {
          for (;;) {
            const owner = await f.pool.query<{ expired: boolean }>(
              'SELECT lease_until<=clock_timestamp() expired FROM agent.runtime_sessions WHERE session_id=$1',
              [f.session.id],
            );
            if (owner.rows[0]?.expired) break;
            await new Promise((resolve) => setTimeout(resolve, 50));
          }
        })(),
      );
      runtime = await SessionRuntime.open(
        f.pool,
        new BusinessClient(f.origin, 'fixture'),
        f.run,
        30000,
      );
      await runtime.start();
      const state = await runtime.state();
      expect(f.lookups.slice(lookupsBefore)).toContain(original.call.call_id);
      if (mode === 'model' || mode === 'plan')
        expect(f.sends).toEqual(sentBefore);
      else
        expect(f.sends.filter((x) => x === original.call.call_id)).toHaveLength(
          1,
        );
      expect(state?.status).toBe(status);
      expect(state?.model_steps).toBe(mode === 'model' ? 1 : 2);
      if (status === 'completed') expect(state?.failure).toBeUndefined();
      if (mode === 'model') {
        expect(state).toMatchObject({
          reconciliation: {
            status,
            segment_id: f.run.submission_id,
            official_submission_status: 'interrupted',
            calls: [
              {
                call: original.call,
                receipt_id: original.receipt_id,
                status: original.status,
              },
            ],
          },
        });
        const proof = await f.pool.query(
          "SELECT body FROM agent.runtime_records WHERE session_id=$1 AND record_type='entry' AND body::jsonb->>'kind'='glowloom.reconciled_receipt'",
          [f.session.id],
        );
        expect(proof.rowCount).toBe(status === 'completed' ? 1 : 0);
        if (status === 'completed') {
          await reconcileInterrupted({
            harness: runtime.harness,
            root: runtime.root,
            business: new BusinessClient(f.origin, 'fixture'),
            runId: f.run.id,
            segmentId: f.run.submission_id,
          });
          expect(
            (
              await f.pool.query(
                "SELECT id FROM agent.runtime_records WHERE session_id=$1 AND record_type='entry' AND body::jsonb->>'kind'='glowloom.reconciled_receipt'",
                [f.session.id],
              )
            ).rowCount,
          ).toBe(1);
          const publicText = await f.pool.query(
            "SELECT payload FROM agent.runtime_events WHERE session_id=$1 AND event_type='text' AND payload->>'receipt_id'=$2",
            [f.session.id, original.receipt_id],
          );
          expect(publicText.rows).toEqual([
            {
              payload: {
                root_run_id: f.run.id,
                conversation_id: runtime.root.id,
                text: 'Original provider answer',
                receipt_id: original.receipt_id,
                source: 'reconciled_original_receipt',
                message_parts: [
                  {
                    run_id: f.run.id,
                    conversation_id: runtime.root.id,
                    message_id: expect.stringMatching(/^[0-9a-f-]{36}$/),
                    sequence: expect.stringMatching(/^\d+:\d+$/),
                    kind: 'assistant',
                    part_index: 0,
                    content: 'Original provider answer',
                  },
                ],
              },
            },
          ]);
          const messages = await f.pool.query(
            'SELECT message_id,run_id,conversation_id,sequence,kind,part_index,content FROM agent.public_messages WHERE run_id=$1',
            [f.run.id],
          );
          expect(messages.rowCount).toBe(1);
          expect(
            messages.rows.map((message) => ({
              ...message,
              conversation_id: Number(message.conversation_id),
            })),
          ).toEqual(publicText.rows[0]?.payload.message_parts);
        } else
          expect(JSON.stringify(state?.reconciliation)).toContain(
            original.call.call_id,
          );
      }
      const submissions = await f.pool.query<{ body: string }>(
        "SELECT body FROM agent.runtime_records WHERE session_id=$1 AND record_type='submission' AND request_key=$2",
        [f.session.id, JSON.stringify(f.run.submission_id)],
      );
      expect(submissions.rowCount).toBe(1);
      const official = JSON.parse(submissions.rows[0]!.body);
      expect(official.status).toBe(
        mode === 'model' || mode === 'plan' ? 'unanswered' : 'done',
      );
      if (mode === 'model') expect(official.reason).toBe('interrupted');
    } finally {
      await runtime?.close();
      await f.cleanup();
    }
  },
  20000,
);

it.each(['child_create_config', 'child_submit', 'child_collect'])(
  'reuses the same owned child after SIGKILL at %s',
  async (window) => {
    const f = await fixture('owned');
    let runtime: SessionRuntime | undefined;
    try {
      const child = f.spawn(window);
      const marker = await deadline(
        new Promise<{
          child_id: string;
          owner_task_id: string;
          submission_ids: string[];
        }>((resolve) =>
          child.on('message', (message) => {
            const m = message as {
              kind: string;
              child_id: string;
              owner_task_id: string;
              submission_ids: string[];
            };
            if (m.kind === 'window') resolve(m);
          }),
        ),
      );
      const exit = once(child, 'exit');
      child.kill('SIGKILL');
      expect((await exit)[1]).toBe('SIGKILL');
      const config = await f.pool.query(
        "SELECT id FROM agent.runtime_records WHERE session_id=$1 AND record_type='document' AND scope_key=$2 AND kind_key=$3",
        [
          f.session.id,
          JSON.stringify(['conversation', Number(marker.child_id)]),
          JSON.stringify('pi.agent'),
        ],
      );
      expect(config.rowCount).toBe(1);
      const configValue = await f.pool.query<{ content: string }>(
        'SELECT content FROM agent.runtime_document_versions WHERE session_id=$1 AND document_id=$2 ORDER BY commit_seq DESC LIMIT 1',
        [f.session.id, config.rows[0]!.id],
      );
      const agent = JSON.parse(configValue.rows[0]!.content);
      expect(agent.tools).toEqual(['skill_load']);
      expect(agent.extensions).not.toContain('glowloom.producer');
      if (window === 'child_create_config')
        expect(marker.submission_ids).toEqual([]);
      else expect(marker.submission_ids).toHaveLength(1);
      await deadline(
        (async () => {
          for (;;) {
            if (
              (
                await f.pool.query(
                  'SELECT lease_until<=clock_timestamp() expired FROM agent.runtime_sessions WHERE session_id=$1',
                  [f.session.id],
                )
              ).rows[0]?.expired
            )
              break;
            await new Promise((r) => setTimeout(r, 50));
          }
        })(),
      );
      runtime = await SessionRuntime.open(
        f.pool,
        new BusinessClient(f.origin, 'fixture'),
        f.run,
        30000,
      );
      await runtime.start();
      const children = await f.pool.query(
        "SELECT id,owner_task_id FROM agent.runtime_records WHERE session_id=$1 AND record_type='conversation' AND owner_task_id IS NOT NULL",
        [f.session.id],
      );
      expect(children.rows).toEqual([
        { id: marker.child_id, owner_task_id: marker.owner_task_id },
      ]);
      const submissions = await f.pool.query<{ id: string; body: string }>(
        "SELECT id,body FROM agent.runtime_records WHERE session_id=$1 AND record_type='submission' AND conversation_id=$2 AND request_key=$3",
        [
          f.session.id,
          marker.child_id,
          JSON.stringify(`delegation:${marker.owner_task_id}`),
        ],
      );
      expect(submissions.rowCount).toBe(1);
      if (marker.submission_ids.length)
        expect(submissions.rows[0]!.id).toBe(marker.submission_ids[0]);
      const state = await runtime.state();
      expect(state?.status).toBe('completed');
      expect(state?.delegations).toHaveLength(1);
      expect(state?.delegations[0]?.status).toBe('done');
      expect(state?.delegations[0]?.source_refs).toHaveLength(1);
      expect(f.sends).toHaveLength(3);
      expect(new Set(f.sends).size).toBe(3);
    } finally {
      await runtime?.close();
      await f.cleanup();
    }
  },
  20000,
);
