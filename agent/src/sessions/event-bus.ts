import { EventEmitter } from 'node:events';

/**
 * In-process run event notification bus.
 * Wakes active SSE and gRPC stream subscribers immediately upon storage commit,
 * falling back to bounded polling for cross-process or keepalive ticks.
 */
export class RunEventBus {
  private readonly emitter = new EventEmitter();

  constructor() {
    this.emitter.setMaxListeners(0);
  }

  notify(runId: string): void {
    this.emitter.emit(runId);
  }

  waitForUpdate(
    runId: string,
    timeoutMs: number,
    signal?: AbortSignal,
  ): Promise<void> {
    if (signal?.aborted) {
      return Promise.reject(
        signal.reason ?? new DOMException('Aborted', 'AbortError'),
      );
    }
    return new Promise<void>((resolve, reject) => {
      const cleanup = () => {
        clearTimeout(timer);
        this.emitter.off(runId, onNotify);
        signal?.removeEventListener('abort', onAbort);
      };
      const onNotify = () => {
        cleanup();
        resolve();
      };
      const onAbort = () => {
        cleanup();
        reject(signal?.reason ?? new DOMException('Aborted', 'AbortError'));
      };
      const timer = setTimeout(() => {
        cleanup();
        resolve();
      }, timeoutMs);
      timer.unref();
      this.emitter.once(runId, onNotify);
      signal?.addEventListener('abort', onAbort, { once: true });
    });
  }

  close(): void {
    this.emitter.removeAllListeners();
  }
}
