import type { Pool } from 'pg';
import { database, queries } from '../database/connection.js';
import { BACKGROUND_CONTEXT } from '@earendil-works/chord/context';
import type {
  RuntimeEvent,
  AgentApplication,
  CreateSessionInput,
  UpdateSessionInput,
  CreateRunInput,
  ControlInput,
  Identity,
  HistoryInput,
  MessageInput,
} from './types.js';
import { AgentError } from './errors.js';
import { BusinessClient } from '../business/client.js';
import {
  SessionStore,
  inTransaction,
  runRow,
  type RunRow,
} from './persistence/store.js';
import { SessionRuntime } from './execution/runtime.js';
import { freezeManifest } from './manifest.js';
import { StateDoc } from '../pi-durable/docs.js';
import { ConversationStore } from './persistence/conversations.js';
import type { DeliveryOptions } from './execution/delivery.js';
import { runtimeDefaults } from '../config.js';
import {
  RecoveryStore,
  RecoveryKind,
  RecoveryCode,
  type RecoveryEvent,
} from './recovery/store.js';
import { SessionControls } from './execution/controls.js';
import { readRunEvents } from './persistence/public-events.js';
import { RecoveryPump } from './recovery/pump.js';

const context = BACKGROUND_CONTEXT;

export class SessionService implements AgentApplication {
  readonly store: SessionStore;
  readonly conversations: ConversationStore;
  private readonly backoff: RecoveryStore;
  private readonly controls: SessionControls;
  private readonly recovery: RecoveryPump;
  private active = new Map<string, SessionRuntime>();
  private opening = new Map<string, Promise<SessionRuntime>>();
  private jobs = new Map<string, Promise<void>>();
  private stopping = false;
  private closing: Promise<void> | undefined;
  constructor(
    private pool: Pool,
    private business: BusinessClient,
    private options: {
      leaseMs: number;
      onRecoveryEvent?: (event: RecoveryEvent) => void;
    } & DeliveryOptions,
  ) {
    this.store = new SessionStore(pool);
    this.backoff = new RecoveryStore(pool);
    this.conversations = new ConversationStore(pool, this.store);
    this.controls = new SessionControls(
      this.store,
      business,
      this.backoff,
      (run) => this.runtime(run),
      (run) => this.launch(run),
    );
    this.recovery = new RecoveryPump(this.backoff, this.store, {
      command: (id, input) => this.command(id, input),
      launch: (run) => this.launch(run),
      onEvent: options.onRecoveryEvent,
    });
  }
  async ready() {
    try {
      await queries(this.pool)
        .selectFrom('service_sessions as sessions')
        .leftJoin('service_runs as runs', 'runs.session_id', 'sessions.id')
        .leftJoin('public_messages as messages', 'messages.run_id', 'runs.id')
        .leftJoin(
          'recovery_backoff as backoff',
          'backoff.root_run_id',
          'runs.id',
        )
        .select([
          'sessions.id',
          'sessions.pinned',
          'sessions.archived_at',
          'runs.input',
          'runs.retry_of_run_id',
          'messages.message_id',
          'backoff.attempts',
        ])
        .limit(0)
        .execute();
      return !this.stopping;
    } catch {
      return false;
    }
  }
  createSession(input: CreateSessionInput) {
    if (this.stopping) throw new AgentError('shutting_down');
    return this.store.createSession(input);
  }
  updateSession(id: string, input: UpdateSessionInput) {
    if (this.stopping) throw new AgentError('shutting_down');
    return this.conversations.updateSession(id, input);
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
        const last = await database(client).serviceRun.findFirst({
          where: { session_id: id },
          orderBy: { accepted_revision: 'desc' },
        });
        const run = last
          ? await this.store.snapshot(runRow(last), client)
          : undefined;
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
      {
        ...this.options,
        onCloseFailure: () =>
          this.recovery.failed(
            RecoveryKind.Run,
            run.id,
            run.id,
            RecoveryCode.CloseFailed,
          ),
      },
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
    if (
      this.stopping ||
      this.jobs.has(run.id) ||
      this.jobs.size >= runtimeDefaults.maxConcurrentRoots
    )
      return;
    const job = (async () => {
      const runtime = await this.runtime(run);
      await runtime.start();
      await this.backoff.succeeded(RecoveryKind.Run, run.id);
    })()
      .catch(async () => {
        await this.recovery.failed(
          RecoveryKind.Run,
          run.id,
          run.id,
          RecoveryCode.ExecutionUnavailable,
        );
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
            .catch(() =>
              this.recovery.failed(
                RecoveryKind.Run,
                run.id,
                run.id,
                RecoveryCode.ExecutionUnavailable,
              ),
            );
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
            .catch(() =>
              this.recovery.failed(
                RecoveryKind.Run,
                run.id,
                run.id,
                RecoveryCode.CloseFailed,
              ),
            );
        }
        if (current?.isClosed && this.active.get(run.session_id) === current)
          this.active.delete(run.session_id);
        this.jobs.delete(run.id);
        // A control may submit its continuation before the previous drive has returned.
        if (continueRun) this.launch(run);
      });
    this.jobs.set(run.id, job);
  }
  async waitForRun(id: string) {
    await this.jobs.get(id);
  }
  command(id: string, input: ControlInput) {
    if (this.stopping) return Promise.reject(new AgentError('shutting_down'));
    return this.controls.command(id, input);
  }

  async events(
    id: string,
    who: Identity,
    after: string | undefined,
  ): Promise<RuntimeEvent[]> {
    const run = await this.store.run(id, who);
    if (!run) throw new AgentError('run_not_found');
    return readRunEvents(this.pool, run, after);
  }

  get recoveryStatus() {
    return this.recovery.status;
  }

  recoverOnce() {
    return this.recovery.scanOnce();
  }

  startRecovery() {
    this.recovery.start();
  }

  stopAccepting() {
    this.stopping = true;
    this.recovery.stop();
  }

  close(): Promise<void> {
    this.stopAccepting();
    this.closing ??= this.drain();
    return this.closing;
  }

  private async drain() {
    await this.recovery.close();
    await Promise.allSettled([...this.opening.values()]);
    const closed = await Promise.allSettled(
      [...this.active.values()].map((runtime) => runtime.close()),
    );
    await Promise.allSettled([...this.jobs.values()]);
    this.active.clear();
    if (closed.some((result) => result.status === 'rejected'))
      throw new AggregateError(
        closed
          .filter((result) => result.status === 'rejected')
          .map(() => new Error(RecoveryCode.CloseFailed)),
        'Execution shutdown failed',
      );
  }
}
