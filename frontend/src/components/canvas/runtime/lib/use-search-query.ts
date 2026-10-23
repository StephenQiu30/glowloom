"use client";
import { useCallback, useEffect, useMemo, useRef } from "react";
import useSWR, {
  unstable_serialize,
  useSWRConfig,
  type Key,
  type SWRConfiguration,
} from "swr";
import {
  onlineProjections,
  revision,
  type OnlineProjection,
  type ProjectionSignal,
} from "./online-projections";
import {
  recoverRead,
  isTransientReadFailure,
  SyncPendingError,
  RECOVERY_TIMEOUT_MS,
  RECOVERY_MAX_ATTEMPTS,
  type RecoveryRequestConfig,
} from "./read-recovery";

type Observer = {
  event: (event: ProjectionSignal) => void;
  resume: () => void;
};
class ReadEntry {
  private observers = new Set<Observer>();
  private disconnect?: () => void;
  private pending?: Promise<unknown>;
  private controller?: AbortController;
  private timer?: ReturnType<typeof setTimeout>;
  private failure?: unknown;
  private attempts = 0;
  private deadline?: number;
  private reported?: unknown;
  private facts = new Map<string, bigint>();
  epoch = -1;
  read<T>(
    load: (options: RecoveryRequestConfig) => Promise<T>,
    mediaRecovery = false,
  ) {
    if (this.pending) return this.pending as Promise<T>;
    if (this.failure) return Promise.reject(this.failure);
    const controller = new AbortController();
    this.controller = controller;
    const remaining = Math.max(1, RECOVERY_MAX_ATTEMPTS - this.attempts);
    this.deadline ??= performance.now() + RECOVERY_TIMEOUT_MS;
    this.pending = recoverRead(
      (options) => {
        this.attempts++;
        return load(options);
      },
      {
        signal: controller.signal,
        transientErrors: true,
        maxAttempts: remaining,
        deadline: this.deadline,
        // The media resolver already budgets grants and byte retries as one operation.
        ...(mediaRecovery ? { retry: () => false } : {}),
      },
    )
      .then((value) => {
        this.attempts = 0;
        this.deadline = undefined;
        return value;
      })
      .catch((error: unknown) => {
        if (error instanceof Error && error.name === "AbortError") {
          this.attempts = 0;
          this.deadline = undefined;
        } else {
          this.failure = error;
          if (mediaRecovery) this.attempts = RECOVERY_MAX_ATTEMPTS;
        }
        throw error;
      })
      .finally(() => {
        this.pending = undefined;
        this.controller = undefined;
      });
    return this.pending as Promise<T>;
  }
  retry() {
    if (!this.pending) {
      this.failure = undefined;
      this.reported = undefined;
      this.attempts = 0;
      this.deadline = undefined;
    }
  }
  report(error: unknown) {
    if (this.reported === error) return false;
    this.reported = error;
    return true;
  }
  fact(event: OnlineProjection) {
    const key = `${event.resourceType}:${event.resourceId}:${event.payload.dataEpoch ?? ""}`;
    const previous = this.facts.get(key);
    const next = revision(event.resourceVersion);
    if (previous !== undefined && next <= previous) return false;
    this.facts.set(key, next);
    this.retry();
    return true;
  }
  schedule(run: () => void) {
    // In-flight invalidations are retained by ProjectionStream and checked by every snapshot.
    if (this.pending || this.timer) return;
    if (this.failure) {
      if (
        this.attempts >= RECOVERY_MAX_ATTEMPTS ||
        performance.now() >= (this.deadline ?? Infinity) ||
        !isTransientReadFailure(this.failure)
      )
        return;
      this.failure = undefined;
    }
    this.timer = setTimeout(() => {
      this.timer = undefined;
      run();
    }, 100);
  }
  attach(observer: Observer, onReleased?: () => void) {
    this.observers.add(observer);
    if (!this.disconnect) {
      const unsubscribe = onlineProjections.subscribe((event) =>
        this.observers.values().next().value?.event(event),
      );
      const resume = () => this.observers.values().next().value?.resume();
      window.addEventListener("focus", resume);
      window.addEventListener("online", resume);
      this.disconnect = () => {
        unsubscribe();
        window.removeEventListener("focus", resume);
        window.removeEventListener("online", resume);
      };
    }
    return () => {
      this.observers.delete(observer);
      if (this.observers.size) return;
      clearTimeout(this.timer);
      this.timer = undefined;
      this.disconnect?.();
      this.disconnect = undefined;
      queueMicrotask(() => {
        if (!this.observers.size) {
          this.controller?.abort();
          // SWR may publish a cancelled read after the grant owner clears its cache.
          void Promise.resolve(this.pending)
            .catch(() => undefined)
            .then(() => {
              setTimeout(() => {
                if (!this.observers.size) onReleased?.();
              }, 0);
            });
        }
      });
      // Exhaustion and seen versions survive remounts within the same private cache.
    };
  }
}
const stores = new WeakMap<object, Map<string, ReadEntry>>();
export function isSearchQuery(key: readonly unknown[]) {
  if (
    [
      "my-assets",
      "public-assets",
      "personal-projects",
      "project-canvases",
    ].includes(String(key[0]))
  )
    return String(key[4] ?? "").trim().length > 0;
  if (key[0] === "workspace-search")
    return String(key[2] ?? "").trim().length > 0;
  return key[0] === "canvas-search-status";
}
function entryFor(cache: object, key: string) {
  let store = stores.get(cache);
  if (!store) {
    store = new Map();
    stores.set(cache, store);
  }
  let entry = store.get(key);
  if (!entry) {
    entry = new ReadEntry();
    store.set(key, entry);
  }
  return entry;
}
type ProjectionPolicy = {
  strict: boolean;
  live: boolean;
  patch: (
    data: unknown,
    key: readonly unknown[],
    event: OnlineProjection,
  ) => { data: unknown; read: boolean };
  merge: (
    incoming: unknown,
    previous: unknown,
    key: readonly unknown[],
  ) => unknown;
};

