import { randomUUID } from 'node:crypto';
import { BACKGROUND_CONTEXT } from '@earendil-works/chord/context';
import { expect, it, vi } from 'vitest';
import { BusinessClient } from '../src/business/client.js';
import { SessionStore } from '../src/sessions/persistence/store.js';
import { SessionRuntime } from '../src/sessions/runtime.js';
import { SessionService } from '../src/sessions/service.js';
import { freezeManifest } from '../src/sessions/manifest.js';
import {
  claimSessionOwner,
  releaseSessionOwner,
} from '../src/pi-durable/storage/ownership.js';
import {
  RecoveryStore,
  RecoveryKind,
  RecoveryCode,
} from '../src/sessions/recovery.js';
import { testPool } from './storage/database.js';
import { authorizedText } from './helpers/conversation-input.js';

async function admitted() {
  const pool = testPool();
  const scope = {
    actor_id: randomUUID(),
    org_id: randomUUID(),
    scope_id: randomUUID(),
    project_id: randomUUID(),
  };
  const store = new SessionStore(pool);
  const session = await store.createSession({
    ...scope,
    request_id: randomUUID(),
    content_hash: 'a'.repeat(64),
    title: 'Recovery',
  });
  const run = await store.admitRun(
    session.id,
    {
      ...scope,
      request_id: randomUUID(),
      content_hash: 'b'.repeat(64),
      expected_execution_revision: 0,
      goal: 'Recovery',
      input: authorizedText('Recovery'),
      manifest: { model_key: 'fixture' },
    },
    freezeManifest({ model_key: 'fixture' }, []),
  );
  return { pool, store, run, scope };
}

it('keeps the real PG lease when Harness.close cannot prove draining', async () => {
  const { pool, run } = await admitted();
  const runtime = await SessionRuntime.open(
    pool,
    new BusinessClient('http://fixture.invalid', 'fixture'),
    run,
    30000,
  );
  const actualClose = runtime.harness.close.bind(runtime.harness);
  const close = vi
    .spyOn(runtime.harness, 'close')
    .mockRejectedValueOnce(new Error('drain failed'));
  try {
    await expect(runtime.close()).rejects.toThrow('drain failed');
    await expect(
      claimSessionOwner(pool, run.session_id, randomUUID(), 30000),
    ).rejects.toThrow();
    expect(runtime.isClosed).toBe(true);
    expect(() => runtime.grant()).toThrow();
  } finally {
    close.mockRestore();
    await actualClose(BACKGROUND_CONTEXT);
    await releaseSessionOwner(pool, run.session_id, runtime.owner);
    await pool.end();
  }
});

it('persists bounded retry and parking, exposing only safe diagnostics in snapshots', async () => {
  const { pool, run, store } = await admitted();
  try {
    const recovery = new RecoveryStore(pool);
    for (let i = 0; i < 5; i++)
      await recovery.failed(
        RecoveryKind.Run,
        run.id,
        run.id,
        RecoveryCode.ExecutionUnavailable,
      );
    const reopened = new RecoveryStore(pool);
    const rows = await reopened.read(run.id);
    expect(rows).toHaveLength(1);
    expect(rows[0]?.attempts).toBe(5);
    expect(rows[0]?.parked_at).not.toBeNull();
    expect(rows[0]?.code).toBe(RecoveryCode.ExecutionUnavailable);
    const snapshot = await store.snapshot(run);
    expect(snapshot.recovery).toEqual(rows);
    expect(snapshot.execution_available).toBe(true);
  } finally {
    await pool.end();
  }
});

it('reports recovery scan errors without swallowing them or leaking the error text', async () => {
  const pool = testPool();
  const events: unknown[] = [];
  const service = new SessionService(
    pool,
    new BusinessClient('http://fixture.invalid', 'fixture'),
    { leaseMs: 30000, onRecoveryEvent: (event) => events.push(event) },
  );
  const query = vi
    .spyOn(pool, 'query')
    .mockRejectedValueOnce(new Error('password=should-not-be-logged'));
  try {
    await service.recoverOnce();
    expect(events).toHaveLength(1);
    expect(JSON.stringify(events)).toContain(RecoveryCode.ScanUnavailable);
    expect(JSON.stringify(events)).not.toContain('password');
    expect(service.recoveryStatus.consecutive_failures).toBe(1);
  } finally {
    query.mockRestore();
    await service.close();
    await pool.end();
  }
});

it('reads an admitted run within the same transaction using a one-connection pool', async () => {
  const { pool, run, scope } = await admitted();
  const { Pool } = await import('pg');
  const single = new Pool({
    connectionString: process.env.GLOWLOOM_AGENT_TEST_DB_DSN,
    max: 1,
    connectionTimeoutMillis: 150,
  });
  const service = new SessionService(
    single,
    new BusinessClient('http://fixture.invalid', 'fixture'),
    { leaseMs: 30000 },
  );
  try {
    expect((await service.readRun(run.id, scope))?.root_run_id).toBe(run.id);
    expect(
      (await service.readSession(run.session_id, scope))?.run?.root_run_id,
    ).toBe(run.id);
  } finally {
    await service.close();
    await single.end();
    await pool.end();
  }
});

it('reports shutdown drain failures and preserves the original lease', async () => {
  const { pool, run } = await admitted();
  const events: unknown[] = [];
  const service = new SessionService(
    pool,
    new BusinessClient('http://fixture.invalid', 'fixture'),
    { leaseMs: 30000, onRecoveryEvent: (event) => events.push(event) },
  );
  // Open the normal lazy runtime without launching model work, to isolate shutdown.
  const runtime = await service['runtime'](run);
  const actualClose = runtime.harness.close.bind(runtime.harness);
  const close = vi
    .spyOn(runtime.harness, 'close')
    .mockRejectedValueOnce(new Error('drain failed'));
  try {
    await expect(service.close()).rejects.toThrow('Execution shutdown failed');
    expect(events).toContainEqual(
      expect.objectContaining({
        root_run_id: run.id,
        code: RecoveryCode.CloseFailed,
      }),
    );
    await expect(
      claimSessionOwner(pool, run.session_id, randomUUID(), 30000),
    ).rejects.toThrow();
  } finally {
    close.mockRestore();
    await actualClose(BACKGROUND_CONTEXT);
    await releaseSessionOwner(pool, run.session_id, runtime.owner);
    await pool.end();
  }
});
