import { authorizedText } from './helpers/conversation-input.js';
import { afterAll, expect, it } from 'vitest';
import { randomUUID } from 'node:crypto';
import { SessionStore } from '../src/sessions/persistence/store.js';
import { ConversationStore } from '../src/sessions/persistence/conversations.js';
import { freezeManifest } from '../src/sessions/manifest.js';
import { testPool } from './storage/database.js';
import {
  ROOT_CONVERSATION_ID,
  type StorageWrite,
} from '@earendil-works/pi-durable';
import { BACKGROUND_CONTEXT } from '@earendil-works/chord/context';
import { claimSessionOwner } from '../src/adapters/postgres/pi-durable/ownership.js';
import { openPostgresStorage } from '../src/adapters/postgres/pi-durable/postgres.js';
const pool = testPool();
afterAll(() => pool.end());
const manifest = freezeManifest({ model_key: 'fixture', max_micros: 100 }, []);
async function fixture() {
  const store = new SessionStore(pool);
  const who = {
    actor_id: randomUUID(),
    org_id: randomUUID(),
    scope_id: randomUUID(),
    project_id: randomUUID(),
  };
  const session = await store.createSession({
    ...who,
    request_id: randomUUID(),
    content_hash: 'a'.repeat(64),
    title: '会话',
  });
  return { store, who, session, history: new ConversationStore(pool, store) };
}
it('atomically retains the exact accepted input and replays the same admission', async () => {
  const { store, who, session } = await fixture();
  const input = {
    schema_version: 1 as const,
    draft: { reference: 'frozen' },
    input_hash: 'b'.repeat(64),
    plain_text: '第一条',
    prompt: '第一条',
    media_pins: [],
  };
  const command = {
    ...who,
    request_id: randomUUID(),
    content_hash: 'c'.repeat(64),
    expected_execution_revision: 0,
    goal: '第一条',
    manifest: {},
    input,
  };
  const run = await store.admitRun(session.id, command, manifest);
  expect(run.input).toEqual(input);
  expect((await store.admitRun(session.id, command, manifest)).id).toBe(run.id);
  await expect(
    store.admitRun(
      session.id,
      { ...command, input: { ...input, plain_text: 'changed' } },
      manifest,
    ),
  ).rejects.toThrow(/identity/);
  const snapshot = await store.snapshot(run);
  expect(snapshot.input).toEqual(input);
  expect(snapshot.messages_page).toMatchObject({ items: [], through: '0:0' });
});
it('pages all attempts and binds the cursor to its private session', async () => {
  const { store, who, session, history } = await fixture();
  for (let i = 1; i <= 53; i++)
    await pool.query(
      'INSERT INTO agent.service_runs(id,session_id,request_id,content_hash,goal,manifest,submission_id,accepted_revision) VALUES($1,$2,$3,$4,$5,$6,$7,$8)',
      [
        randomUUID(),
        session.id,
        randomUUID(),
        'a'.repeat(64),
        `消息${i}`,
        manifest,
        randomUUID(),
        i,
      ],
    );
  const first = await history.runs(session.id, who);
  expect(first.items.map((item) => item.accepted_revision)).toEqual(
    Array.from({ length: 20 }, (_, i) => i + 34),
  );
  const second = await history.runs(session.id, {
    ...who,
    cursor: first.next_cursor!,
  });
  expect(second.items.map((item) => item.accepted_revision)).toEqual(
    Array.from({ length: 20 }, (_, i) => i + 14),
  );
  const last = await history.runs(session.id, {
    ...who,
    cursor: second.next_cursor!,
  });
  expect(last.items.map((item) => item.accepted_revision)).toEqual(
    Array.from({ length: 13 }, (_, i) => i + 1),
  );
  expect(last.next_cursor).toBeUndefined();
  await expect(
    history.runs(session.id, {
      ...who,
      actor_id: randomUUID(),
      cursor: first.next_cursor!,
    }),
  ).rejects.toThrow(/not_found/);
  await expect(
    history.runs(session.id, {
      ...who,
      cursor: first.next_cursor!.slice(0, -1) + '!',
    }),
  ).rejects.toThrow(/cursor/);
  expect(
    (await history.sessions({ ...who, actor_id: randomUUID() })).items,
  ).toEqual([]);
  expect(
    await store.session(session.id, { ...who, actor_id: randomUUID() }),
  ).toBeUndefined();
});

