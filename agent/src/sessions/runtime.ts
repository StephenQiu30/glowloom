import { reconcileInterrupted } from './reconciliation.js';
import { resolve } from 'node:path';
import { loadTrustedExtensions } from '../pi-durable/trusted-extensions.js';
import { randomUUID } from 'node:crypto';
import type { Pool } from 'pg';
import {
  Harness,
  configure,
  ROOT_CONVERSATION_ID,
  type Conversation,
  type JsonObject,
} from '@earendil-works/pi-durable';
import { BACKGROUND_CONTEXT } from '@earendil-works/chord/context';
import { openPostgresStorage } from '../pi-durable/storage/postgres.js';
import {
  claimSessionOwner,
  renewSessionOwner,
  releaseSessionOwner,
  type SessionOwner,
} from '../pi-durable/storage/ownership.js';
import {
  BusinessClient,
  BusinessError,
  type Grant,
  type CallIdentity,
} from '../business/client.js';
import type { RunRow } from './persistence/store.js';
import { StateDoc, releaseOwnedAllocations } from '../pi-durable/docs.js';
import { PhaseTelemetry } from '../pi-durable/telemetry.js';
import { createBusinessModels } from '../pi-durable/model-provider.js';
import { buildRegistry, type HarnessGate } from '../pi-durable/registry.js';
import { eventProjection } from '../pi-durable/events.js';
import { canonicalHash, hasCurrentDefinitions } from './manifest.js';
import { resolveConversationContent } from './input.js';
const context = BACKGROUND_CONTEXT;

