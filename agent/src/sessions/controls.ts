import { BACKGROUND_CONTEXT as context } from '@earendil-works/chord/context';
import type { BusinessClient } from '../business/client.js';
import {
  StateDoc,
  RunStatus,
  releaseOwnedAllocations,
} from '../harness/state-doc.js';
import { AgentError } from './session-errors.js';
import { stableId } from './manifest.js';
import type { RunRow, SessionStore } from './session-store.js';
import { RecoveryKind, type RecoveryStore } from './recovery-store.js';
import type { ControlInput } from './types.js';
import type { SessionRuntime } from '../harness/runtime.js';

export class SessionControls {
  constructor(
    private readonly store: SessionStore,
    private readonly business: BusinessClient,
    private readonly backoff: RecoveryStore,
    private readonly runtime: (run: RunRow) => Promise<SessionRuntime>,
    private readonly launch: (run: RunRow) => void,
  ) {}

  async command(id: string, input: ControlInput) {
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
          const resumingDelivery =
            input.action === 'resume' &&
            state.submission_status === RunStatus.Completed &&
            state.delivery_wait !== null;
          if (!resumingDelivery)
            await runtime.root.submit(tx, {
              type: 'input',
              content:
                'Continue the unfinished adopted plan under the current approval. Query original business receipts before generating or writing again.',
              requestId: continuation,
              whenBusy: 'reject',
            });
          state.segment_id = continuation;
          state.status = resumingDelivery
            ? RunStatus.WaitingDelivery
            : RunStatus.Running;
          state.pending_proposal = null;
          state.pending_production_continuation = null;
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
    await this.store.recordControlReceipt(input.decision_id, receipt);
    await this.business.receipt(
      input.root_run_id,
      receipt as Parameters<BusinessClient['receipt']>[1],
    );
    if (input.action === 'resume' && receipt.status === 'applied')
      await this.backoff.succeeded(RecoveryKind.Run, input.root_run_id);
    return receipt;
  }
}