it('reopens and pages more than 100 committed messages without losing older output', async () => {
  const { store, who, session, history } = await fixture();
  const run = await store.admitRun(
    session.id,
    {
      input: authorizedText('history'),
      ...who,
      request_id: randomUUID(),
      content_hash: 'a'.repeat(64),
      goal: 'history',
      expected_execution_revision: 0,
      manifest: {},
    },
    manifest,
  );
  const owner = await claimSessionOwner(pool, session.id, randomUUID(), 60000);
  let index = 0;
  const storage = await openPostgresStorage(pool, session.id, owner, {
    projectEvents: () => [
      {
        type: 'text',
        payload: {
          root_run_id: run.id,
          conversation_id: Number(ROOT_CONVERSATION_ID),
          text: `part${++index}`,
        },
      },
    ],
  });
  const write: StorageWrite = {
    type: 'conversation',
    value: { id: ROOT_CONVERSATION_ID },
  };
  try {
    for (let i = 0; i < 123; i++) {
      const result = await storage.commit(
        [i === 0 ? [write] : []],
        () => {},
        BACKGROUND_CONTEXT,
      );
      expect(result.stored).toBe(1);
    }
    const snapshot = await store.snapshot(run);
    expect(snapshot.messages_page.items).toHaveLength(50);
    const page2 = await history.messages(run.id, {
      ...who,
      through: snapshot.snapshot_cursor,
      before: snapshot.messages_page.next_cursor!,
    });
    const page3 = await history.messages(run.id, {
      ...who,
      through: snapshot.snapshot_cursor,
      before: page2.next_cursor!,
    });
    expect(
      [...page3.items, ...page2.items, ...snapshot.messages_page.items].map(
        (part) => part.content,
      ),
    ).toEqual(Array.from({ length: 123 }, (_, i) => `part${i + 1}`));
    expect(page3.next_cursor).toBeUndefined();
    const last = (
      await pool.query<{ payload: { message_parts: unknown[] } }>(
        'SELECT payload FROM agent.runtime_events WHERE session_id=$1 ORDER BY commit_seq DESC,event_ordinal DESC LIMIT 1',
        [session.id],
      )
    ).rows[0]!;
    expect(last.payload.message_parts).toEqual([
      snapshot.messages_page.items.at(-1),
    ]);
  } finally {
    await storage.close(BACKGROUND_CONTEXT);
  }
});

it('rolls back public messages together with a refused Storage commit', async () => {
  const { session } = await fixture();
  const owner = await claimSessionOwner(pool, session.id, randomUUID(), 60000);
  const storage = await openPostgresStorage(pool, session.id, owner, {
    projectEvents: () => [
      {
        type: 'text',
        payload: {
          root_run_id: randomUUID(),
          conversation_id: Number(ROOT_CONVERSATION_ID),
          text: 'must roll back',
        },
      },
      {
        type: 'text',
        payload: {
          root_run_id: 'invalid',
          conversation_id: Number(ROOT_CONVERSATION_ID),
          text: 'refused',
        },
      },
    ],
  });
  try {
    await expect(
      storage.commit(
        [[{ type: 'conversation', value: { id: ROOT_CONVERSATION_ID } }]],
        () => {},
        BACKGROUND_CONTEXT,
      ),
    ).rejects.toThrow();
    expect(
      Number(
        (
          await pool.query<{ count: string }>(
            'SELECT count(*) FROM agent.public_messages WHERE session_id=$1',
            [session.id],
          )
        ).rows[0]!.count,
      ),
    ).toBe(0);
    expect(
      Number(
        (
          await pool.query<{ count: string }>(
            'SELECT count(*) FROM agent.runtime_events WHERE session_id=$1',
            [session.id],
          )
        ).rows[0]!.count,
      ),
    ).toBe(0);
  } finally {
    await storage.close(BACKGROUND_CONTEXT);
  }
});
it('backfills retained public events deterministically and can run twice', async () => {
  const { backfillConversationMessages } =
    await import('../src/sessions/persistence/backfill-messages.js');
  const { session } = await fixture(),
    rid = randomUUID();
  await claimSessionOwner(pool, session.id, randomUUID(), 30000);
  await pool.query(
    "INSERT INTO agent.runtime_events(session_id,commit_seq,event_ordinal,event_type,payload) VALUES($1,1,1,'text',$2)",
    [
      session.id,
      {
        root_run_id: rid,
        conversation_id: Number(ROOT_CONVERSATION_ID),
        text: '历史公开文字',
      },
    ],
  );
  await backfillConversationMessages(pool, 2);
  await backfillConversationMessages(pool, 2);
  const rows = (
    await pool.query(
      'SELECT message_id,content FROM agent.public_messages WHERE session_id=$1',
      [session.id],
    )
  ).rows;
  expect(rows).toHaveLength(1);
  expect(rows[0]?.content).toBe('历史公开文字');
});
it('rejects retry settings that differ from the accepted manifest', async () => {
  const { store, who, session } = await fixture(),
    input = authorizedText('原始要求');
  const first = await store.admitRun(
    session.id,
    {
      ...who,
      request_id: randomUUID(),
      content_hash: '3'.repeat(64),
      expected_execution_revision: 0,
      goal: '原始要求',
      input,
      manifest: {},
    },
    manifest,
  );
  await claimSessionOwner(pool, session.id, randomUUID(), 30000);
  await pool.query(
    "INSERT INTO agent.runtime_events(session_id,commit_seq,event_ordinal,event_type,payload) VALUES($1,1,1,'agent.state',$2)",
    [
      session.id,
      { root_run_id: first.id, status: 'completed', execution_revision: 1 },
    ],
  );
  const changed = freezeManifest({ model_key: 'other', max_micros: 100 }, []);
  await expect(
    store.admitRun(
      session.id,
      {
        ...who,
        request_id: randomUUID(),
        content_hash: '4'.repeat(64),
        expected_execution_revision: 1,
        goal: '原始要求',
        input,
        retry_of_run_id: first.id,
        manifest: {},
      },
      changed,
    ),
  ).rejects.toThrow(/retry_input_changed/);
});
it('rejects a message watermark newer than the committed session', async () => {
  const { store, who, session, history } = await fixture(),
    input = authorizedText('watermark');
  const run = await store.admitRun(
    session.id,
    {
      ...who,
      request_id: randomUUID(),
      content_hash: '5'.repeat(64),
      expected_execution_revision: 0,
      goal: 'watermark',
      input,
      manifest: {},
    },
    manifest,
  );
  await expect(
    history.messages(run.id, { ...who, through: '999999:1' }),
  ).rejects.toThrow(/invalid_snapshot_watermark/);
});

