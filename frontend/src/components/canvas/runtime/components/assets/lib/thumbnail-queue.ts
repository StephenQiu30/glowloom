import { previewFiles } from "@/components/canvas/runtime/api/file-preview-bff";
import { unwrap } from "@/components/canvas/runtime/lib/request";

export type ThumbnailGrant = CanvasContract.FilePreviewVO & {
  expiresAt?: number;
};

type Pending = {
  ref: CanvasContract.FilePreviewRequest;
  resolve: (value: ThumbnailGrant) => void;
  reject: (reason: unknown) => void;
  cancelled: boolean;
};
const cancelled = () => new DOMException("请求已取消", "AbortError");

/** One live account/cache scope: batch control requests and separately budget image bytes. */
export class ThumbnailQueue {
  private watchers = new Map<string, number>();
  private clearers = new Map<string, () => void>();
  private pending = new Map<string, Pending>();
  private inFlight = new Map<string, Promise<ThumbnailGrant>>();
  private running = new Set<{
    controller: AbortController;
    entries: Pending[];
  }>();
  private timer?: ReturnType<typeof setTimeout>;
  private bytes = new Set<() => void>();
  private byteWaiters: (() => void)[] = [];
  private reportedErrors = new WeakSet<object>();

  /** A rejected batch shares one error across its SWR keys, within this account scope. */
  notifyOnce(error: unknown, notify: (error: unknown) => void) {
    if (error instanceof Error && error.name === "AbortError") return;
    if (typeof error === "object" && error !== null) {
      if (this.reportedErrors.has(error)) return;
      this.reportedErrors.add(error);
    }
    notify(error);
  }

  private key(ref: CanvasContract.FilePreviewRequest) {
    return JSON.stringify([ref.fileId, ref.version, ref.variant]);
  }

  retain(ref: CanvasContract.FilePreviewRequest, clear: () => void) {
    const key = this.key(ref);
    this.watchers.set(key, (this.watchers.get(key) ?? 0) + 1);
    this.clearers.set(key, clear);
    return () => {
      this.watchers.set(key, (this.watchers.get(key) ?? 1) - 1);
      // StrictMode immediately reattaches; actual departure cancels and clears private grants.
      queueMicrotask(() => {
        if (this.watchers.get(key)) return;
        this.watchers.delete(key);
        const entry = this.pending.get(key);
        if (entry) {
          this.pending.delete(key);
          entry.cancelled = true;
          entry.reject(cancelled());
        }
        for (const batch of this.running) {
          for (const item of batch.entries)
            if (this.key(item.ref) === key) {
              item.cancelled = true;
              item.reject(cancelled());
            }
          if (batch.entries.every((item) => item.cancelled))
            batch.controller.abort();
        }
        this.clearers.get(key)?.();
        this.clearers.delete(key);
        if (!this.watchers.size && this.timer) {
          clearTimeout(this.timer);
          this.timer = undefined;
        }
      });
    };
  }

  request(ref: CanvasContract.FilePreviewRequest): Promise<ThumbnailGrant> {
    const key = this.key(ref);
    const existing = this.inFlight.get(key);
    if (existing) return existing;
    const promise = new Promise<ThumbnailGrant>((resolve, reject) => {
      this.pending.set(key, {
        ref,
        resolve: (value) => {
          if (this.inFlight.get(key) === promise) this.inFlight.delete(key);
          resolve(value);
        },
        reject: (reason) => {
          if (this.inFlight.get(key) === promise) this.inFlight.delete(key);
          reject(reason);
        },
        cancelled: false,
      });
      // IntersectionObserver and SWR publish the visible cards in one frame.
      this.timer ??= setTimeout(() => {
        this.timer = undefined;
        void this.flush();
      }, 32);
    });
    this.inFlight.set(key, promise);
    return promise;
  }

  private async flush() {
    const entries = [...this.pending.values()].slice(0, 48);
    if (!entries.length) return;
    for (const entry of entries) this.pending.delete(this.key(entry.ref));
    if (this.pending.size)
      this.timer = setTimeout(() => {
        this.timer = undefined;
        void this.flush();
      }, 32);
    const batch = { controller: new AbortController(), entries };
    this.running.add(batch);
    // Conservatively include network and batch processing time in the short grant's lifetime.
    const startedAt = Date.now();
    try {
      const results = unwrap(
        await previewFiles(
          entries.map((entry) => entry.ref),
          { signal: batch.controller.signal },
        ),
      );
      if (results.length !== entries.length)
        throw new Error("小图批量授权返回不完整");
      entries.forEach((entry, index) => {
        if (entry.cancelled) return;
        const result = results[index];
        if (
          result.status === "READY" &&
          !result.url?.startsWith("/api/file/content?ticket=")
        )
          entry.reject(new Error("预览授权无效"));
        else
          entry.resolve({
            ...result,
            expiresAt:
              typeof result.expiresIn === "number" &&
              Number.isFinite(result.expiresIn)
                ? startedAt + result.expiresIn * 1000
                : undefined,
          });
      });
    } catch (error) {
      for (const entry of entries) if (!entry.cancelled) entry.reject(error);
    } finally {
      this.running.delete(batch);
    }
  }

  /** Permit is held from assigning img.src until load/error/removal, not just authorization. */
  reserve(signal: AbortSignal): Promise<() => void> {
    return new Promise((resolve, reject) => {
      let released = false;
      let granted = false;
      const release = () => {
        if (released) return;
        released = true;
        signal.removeEventListener("abort", abort);
        this.byteWaiters = this.byteWaiters.filter((next) => next !== start);
        this.bytes.delete(release);
        this.drain();
      };
      const abort = () => {
        release();
        if (!granted) reject(cancelled());
      };
      const start = () => {
        if (released) return;
        granted = true;
        this.bytes.add(release);
        resolve(release);
      };
      if (signal.aborted) return abort();
      signal.addEventListener("abort", abort, { once: true });
      this.byteWaiters.push(start);
      this.drain();
    });
  }

  private drain() {
    while (this.bytes.size < 4 && this.byteWaiters.length)
      this.byteWaiters.shift()?.();
  }
}

const queues = new WeakMap<object, Map<string, ThumbnailQueue>>();
export function thumbnailQueue(cache: object, scope: string) {
  let scoped = queues.get(cache);
  if (!scoped) queues.set(cache, (scoped = new Map()));
  let queue = scoped.get(scope);
  if (!queue) scoped.set(scope, (queue = new ThumbnailQueue()));
  return queue;
}
