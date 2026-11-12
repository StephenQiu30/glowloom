import { authorizedText } from './helpers/conversation-input.js';
import { randomUUID } from 'node:crypto';
import { expect, it, vi } from 'vitest';
import { SessionRuntime } from '../src/sessions/runtime.js';
import { SessionService } from '../src/sessions/service.js';
import {
  BusinessClient,
  BusinessError,
  type Grant,
  type BusinessDecision,
  type CallReceipt,
} from '../src/business/client.js';
import { BACKGROUND_CONTEXT as context } from '@earendil-works/chord/context';
import { StateDoc } from '../src/pi-durable/docs.js';
import {
  RecoveryStore,
  RecoveryKind,
  RecoveryCode,
} from '../src/sessions/recovery.js';
import { testPool } from './storage/database.js';

it.each([
  'normal',
  'restart',
  'prior_job_pending',
  'quiesced_reply_lost',
  'production_approval',
  'parked_resume',
])(
  'quiesces an actual proposal round, admits one approved continuation after mode=%s, and never revives an aborted submission',
  async (mode) => {
    let releaseDrive!: () => void, driveDone!: () => void;
    const held = new Promise<void>((resolve) => {
      releaseDrive = resolve;
    });
    const firstDrive = new Promise<void>((resolve) => {
      driveDone = resolve;
    });
    let first = true;
    const originalProposal = SessionRuntime.prototype.proposal;
    const proposalSpy =
      mode === 'production_approval'
        ? vi
            .spyOn(SessionRuntime.prototype, 'proposal')
            .mockImplementation(async function (this: SessionRuntime, ...args) {
              await this.harness.commit(async (tx) => {
                const state = await tx.doc(StateDoc, this.root.id);
                state.pending_production_continuation = randomUUID();
              }, context);
              await originalProposal.apply(this, args);
              expect(
                (await this.state())?.pending_production_continuation,
              ).toBeNull();
            })
        : undefined;
    const originalStart = SessionRuntime.prototype.start;
    const spy =
      mode === 'prior_job_pending'
        ? vi
            .spyOn(SessionRuntime.prototype, 'start')
            .mockImplementation(async function (this: SessionRuntime) {
              const result = await originalStart.call(this);
              if (first) {
                first = false;
                driveDone();
                await held;
              }
              return result;
            })
        : undefined;
    const pool = testPool();
    const scope = {
      actor_id: randomUUID(),
      org_id: randomUUID(),
      scope_id: randomUUID(),
      project_id: randomUUID(),
    };
    class Fixture extends BusinessClient {
      revision = 0;
      barrier = false;
      current: BusinessDecision | null = null;
      models = 0;
      quiescedCount = 0;
      receipts = 0;
      ownerEpoch = 0;
      remoteSegment: string | undefined;
      constructor() {
        super('http://127.0.0.1:1', 'fixture');
      }
      override async tools() {
        return [
          {
            name: 'make_proposal',
            description: 'Propose a paid operation',
            parameters: {
              type: 'object',
              properties: {},
              additionalProperties: false,
            },
            contract_version: '1',
            replay: 'safe' as const,
            effect: 'proposal' as const,
          },
        ];
      }
      override async decisions() {
        return {
          current: this.current,
          authorization_revision: this.revision,
          barrier: this.barrier,
          terminal: false,
        };
      }
      override async grant(
        input: Parameters<BusinessClient['grant']>[0],
      ): Promise<Grant> {
        if (
          !this.barrier &&
          this.remoteSegment &&
          input.submission_id !== this.remoteSegment
        )
          throw new BusinessError(403, 'agent_grant_invalid');
        this.ownerEpoch = input.owner_epoch;
        if (this.barrier)
          throw new BusinessError(409, 'agent_authorization_blocked');
        return {
          ...input,
          grant_id: randomUUID(),
          available_micros: 1000000,
          expires_at: input.lease_until,
          scope,
        };
      }
      override async validate(grant: Grant) {
        if (this.barrier || grant.authorization_revision !== this.revision)
          throw new BusinessError(409, 'blocked');
        return grant;
      }
      override async snapshot() {
        return { revision: 1 };
      }
      override async findCall() {
        return undefined;
      }
      override async receipt() {
        this.receipts++;
        return { accepted: true };
      }
      override async quiesced(
        run: string,
        input: Parameters<BusinessClient['quiesced']>[1],
      ) {
        if (input.owner_epoch !== this.ownerEpoch)
          throw new BusinessError(409, 'owner_epoch_conflict');
        this.quiescedCount++;
        this.barrier = false;
        this.remoteSegment = input.continuation_id;
        const grant = await this.grant({
          root_run_id: run,
          submission_id: input.continuation_id,
          owner_epoch: input.owner_epoch,
          authorization_revision: input.authorization_revision,
          lease_until: input.lease_until,
        });
        if (mode === 'quiesced_reply_lost' && this.quiescedCount === 1)
          throw new BusinessError(503, 'fixture_reply_lost');
        return grant;
      }
      override async execute(
        ...args: Parameters<BusinessClient['execute']>
      ): Promise<CallReceipt> {
        const [kind, , grant, call] = args;
        let result: unknown;
        if (kind === 'model') {
          this.models++;
          result = {
            output: this.models === 1 ? '需批准生成' : '已按批准接续',
            ...(this.models === 1
              ? {
                  tool_calls: [
                    { id: 'propose', name: 'make_proposal', arguments: {} },
                  ],
                }
              : {}),
            usage: { input_tokens: 1, output_tokens: 1 },
            model_key: 'test',
            stop_reason: this.models === 1 ? 'tool_use' : 'stop',
          };
        } else {
          this.barrier = true;
          result = {
            proposal_id: randomUUID(),
            authorization_revision: this.revision,
            control: { terminate: true },
          };
        }
        return {
          receipt_id: randomUUID(),
          root_run_id: grant.root_run_id,
          submission_id: grant.submission_id,
          call,
          status: 'completed',
          cost_micros: 0,
          result,
        };
      }
    }
    const fixture = new Fixture();
    let service = new SessionService(pool, fixture, { leaseMs: 30000 });
    try {
      const session = await service.createSession({
        ...scope,
        request_id: randomUUID(),
        content_hash: 'a'.repeat(64),
        title: '审批',
      });
      const run = await service.createRun(session.id, {
        input: authorizedText('执行需批准的生成'),
        ...scope,
        request_id: randomUUID(),
        content_hash: 'b'.repeat(64),
        goal: '执行需批准的生成',
        expected_execution_revision: 0,
        manifest: { model_key: 'test' },
      });
      if (mode === 'prior_job_pending') await firstDrive;
      else await service.waitForRun(run.root_run_id);
      expect(
        (await service.readRun(run.root_run_id, scope))?.state.status,
      ).toBe('waiting_approval');
      if (mode === 'restart') {
        await service.close();
        service = new SessionService(pool, fixture, { leaseMs: 30000 });
      }
      fixture.revision = 1;
      fixture.current = {
        ...scope,
        decision_id: randomUUID(),
        root_run_id: run.root_run_id,
        session_id: session.id,
        authorization_revision: 1,
        action: 'approve',
        content_hash: 'c'.repeat(64),
      };
      if (mode === 'parked_resume') {
        const backoff = new RecoveryStore(pool);
        for (let i = 0; i < 5; i++)
          await backoff.failed(
            RecoveryKind.Run,
            run.root_run_id,
            run.root_run_id,
            RecoveryCode.ExecutionUnavailable,
          );
        expect(
          (await service.readRun(run.root_run_id, scope))?.execution_available,
        ).toBe(true);
        fixture.current = { ...fixture.current, action: 'pause' };
        await service.command(run.root_run_id, fixture.current);
        expect(
          (await service.readRun(run.root_run_id, scope))?.state.status,
        ).toBe('paused');
        fixture.revision = 2;
        fixture.current = {
          ...fixture.current,
          decision_id: randomUUID(),
          action: 'resume',
          authorization_revision: 2,
        };
      }
      if (mode === 'quiesced_reply_lost') {
        await expect(
          service.command(run.root_run_id, fixture.current),
        ).rejects.toMatchObject({ code: 'fixture_reply_lost' });
        await service.close();
        service = new SessionService(pool, fixture, { leaseMs: 30000 });
      }
      const applied = await service.command(run.root_run_id, fixture.current);
      releaseDrive();
      for (
        let i = 0;
        i < 200 &&
        (await service.readRun(run.root_run_id, scope))?.state.status !==
          'completed';
        i++
      )
        await new Promise((r) => setTimeout(r, 10));
      await service.waitForRun(run.root_run_id);
      expect(
        (await service.readRun(run.root_run_id, scope))?.state.status,
      ).toBe('completed');
      if (mode === 'parked_resume')
        for (let i = 0; i < 5; i++)
          await new RecoveryStore(pool).failed(
            RecoveryKind.Run,
            run.root_run_id,
            run.root_run_id,
            RecoveryCode.ExecutionUnavailable,
          );
      const duplicate = await service.command(run.root_run_id, fixture.current);
      expect(duplicate).toEqual(applied);
      expect(fixture.quiescedCount).toBe(
        mode === 'quiesced_reply_lost' ? 2 : 1,
      );
      expect(fixture.models).toBe(2);
      if (mode === 'parked_resume')
        expect(await new RecoveryStore(pool).read(run.root_run_id)).toEqual([]);
      const inputs = await pool.query(
        "SELECT count(*) FROM agent.runtime_records WHERE session_id=$1 AND record_type='submission' AND request_key IS NOT NULL",
        [session.id],
      );
      expect(Number(inputs.rows[0].count)).toBe(2);
    } finally {
      releaseDrive();
      spy?.mockRestore();
      proposalSpy?.mockRestore();
      await service.close();
      await pool.end();
    }
  },
);