it('persists scoped history updates, pages pinned items first, and archives only terminal sessions', async () => {
  const { store, who, session, history } = await fixture();
  const newest = await store.createSession({
    ...who,
    request_id: randomUUID(),
    content_hash: 'a'.repeat(64),
    title: '最近',
  });
  const admit = (id: string) =>
    store.admitRun(
      id,
      {
        ...who,
        request_id: randomUUID(),
        content_hash: 'a'.repeat(64),
        goal: '隔离历史测试',
        expected_execution_revision: 0,
        input: authorizedText('隔离历史测试'),
        manifest: {},
      },
      manifest,
    );
  const run = await admit(session.id);
  await admit(newest.id);
  expect((await history.sessions(who)).items.map((r) => r.id)).toEqual([
    newest.id,
    session.id,
  ]);
  await expect(
    history.updateSession(session.id, {
      ...who,
      scope_id: randomUUID(),
      action: 'pin',
    }),
  ).rejects.toThrow('session_not_found');
  await expect(
    history.updateSession(session.id, {
      ...who,
      action: 'rename',
      title: '   ',
    }),
  ).rejects.toThrow('invalid_session_update');
  await history.updateSession(session.id, { ...who, action: 'pin' });
  await history.updateSession(session.id, { ...who, action: 'pin' });
  await history.updateSession(session.id, {
    ...who,
    action: 'rename',
    title: '  改名后  ',
  });
  const first = await history.sessions({ ...who, limit: 1 });
  expect(first.items).toMatchObject([
    { id: session.id, pinned: true, title: '改名后' },
  ]);
  expect(
    (await history.sessions({ ...who, cursor: first.next_cursor! })).items,
  ).toMatchObject([{ id: newest.id, pinned: false }]);
  await history.updateSession(session.id, { ...who, action: 'unpin' });
  expect((await history.sessions(who)).items[0]?.id).toBe(newest.id);

  await expect(
    history.updateSession(session.id, { ...who, action: 'delete' }),
  ).rejects.toThrow('session_has_active_run');
  const owner = await claimSessionOwner(pool, session.id, randomUUID(), 60000);
  const storage = await openPostgresStorage(pool, session.id, owner, {
    projectEvents: () => [
      {
        type: 'agent.state',
        payload: { root_run_id: run.id, status: 'completed' },
      },
    ],
  });
  try {
    await storage.commit(
      [[{ type: 'conversation', value: { id: ROOT_CONVERSATION_ID } }]],
      () => {},
      BACKGROUND_CONTEXT,
    );
  } finally {
    await storage.close(BACKGROUND_CONTEXT);
  }
  await history.updateSession(session.id, { ...who, action: 'delete' });
  await history.updateSession(session.id, { ...who, action: 'delete' });
  expect((await history.sessions(who)).items.map((r) => r.id)).toEqual([
    newest.id,
  ]);
  expect(await store.session(session.id, who)).toBeUndefined();
  expect(await store.run(run.id, who)).toBeUndefined();
  await expect(admit(session.id)).rejects.toThrow('session_not_found');
  await expect(
    history.updateSession(session.id, { ...who, action: 'pin' }),
  ).rejects.toThrow('session_not_found');
  expect(
    (
      await pool.query('SELECT id FROM agent.service_runs WHERE id=$1', [
        run.id,
      ])
    ).rowCount,
  ).toBe(1);
});

it('serializes history deletion with admission on the same session', async () => {
  const { store, who, session, history } = await fixture();
  const [admission, removal] = await Promise.allSettled([
    store.admitRun(
      session.id,
      {
        ...who,
        request_id: randomUUID(),
        content_hash: 'a'.repeat(64),
        goal: '同时受理',
        expected_execution_revision: 0,
        input: authorizedText('同时受理'),
        manifest: {},
      },
      manifest,
    ),
    history.updateSession(session.id, { ...who, action: 'delete' }),
  ]);
  if (admission.status === 'fulfilled') {
    expect(removal).toMatchObject({
      status: 'rejected',
      reason: { code: 'session_has_active_run' },
    });
    expect(await store.session(session.id, who)).toBeDefined();
  } else {
    expect(admission.reason).toMatchObject({ code: 'session_not_found' });
    expect(removal.status).toBe('fulfilled');
    expect(await store.session(session.id, who)).toBeUndefined();
  }
});
