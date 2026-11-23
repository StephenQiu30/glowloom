import { recoveryDefaults } from '../../config.js';
import { RunStatus } from '../../pi-durable/docs.js';
import { hasCurrentDefinitions } from '../manifest.js';
import type { RunRow, SessionStore } from '../persistence/store.js';
import type { ControlInput } from '../types.js';
import {
  RecoveryCode,
  RecoveryKind,
  type RecoveryEvent,
  type RecoveryStore,
} from './store.js';

/** Finds durable work and hands it back to the session's existing execution entries. */
export class RecoveryPump {
  readonly status = {
    last_success_at: null as string | null,
    consecutive_failures: 0,
  };
  private stopping = false;
  private timer: NodeJS.Timeout | undefined;
  private inFlight: Promise<void> | undefined;

  constructor(
    private readonly backoff: RecoveryStore,
    private readonly store: SessionStore,
    private readonly options: {
      command: (id: string, input: ControlInput) => Promise<unknown>;
      launch: (run: RunRow) => void;
      onEvent?: ((event: RecoveryEvent) => void) | undefined;
    },
  ) {}

  start() {
    if (this.timer || this.stopping) return;
    this.timer = setInterval(
      () => void this.scanOnce(),
      recoveryDefaults.pollMs,
    );
    this.timer.unref();
    void this.scanOnce();
  }

  async scanOnce() {
    if (this.stopping) return;
    this.inFlight ??= this.scan().finally(() => {
      this.inFlight = undefined;
    });
    await this.inFlight;
  }

  stop() {
    this.stopping = true;
    if (this.timer) clearInterval(this.timer);
  }

  async close() {
    this.stop();
    await this.inFlight;
  }

  async failed(
    kind: RecoveryKind,
    subject: string,
    root: string,
    code: RecoveryCode,
    terminal = false,
  ) {
    this.options.onEvent?.({
      kind,
      subject_id: subject,
      root_run_id: root,
      code,
    });
    try {
      await this.backoff.failed(kind, subject, root, code, terminal);
    } catch {
      this.options.onEvent?.({
        kind: 'scan',
        code: RecoveryCode.ScanUnavailable,
      });
    }
  }

  private async scan() {
    try {
      await this.scanRecovery();
      this.status.last_success_at = new Date().toISOString();
      this.status.consecutive_failures = 0;
    } catch {
      this.status.consecutive_failures++;
      this.options.onEvent?.({
        kind: 'scan',
        code: RecoveryCode.ScanUnavailable,
        consecutive_failures: this.status.consecutive_failures,
      });
    }
  }

  private async scanRecovery() {
    const controls = await this.backoff.pendingControls();
    for (const row of controls) {
      if (this.stopping) return;
      try {
        await this.options.command(row.input.root_run_id, row.input);
        await this.backoff.succeeded(
          RecoveryKind.Control,
          row.input.decision_id,
        );
      } catch {
        await this.failed(
          RecoveryKind.Control,
          row.input.decision_id,
          row.input.root_run_id,
          RecoveryCode.ControlUnavailable,
        );
      }
    }
    if (this.stopping) return;
    const runs = await this.backoff.pendingRuns();
    for (const run of runs) {
      if (this.stopping) return;
      if (!hasCurrentDefinitions(run.manifest) || !run.input) {
        await this.failed(
          RecoveryKind.Run,
          run.id,
          run.id,
          RecoveryCode.DefinitionsUnavailable,
          true,
        );
        continue;
      }
      try {
        const snapshot = await this.store.snapshot(run);
        if (
          (
            [
              RunStatus.Accepted,
              RunStatus.Running,
              RunStatus.WaitingDelivery,
            ] as string[]
          ).includes(snapshot.state.status)
        )
          this.options.launch(run);
      } catch {
        await this.failed(
          RecoveryKind.Run,
          run.id,
          run.id,
          RecoveryCode.ExecutionUnavailable,
        );
      }
    }
  }
}