/** One private-key scheduler; search validation and domain merging remain independent. */
export function useSearchQuery<Data, QueryKey extends Key>(
  key: QueryKey | null,
  fetcher:
    ((key: QueryKey, options: RecoveryRequestConfig) => Promise<Data>) | null,
  config: SWRConfiguration<Data>,
  matches: (key: readonly unknown[], event: OnlineProjection) => boolean,
  policy: ProjectionPolicy,
) {
  const global = useSWRConfig();
  const serialized = unstable_serialize(key);
  const entry = useMemo(
    () => entryFor(global.cache, serialized),
    [global.cache, serialized],
  );
  const latest = useRef({ key, matches, policy, config });
  useEffect(() => {
    latest.current = { key, matches, policy, config };
  });
  const load = () =>
    entry.read(
      async (options) => {
        if (!fetcher || !serialized || key === null)
          throw new DOMException("读取已取消", "AbortError");
        const epoch = onlineProjections.epoch(policy.strict);
        const position = onlineProjections.version();
        let result: Data = await fetcher(key, options);
        const array = Array.isArray(key) ? key : [];
        if (policy.live) {
          if (epoch !== onlineProjections.epoch(policy.strict))
            throw new SyncPendingError();
          const late = new Set(
            onlineProjections.latest(position).map((event) => event.eventId),
          );
          for (const event of onlineProjections.latest()) {
            if (!matches(array, event)) continue;
            entry.fact(event);
            const patched = policy.patch(result, array, event);
            if (late.has(event.eventId) && patched.read)
              throw new SyncPendingError();
            result = patched.data as Data;
          }
          if (!policy.strict && entry.epoch === epoch)
            result = policy.merge(
              result,
              global.cache.get(serialized)?.data,
              array,
            ) as Data;
          entry.epoch = epoch;
        }
        return result;
      },
      ["asset-preview", "generation-file"].includes(
        String((Array.isArray(key) ? key : [])[0]),
      ),
    );
  const query = useSWR<Data>(key, fetcher ? load : null, {
    ...config,
    refreshInterval: 0,
    revalidateOnFocus: false,
    revalidateOnReconnect: false,
    shouldRetryOnError: false,
    onError: (error, readKey, settings) => {
      const report =
        config.onError ?? (global.onError as SWRConfiguration<Data>["onError"]);
      if (
        error?.name !== "AbortError" &&
        !(error instanceof SyncPendingError) &&
        entry.report(error)
      )
        report?.(error, readKey, settings);
    },
  });
  const { mutate } = query;
  const enabled = !!fetcher;
  useEffect(() => {
    if (!serialized || !enabled) return;
    const read = () => {
      void mutate().catch(() => undefined);
    };
    const schedule = () => entry.schedule(read);
    const preserveMedia = () =>
      ["asset-preview", "generation-file"].includes(
        String(
          (Array.isArray(latest.current.key) ? latest.current.key : [])[0],
        ),
      ) &&
      (global.cache.get(serialized)?.data as { status?: string } | undefined)
        ?.status === "READY";
    const media = ["asset-preview", "generation-file"].includes(
      String((Array.isArray(latest.current.key) ? latest.current.key : [])[0]),
    );
    return entry.attach(
      {
        resume: () => {
          if (!preserveMedia()) schedule();
        },
        event: (event) => {
          const current = latest.current;
          if (!current.policy.live || preserveMedia()) return;
          if (event.type === "recover") {
            if (event.scope !== "search" || current.policy.strict) schedule();
            return;
          }
          const array = Array.isArray(current.key) ? current.key : [];
          if (!current.matches(array, event) || !entry.fact(event)) return;
          if (entry.epoch !== onlineProjections.epoch(current.policy.strict)) {
            schedule();
            return;
          }
          const previous = global.cache.get(serialized)?.data;
          const next = current.policy.patch(previous, array, event);
          if (next.read) {
            schedule();
            return;
          }
          if (next.data !== previous) {
            void mutate(next.data as Data, { revalidate: false });
            current.config.onSuccess?.(
              next.data as Data,
              serialized,
              current.config as Parameters<
                NonNullable<SWRConfiguration<Data>["onSuccess"]>
              >[2],
            );
          }
        },
      },
      media ? () => global.cache.delete(serialized) : undefined,
    );
  }, [serialized, entry, mutate, enabled, global.cache]);
  const retry: typeof mutate = useCallback(
    (...args) => {
      entry.retry();
      return mutate(...args);
    },
    [entry, mutate],
  );
  const waiting = query.isValidating;
  return {
    ...query,
    data:
      (policy.strict && (query.error || waiting)) ||
      [401, 403, 404].includes(Math.floor(Number(query.error?.code) / 100))
        ? undefined
        : query.data,
    error:
      waiting || query.error?.name === "AbortError" ? undefined : query.error,
    isLoading: query.isLoading || (policy.strict && waiting),
    mutate: retry,
  };
}
