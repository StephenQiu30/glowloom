import { publicState } from '../runtime/pi-durable/events.js';
import type { Pool } from 'pg';
import { BACKGROUND_CONTEXT } from '@earendil-works/chord/context';
import type {
  CreateSessionInput,
  CreateRunInput,
  ControlInput,
  Identity,
  HistoryInput,
  MessageInput,
} from '../../contracts/agent.js';
import { AgentError } from './errors.js';
import type { AgentApplication, RuntimeEvent } from './port.js';
import { BusinessClient } from '../integrations/go-business-api/client.js';
import {
  SessionStore,
  inTransaction,
  type RunRow,
} from './persistence/store.js';
import { SessionRuntime } from './runtime.js';
import { freezeManifest, stableId } from './manifest.js';
import { DEFINITION_HASH } from '../runtime/pi-durable/definitions.js';
import {
  StateDoc,
  releaseOwnedAllocations,
} from '../runtime/pi-durable/docs.js';
import { ConversationStore } from './persistence/conversations.js';
const context = BACKGROUND_CONTEXT;
export class SessionService implements AgentApplication {
  readonly store: SessionStore;
  readonly conversations: ConversationStore;
  private active = new Map<string, SessionRuntime>();
  private opening = new Map<string, Promise<SessionRuntime>>();
  private jobs = new Map<string, Promise<void>>();
  private stopping = false;
  private pump: NodeJS.Timeout | undefined;
  constructor(
    private pool: Pool,
    private business: BusinessClient,
    private options: { leaseMs: number },
  ) {
    this.store = new SessionStore(pool);
    this.conversations = new ConversationStore(pool, this.store);
  }
  async ready() {
    try {
      await this.pool.query(
        'SELECT s.id,r.input,r.retry_of_run_id,m.message_id FROM agent.service_sessions s LEFT JOIN agent.service_runs r ON r.session_id=s.id LEFT JOIN agent.public_messages m ON m.run_id=r.id LIMIT 0',
      );
      return !this.stopping;
    } catch {
      return false;
    }
  }
  createSession(input: CreateSessionInput) {
    if (this.stopping) throw new AgentError('shutting_down');
    return this.store.createSession(input);
  }
  listSessions(who: HistoryInput) {
    return this.conversations.sessions(who);
  }
  listRuns(id: string, query: HistoryInput) {
    return this.conversations.runs(id, query);
  }
  readSubmission(id: string, request: string, who: Identity) {
    return this.conversations.submission(id, request, who);
  }
  currentSubmission(id: string, who: Identity) {
    return this.conversations.current(id, who);
  }
  listMessages(id: string, query: MessageInput) {
    return this.conversations.messages(id, query);
  }
  readPublicMessage(id: string, messageId: string, who: Identity) {
    return this.conversations.message(id, messageId, who);
  }
  async readSession(id: string, who: Identity) {
    return inTransaction(
      this.pool,
      async (client) => {
        const session = await this.store.session(id, who, client);
        if (!session) return undefined;
        const last = (
          await client.query<RunRow>(
            'SELECT * FROM agent.service_runs WHERE session_id=$1 ORDER BY accepted_revision DESC LIMIT 1',
            [id],
          )
        ).rows[0];
        const run = last ? await this.store.snapshot(last, client) : undefined;
        return {
          ...this.store.sessionDTO(session),
          execution_revision: Number(
            run?.state.execution_revision ?? session.execution_revision,
          ),
          run,
        };
      },
      true,
    );
  }
  async createRun(id: string, input: CreateRunInput) {
    if (this.stopping) throw new AgentError('shutting_down');
    const session = await this.store.session(id, input);
    if (!session) throw new AgentError('session_not_found');
    const manifest = freezeManifest(
      input.manifest,
      await this.business.tools(),
    );
    const run = await this.store.admitRun(id, input, manifest);
    this.launch(run);
    return {
      root_run_id: run.id,
      session_id: run.session_id,
      submission_id: run.submission_id,
      status: 'accepted',
      execution_revision: Number(run.accepted_revision),
    };
  }
  async readRun(id: string, who: Identity) {
    return inTransaction(
      this.pool,
      async (client) => {
        const run = await this.store.run(id, who, client);
        return run ? this.store.snapshot(run, client) : undefined;
      },
      true,
    );
  }
  private async runtime(run: RunRow) {
    const prior = this.active.get(run.session_id);
    if (this.stopping) throw new AgentError('shutting_down');
    if (prior?.run.id === run.id && !prior.isClosed) return prior;
    if (prior) {
      await prior.close();
      this.active.delete(run.session_id);
    }
    const pending = this.opening.get(run.session_id);
    if (pending) return pending;
    const open = SessionRuntime.open(
      this.pool,
      this.business,
      run,
      this.options.leaseMs,
    )
      .then(async (value) => {
        if (this.stopping) {
          await value.close();
          throw new AgentError('shutting_down');
        }
        this.active.set(run.session_id, value);
        return value;
      })
      .finally(() => this.opening.delete(run.session_id));
    this.opening.set(run.session_id, open);
    return open;
  }
  private launch(run: RunRow) {
    if (this.stopping || this.jobs.has(run.id) || this.jobs.size >= 4) return;
    const job = (async () => {
      const runtime = await this.runtime(run);
      await runtime.start();
    })()
      .catch(async () => {
        const current = this.active.get(run.session_id);
        if (current && !current.isClosed)
          await current.harness
            .commit(async (tx) => {
              const state = await tx.doc(StateDoc, current.root.id);
              if (
                state.root_run_id === run.id &&
                ['accepted', 'running'].includes(state.status)
              ) {
                state.status = 'failed';
                state.failure = {
                  code: 'execution_unavailable',
                  message:
                    'The execution could not proceed. Check the original business receipts before retrying.',
                };
                state.execution_revision++;
              }
            }, context)
            .catch(() => {});
      })
      .finally(async () => {
        const current = this.active.get(run.session_id);
        let continueRun = false;
        if (current?.run.id === run.id && !current.isClosed) {
          await current
            .serialize(async () => {
              const state = await current.state();
              continueRun = state?.status === 'running';
              if (state && !['accepted', 'running'].includes(state.status)) {
                if (this.active.get(run.session_id) === current)
                  this.active.delete(run.session_id);
                await current.close();
              }
            })
            .catch(() => {});
        }
        this.jobs.delete(run.id);
        // A control may submit its continuation before the previous drive has returned.
        if (continueRun) this.launch(run);
      });
    this.jobs.set(run.id, job);
  }
  async waitForRun(id: string) {
    await this.jobs.get(id);
  }
  async command(id: string, input: ControlInput) {
    if (this.stopping) throw new AgentError('shutting_down');
    if (input.root_run_id !== id)
      throw new AgentError('decision_scope_conflict');
    const run = await this.store.run(id, input);
    if (!run || run.session_id !== input.session_id)
      throw new AgentError('run_not_found');
    const cached = await this.store.acceptControl(input);
    if (cached)
      return this.finishControl(input, cached as Record<string, unknown>);
    const runtime = await this.runtime(run);
    return runtime.serialize(async () => {
      const saved = (await runtime.state())?.applied_decisions.find(
        (d) => d.decision_id === input.decision_id,
      );
      if (saved) return this.finishControl(input, saved);
      const current = await this.business.decisions(id);
      if (
        current.authorization_revision !== input.authorization_revision ||
        current.current?.decision_id !== input.decision_id ||
        current.current.action !== input.action
      ) {
        if (current.authorization_revision < input.authorization_revision)
          throw new AgentError('authorization_revision_gap');
        const receipt = {
          decision_id: input.decision_id,
          authorization_revision: input.authorization_revision,
          status: 'superseded',
          execution_revision: (await runtime.state())?.execution_revision ?? 0,
          receipt_id: stableId(id, 'receipt:' + input.decision_id),
        };
        await runtime.harness.commit(async (tx) => {
          const state = await tx.doc(StateDoc, runtime.root.id);
          state.applied_decisions.push(receipt);
        }, context);
        return this.finishControl(input, receipt);
      }
      runtime.block();
      await runtime.root.abort(context);
      const continuation = stableId(id, 'continuation:' + input.decision_id);
      const receiptId = stableId(id, 'receipt:' + input.decision_id);
      await runtime.harness.commit(async (tx) => {
        const state = await tx.doc(StateDoc, runtime.root.id);
        if (state.quiesced?.decision_id === input.decision_id) {
          state.quiesced.owner_epoch = runtime.epoch();
          return;
        }
        state.status =
          input.action === 'stop'
            ? 'stopped'
            : input.action === 'reject'
              ? 'rejected'
              : 'paused';
        releaseOwnedAllocations(state, input.action);
        state.authorization_revision = input.authorization_revision;
        state.execution_revision++;
        state.quiesced = {
          receipt_id: receiptId,
          decision_id: input.decision_id,
          authorization_revision: input.authorization_revision,
          owner_epoch: runtime.epoch(),
          continuation_id: continuation,
        };
      }, context);
      let receipt: Record<string, unknown> | undefined;
      if (input.action === 'approve' || input.action === 'resume') {
        await runtime.announceControlOwner(
          input.authorization_revision,
          current.barrier ? undefined : continuation,
        );
        const grant = await this.business.quiesced(id, {
          receipt_id: receiptId,
          decision_id: input.decision_id,
          authorization_revision: input.authorization_revision,
          owner_epoch: runtime.epoch(),
          continuation_id: continuation,
          lease_until: runtime.owner.leaseUntil.toISOString(),
        });
        runtime.setGrant(grant);
        await runtime.harness.commit(async (tx) => {
          const state = await tx.doc(StateDoc, runtime.root.id);
          await runtime.root.submit(tx, {
            type: 'input',
            content:
              'Continue the unfinished adopted plan under the current approval. Query original business receipts before generating or writing again.',
            requestId: continuation,
            whenBusy: 'reject',
          });
          state.segment_id = continuation;
          state.status = 'running';
          state.pending_proposal = null;
          state.execution_revision++;
          const applied = {
            decision_id: input.decision_id,
            authorization_revision: input.authorization_revision,
            status: 'applied',
            execution_revision: state.execution_revision,
            receipt_id: receiptId,
          };
          state.applied_decisions.push(applied);
          receipt = applied;
        }, context);
      } else {
        await runtime.harness.commit(async (tx) => {
          const state = await tx.doc(StateDoc, runtime.root.id);
          const applied = {
            decision_id: input.decision_id,
            authorization_revision: input.authorization_revision,
            status: 'applied',
            execution_revision: state.execution_revision,
            receipt_id: receiptId,
          };
          state.applied_decisions.push(applied);
          receipt = applied;
        }, context);
      }
      if (!receipt) throw new Error('Control receipt was not committed');
      const result = await this.finishControl(input, receipt);
      if (input.action === 'approve' || input.action === 'resume')
        this.launch(run);
      return result;
    });
  }
  private async finishControl(
    input: ControlInput,
    receipt: Record<string, unknown>,
  ) {
    await this.pool.query(
      'UPDATE agent.service_control_inbox SET receipt=$2 WHERE decision_id=$1',
      [input.decision_id, receipt],
    );
    await this.business.receipt(
      input.root_run_id,
      receipt as Parameters<BusinessClient['receipt']>[1],
    );
    return receipt;
  }
  async events(
    id: string,
    who: Identity,
    after: string | undefined,
  ): Promise<RuntimeEvent[]> {
    const run = await this.store.run(id, who);
    if (!run) throw new AgentError('run_not_found');
    const match = /^(0|[1-9]\d*):(0|[1-9]\d*)$/.exec(after ?? '0:0');
    if (!match) throw new AgentError('invalid_event_cursor');
    const seq = BigInt(match[1]!);
    const ordinal = BigInt(match[2]!);
    if (seq > 9007199254740991n || ordinal > 2147483647n)
      throw new AgentError('invalid_event_cursor');
    const row = (
      await this.pool.query<{ min: string | null }>(
        'SELECT min(commit_seq)::text min FROM agent.runtime_events WHERE session_id=$1',
        [run.session_id],
      )
    ).rows[0];
    if (seq > 0n && row?.min && seq < BigInt(row.min))
      throw new AgentError('snapshot_required');
    const result = await this.pool.query<{
      commit_seq: string;
      event_ordinal: number;
      event_type: string;
      payload: unknown;
    }>(
      "SELECT commit_seq,event_ordinal,event_type,payload FROM agent.runtime_events WHERE session_id=$1 AND (commit_seq,event_ordinal)>($2,$3) AND payload->>'root_run_id'=$4 ORDER BY commit_seq,event_ordinal LIMIT 100",
      [run.session_id, seq.toString(), ordinal.toString(), id],
    );
    return result.rows.map((row) => ({
      cursor: `${row.commit_seq}:${row.event_ordinal}`,
      type: row.event_type,
      payload:
        row.event_type === 'agent.state'
          ? publicState(
              row.payload as import('@earendil-works/pi-durable').JsonObject,
            )
          : row.payload,
    }));
  }
  startRecovery() {
    if (this.pump) return;
    const recover = async () => {
      if (this.stopping) return;
      const controls = await this.pool.query<{ input: ControlInput }>(
        'SELECT input FROM agent.service_control_inbox WHERE receipt IS NULL ORDER BY created_at LIMIT 20',
      );
      for (const row of controls.rows) {
        if (this.stopping) return;
        await this.command(row.input.root_run_id, row.input).catch(() => {});
      }
      const runs = await this.pool.query<RunRow>(
        `SELECT latest.* FROM (SELECT DISTINCT ON(session_id) * FROM agent.service_runs ORDER BY session_id,accepted_revision DESC) latest
    LEFT JOIN LATERAL (SELECT payload->>'status' status FROM agent.runtime_events e WHERE e.session_id=latest.session_id::text AND e.payload->>'root_run_id'=latest.id::text AND e.event_type='agent.state' ORDER BY commit_seq DESC,event_ordinal DESC LIMIT 1) state ON true
    LEFT JOIN agent.runtime_sessions owner ON owner.session_id=latest.session_id::text
    WHERE latest.manifest->>'definition_hash'=$1 AND latest.manifest->>'pi_version'='1.5.0' AND coalesce(state.status,'accepted') IN ('accepted','running') AND (owner.session_id IS NULL OR owner.lease_until<=clock_timestamp())
    ORDER BY latest.created_at,latest.id LIMIT 32`,
        [DEFINITION_HASH],
      );
      for (const run of runs.rows) {
        if (this.stopping) return;
        const snapshot = await this.store.snapshot(run);
        if (['accepted', 'running'].includes(String(snapshot.state.status)))
          this.launch(run);
      }
    };
    this.pump = setInterval(() => {
      void recover().catch(() => {});
    }, 1000);
    this.pump.unref();
    void recover().catch(() => {});
  }
  async close() {
    if (this.stopping) return;
    this.stopping = true;
    if (this.pump) clearInterval(this.pump);
    await Promise.allSettled([...this.opening.values()]);
    await Promise.allSettled(
      [...this.active.values()].map((runtime) => runtime.close()),
    );
    await Promise.allSettled([...this.jobs.values()]);
    this.active.clear();
  }
}