export class SessionRuntime {
  private harnessValue!: Harness;
  root!: Conversation;
  owner!: SessionOwner;
  private activeGrant: Grant | undefined;
  private trusted:
    Awaited<ReturnType<typeof loadTrustedExtensions>> | undefined;
  private stopping = false;
  private renewal: NodeJS.Timeout | undefined;
  private closing: Promise<void> | undefined;
  private running: Promise<void> | undefined;
  private controlLine: Promise<unknown> = Promise.resolve();
  private proposalLine: Promise<void> | undefined;
  private segment: string;
  private inputRead: AbortController | undefined;
  private constructor(
    readonly pool: Pool,
    readonly business: BusinessClient,
    readonly run: RunRow,
    readonly leaseMs: number,
  ) {
    this.segment = run.submission_id;
  }
  static async open(
    pool: Pool,
    business: BusinessClient,
    run: RunRow,
    leaseMs: number,
  ) {
    if (!hasCurrentDefinitions(run.manifest))
      throw new Error('Pinned Harness definitions are unavailable');
    const runtime = new SessionRuntime(pool, business, run, leaseMs);
    runtime.owner = await claimSessionOwner(
      pool,
      run.session_id,
      randomUUID(),
      leaseMs,
    );
    try {
      const docs = await pool.query<{ id: string }>(
        "SELECT id FROM agent.runtime_records WHERE session_id=$1 AND record_type='document' AND kind_key=$2 AND retired_at_seq IS NULL",
        [run.session_id, JSON.stringify('glowloom.agent')],
      );
      const storage = await openPostgresStorage(
        pool,
        run.session_id,
        runtime.owner,
        {
          projectEvents: eventProjection(
            run.id,
            docs.rows.map((r) => Number(r.id)),
          ),
        },
      );
      let rootId = ROOT_CONVERSATION_ID;
      const gate: HarnessGate = {
        manifest: run.manifest,
        harness: () => runtime.harnessValue,
        rootId: () => rootId,
        grant: () => runtime.grant(),
        assertActive: () => runtime.assertActive(),
        assertSources: () => runtime.assertSources(),
        reserve: (call, kind, origin) => runtime.reserve(call, kind, origin),
        proposal: (id, task, details) => runtime.proposal(id, task, details),
      };
      runtime.trusted = await loadTrustedExtensions({
        imageRoot: resolve(process.cwd(), 'trusted'),
        entries: run.manifest.trusted_extensions,
      });
      const { models } = createBusinessModels(business, run.manifest, gate);
      const registry = buildRegistry(business, gate, runtime.trusted.tools);
      runtime.harnessValue = await Harness.open(
        storage,
        {
          models,
          registry: registry.registry,
          telemetry: new PhaseTelemetry(),
          onReport: (report) => {
            if (report.kind === 'storage_failed') runtime.sealFailure();
          },
          settings: {
            retry: { maxRetries: 0 },
            toolExecution: 'sequential',
            stream: { timeoutMs: 180000 },
            compaction: { enabled: true, backgroundTokens: 0 },
            progress: {
              partialIntervalMs: 100,
              outputIntervalMs: 100,
              persistWithinMs: 1000,
            },
          },
        },
        context,
      );
      runtime.root = await runtime.harnessValue.root(context, {
        agent: {
          model: { provider: 'glowloom', modelId: run.manifest.model_key },
          extensions: registry.rootExtensions,
          tools: registry.rootTools,
          maxTurns: 40,
          instructions:
            'You are the creative producer. Maintain a concrete versioned plan, delegate professional work, read real evidence, and use authorized business tools. A plan or text answer never means media was generated or approved. Request a business proposal before expensive or destructive work.',
        },
      });
      rootId = runtime.root.id;
      await runtime.harnessValue.commit(async (tx) => {
        const state = await tx.doc(StateDoc, rootId);
        if (state.root_run_id === run.id) {
          if (state.manifest_hash !== run.manifest.sha256)
            throw new Error('Execution manifest mismatch');
          runtime.segment = state.segment_id;
        } else {
          if (
            state.root_run_id &&
            !['completed', 'failed', 'stopped', 'rejected'].includes(
              state.status,
            )
          )
            throw new Error('Another run is active');
          await configure(tx, rootId, {
            model: { provider: 'glowloom', modelId: run.manifest.model_key },
            extensions: registry.rootExtensions,
            tools: registry.rootTools,
            maxTurns: 40,
          });
          Object.assign(state, {
            root_run_id: run.id,
            segment_id: run.submission_id,
            manifest_hash: run.manifest.sha256,
            status: 'accepted',
            execution_revision: Number(run.accepted_revision),
            authorization_revision: 0,
            model_steps: 0,
            tool_steps: 0,
            goal: run.goal,
            plan: [],
            adopted: [],
            skills: run.manifest.skills,
            pending_proposal: null,
            applied_decisions: [],
            quiesced: null,
            reserved_calls: [],
            delegations: [],
            loaded_skills: [],
            model_contexts: [],
          });
        }
      }, context);
      runtime.renewal = setInterval(
        () => {
          void runtime.renew().catch(() => runtime.close());
        },
        Math.max(500, Math.floor(leaseMs / 3)),
      );
      runtime.renewal.unref();
      return runtime;
    } catch (error) {
      await runtime.close().catch(() => {});
      throw error;
    }
  }
  get isClosed() {
    return this.stopping;
  }
  get harness() {
    return this.harnessValue;
  }
  grant() {
    if (!this.activeGrant) throw new Error('No current business grant');
    return this.activeGrant;
  }
  async state() {
    return this.harnessValue.snapshot(StateDoc, this.root.id, context);
  }
  epoch() {
    const value = Number(this.owner.epoch);
    if (!Number.isSafeInteger(value))
      throw new Error('Epoch exceeds safe HTTP range');
    return value;
  }
  async authorize() {
    const decision = await this.business.decisions(this.run.id);
    if (decision.terminal || decision.barrier)
      throw new Error('Business run is blocked');
    this.activeGrant = await this.business.grant({
      root_run_id: this.run.id,
      submission_id: this.segment,
      owner_epoch: this.epoch(),
      authorization_revision: decision.authorization_revision,
      lease_until: this.owner.leaseUntil.toISOString(),
    });
    await this.harnessValue.commit(async (tx) => {
      const state = await tx.doc(StateDoc, this.root.id);
      if (
        state.authorization_revision !==
        this.activeGrant!.authorization_revision
      ) {
        state.authorization_revision = this.activeGrant!.authorization_revision;
        state.execution_revision++;
      }
    }, context);
  }
  async announceControlOwner(
    authorizationRevision: number,
    submissionId = this.segment,
  ) {
    this.activeGrant = undefined;
    try {
      // A blocked registration commits the new fencing epoch without issuing a grant.
      await this.business.grant({
        root_run_id: this.run.id,
        submission_id: submissionId,
        owner_epoch: this.epoch(),
        authorization_revision: authorizationRevision,
        lease_until: this.owner.leaseUntil.toISOString(),
      });
    } catch (error) {
      if (!(
        error instanceof BusinessError &&
        error.status === 409 &&
        error.code === 'agent_authorization_blocked'
      ))
        throw error;
    }
  }
  async renew() {
    if (this.stopping) return;
    this.owner = await renewSessionOwner(
      this.pool,
      this.run.session_id,
      this.owner,
      this.leaseMs,
    );
    if (this.activeGrant) {
      const current = await this.business.decisions(this.run.id);
      if (current.barrier || current.terminal) {
        this.activeGrant = undefined;
        return;
      }
      await this.authorize();
    }
  }
  async assertSources() {
    // The official transcript may contain inputs from any prior accepted round.
    // Recheck authority without submitting or decoding those inputs again.
    const history = await this.pool.query<{
      input: import('./types.js').AuthorizedInput;
    }>(
      'SELECT input FROM agent.service_runs WHERE session_id=$1 AND accepted_revision<=$2 AND input IS NOT NULL ORDER BY accepted_revision',
      [this.run.session_id, this.run.accepted_revision],
    );
    for (const { input } of history.rows) {
      const refs = input.draft.references;
      if (
        (Array.isArray(refs) && refs.length > 0) ||
        input.media_pins.length > 0
      )
        try {
          await this.business.validateInputSources(this.grant(), input.draft);
        } catch (error) {
          if (
            error instanceof BusinessError &&
            error.code === 'conversation_reference_unavailable'
          ) {
            await this.harnessValue.commit(async (tx) => {
              const state = await tx.doc(StateDoc, this.root.id);
              state.failure = { code: 'conversation_reference_unavailable' };
            }, context);
          }
          throw error;
        }
    }
  }
  async assertActive() {
    if (this.stopping) throw new Error('Runtime is closing');
    const state = await this.state();
    if (!state || !['accepted', 'running'].includes(state.status))
      throw new Error('Run is not accepting new work');
    const grant = this.grant();
    if (
      grant.submission_id !== this.segment ||
      new Date(grant.expires_at).getTime() <= Date.now()
    )
      throw new Error('Execution segment grant expired');
    await this.business.validate(grant);
  }
  async reserve(
    call: CallIdentity,
    kind: 'model' | 'tool',
    origin: 'business' | 'local' = 'business',
  ) {
    await this.harnessValue.commit(async (tx) => {
      const state = await tx.doc(StateDoc, this.root.id);
      const entries = (state.reserved_calls ?? []) as JsonObject[];
      const key = canonicalHash({
        task: call.task_id,
        call: call.call_id,
        checkpoint: call.checkpoint_id,
        attempt: call.attempt,
        segment: this.segment,
      });
      const prior = entries.find((x) => x.key === key);
      if (prior) {
        if (prior.hash !== call.input_hash)
          throw new Error('Call identity conflict');
        return;
      }
      const field = kind === 'model' ? 'model_steps' : 'tool_steps';
      if (state[field] >= 40) throw new Error('Run step budget exceeded');
      const taskId = Number(call.task_id);
      if (!Number.isSafeInteger(taskId))
        throw new Error('Invalid task identity');
      const task = await tx.task(taskId as Parameters<Harness['getTask']>[0]);
      if (!task) throw new Error('Unknown tool/model task');
      const child = state.delegations.find(
        (item) =>
          item.conversation_id === task.conversationId &&
          item.status === 'running',
      );
      const budget = kind === 'model' ? 'model_budget' : 'tool_budget',
        used = kind === 'model' ? 'model_used' : 'tool_used';
      if (child) {
        if (Number(child[used] ?? 0) >= Number(child[budget] ?? 0))
          throw new Error('Specialist budget exceeded');
        child[used] = Number(child[used] ?? 0) + 1;
      } else {
        if (task.conversationId !== this.root.id)
          throw new Error('Specialist allocation is unavailable');
        const held = state.delegations
          .filter((item) => item.status === 'running')
          .reduce(
            (sum, item) =>
              sum +
              Math.max(0, Number(item[budget] ?? 0) - Number(item[used] ?? 0)),
            0,
          );
        if (state[field] + held >= 40)
          throw new Error('Root budget is allocated to running specialists');
      }
      state[field]++;
      entries.push({
        key,
        hash: call.input_hash,
        kind,
        origin,
        segment_id: this.segment,
        call: { ...call },
      });
      state.reserved_calls = entries;
    }, context);
  }
  async start() {
    if (this.running) return this.running;
    this.running = this.drive().finally(() => {
      this.running = undefined;
    });
    return this.running;
  }
  private async drive() {
    const state = await this.state();
    if (!state || !['accepted', 'running'].includes(state.status)) return;
    await this.authorize();
    const segment = this.segment;
    await this.harnessValue.commit(async (tx) => {
      const s = await tx.doc(StateDoc, this.root.id);
      s.status = 'running';
      s.execution_revision++;
    }, context);
    const initialContent =
      this.segment === this.run.submission_id && this.run.input
        ? await resolveConversationContent(
            this.run.input,
            async (pin) => {
              await this.assertActive();
              const input = {
                asset_id: pin.sourceId,
                revision: Number(pin.sourceVersion),
                ...(this.run.manifest.tools.some(
                  (tool) => tool.name === 'media_reference_inspect',
                )
                  ? { media_kind: pin.mediaKind }
                  : {}),
              };
              const multimedia = 'media_kind' in input;
              if (!multimedia && pin.mediaKind !== 'image')
                throw new Error(
                  'Pinned media inspection capability is unavailable',
                );
              const identity: CallIdentity = {
                task_id: 'user-input',
                call_id: pin.sourceId,
                checkpoint_id: this.run.input!.input_hash,
                attempt: 1,
                contract_version: 'v1',
                input_hash: canonicalHash(input),
              };
              // Always execute the authorized read endpoint: it revalidates cached
              // image receipts too. A generic receipt lookup would skip that check.
              const controller = new AbortController();
              this.inputRead = controller;
              let receipt: Awaited<ReturnType<BusinessClient['execute']>>;
              try {
                receipt = await this.business.execute(
                  'tool',
                  multimedia
                    ? 'media_reference_inspect'
                    : 'media_image_inspect',
                  this.grant(),
                  identity,
                  input,
                  0,
                  controller.signal,
                );
                await this.assertActive();
              } finally {
                if (this.inputRead === controller) this.inputRead = undefined;
              }
              if (
                receipt.status !== 'completed' ||
                receipt.root_run_id !== this.run.id ||
                receipt.submission_id !== this.segment ||
                receipt.call.input_hash !== identity.input_hash
              )
                throw new Error('Input image requires reconciliation');
              return receipt.result;
            },
            Math.max(
              1,
              this.run.manifest.context_window -
                this.run.manifest.max_tokens -
                4096,
            ),
          )
        : this.run.goal;
    const submission = await this.root.submit(
      {
        type: 'input',
        content:
          this.segment === this.run.submission_id
            ? initialContent
            : 'Continue the unfinished adopted plan. Verify current business receipts before any new action.',
        requestId: this.segment,
      },
      context,
    );
    const settled = await submission.wait(context);
    let recoveredStatus: 'completed' | 'needs_reconciliation' | undefined;
    if (settled.status === 'unanswered' && settled.reason === 'interrupted') {
      try {
        recoveredStatus = (
          await reconcileInterrupted({
            harness: this.harnessValue,
            root: this.root,
            business: this.business,
            runId: this.run.id,
            segmentId: segment,
          })
        ).status;
      } catch {
        recoveredStatus = 'needs_reconciliation';
      }
    }
    if (this.proposalLine) await this.proposalLine;
    await this.harnessValue.commit(async (tx) => {
      const s = await tx.doc(StateDoc, this.root.id);
      if (s.status !== 'running' || s.segment_id !== segment) return;
      s.status = s.pending_proposal
        ? 'waiting_approval'
        : (recoveredStatus ??
          (settled.status === 'done' ? 'completed' : 'failed'));
      s.execution_revision++;
      if (settled.status === 'unanswered' && recoveredStatus !== 'completed')
        s.failure = {
          ...(s.failure &&
          typeof s.failure === 'object' &&
          !Array.isArray(s.failure)
            ? s.failure
            : {}),
          reason: settled.reason,
        };
    }, context);
  }
  async proposal(id: string, taskId: number, details: JsonObject) {
    await this.harnessValue.commit(async (tx) => {
      const state = await tx.doc(StateDoc, this.root.id);
      state.pending_proposal = id;
      const safe = { ...details };
      delete safe.grant;
      state.proposal = safe;
      if (typeof details.authorization_revision === 'number')
        state.authorization_revision = details.authorization_revision;
      state.status = 'waiting_approval';
      state.execution_revision++;
    }, context);
    this.activeGrant = undefined;
    if (!this.proposalLine)
      this.proposalLine = (async () => {
        for (let i = 0; i < 500 && !this.stopping; i++) {
          const task = await this.harnessValue.getTask(
            taskId as Parameters<Harness['getTask']>[0],
            context,
          );
          if (!task || task.state.status === 'terminal') break;
          await new Promise((resolve) => setTimeout(resolve, 10));
        }
        if (!this.stopping) {
          await this.root.abort(context);
          await this.harnessValue.commit(async (tx) => {
            const state = await tx.doc(StateDoc, this.root.id);
            releaseOwnedAllocations(state, 'proposal_barrier');
          }, context);
        }
      })();
  }
  serialize<T>(work: () => Promise<T>): Promise<T> {
    const next = this.controlLine.then(work, work);
    this.controlLine = next.catch(() => {});
    return next;
  }
  setGrant(grant: Grant) {
    this.activeGrant = grant;
    this.segment = grant.submission_id;
  }
  block() {
    this.activeGrant = undefined;
    this.inputRead?.abort();
  }
  private sealFailure() {
    this.stopping = true;
    this.activeGrant = undefined;
    if (this.renewal) clearInterval(this.renewal);
    setImmediate(() => {
      void this.close().catch(() => {});
    });
  }
  async close() {
    if (this.closing) return this.closing;
    this.stopping = true;
    this.inputRead?.abort();
    if (this.renewal) clearInterval(this.renewal);
    this.activeGrant = undefined;
    this.closing = (async () => {
      await this.harnessValue?.close(context).catch(() => {});
      await this.trusted?.close();
      if (this.owner)
        await releaseSessionOwner(this.pool, this.run.session_id, this.owner);
    })();
    return this.closing;
  }
}
