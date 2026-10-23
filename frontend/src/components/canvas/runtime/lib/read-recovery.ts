import type { RequestOptions } from "@/lib/request";

export const RECOVERY_TIMEOUT_MS = 30000;
export const RECOVERY_MAX_ATTEMPTS = 4;
const delays = [1000, 2000, 5000];
export type RecoveryRequestConfig = RequestOptions & {
  canvasRecovery?: { deadline: number };
};
type Failure = Error & { code?: number; retryAfterMs?: number };

/** A verified visibility wait, distinct from an unavailable upstream. */
export class SyncPendingError extends Error {
  readonly code = 50301;
  constructor(message = "状态正在同步，请稍后重新读取") {
    super(message);
    this.name = "SyncPendingError";
  }
}

export function retryAfterMilliseconds(value: unknown, now = Date.now()) {
  if (typeof value !== "string" || !value.trim()) return undefined;
  const seconds = Number(value);
  if (Number.isFinite(seconds) && seconds >= 0) return seconds * 1000;
  const date = Date.parse(value);
  return Number.isFinite(date) ? Math.max(0, date - now) : undefined;
}

export function isTransientReadFailure(error: unknown) {
  const failure = error as Failure;
  return (
    [429, 502, 503, 504].includes(Math.floor((failure?.code ?? 0) / 100)) ||
    (!failure?.code &&
      (failure?.name === "AxiosError" || failure instanceof TypeError))
  );
}

/** One logical read owns HTTP time, retries and cancellation, including status + list. */
export async function recoverRead<T>(
  read: (options: RecoveryRequestConfig) => Promise<T>,
  options: {
    signal?: AbortSignal;
    retry?: (error: unknown) => boolean;
    transientErrors?: boolean;
    maxAttempts?: number;
    deadline?: number;
  } = {},
): Promise<T> {
  const controller = new AbortController();
  const signal = options.signal
    ? AbortSignal.any([controller.signal, options.signal])
    : controller.signal;
  const timeout =
    options.deadline === undefined
      ? RECOVERY_TIMEOUT_MS
      : Math.max(
          0,
          Math.min(RECOVERY_TIMEOUT_MS, options.deadline - performance.now()),
        );
  const deadline = performance.now() + timeout;
  const expired = new Error("读取等待超时，请重新读取");
  const timer = setTimeout(() => controller.abort(expired), timeout);
  let last: unknown;
  let rejectAbort: (reason: unknown) => void;
  const aborted = new Promise<never>((_, reject) => {
    rejectAbort = reject;
  });
  const onAbort = () =>
    rejectAbort(
      controller.signal.aborted
        ? (last ?? expired)
        : new DOMException("请求已取消", "AbortError"),
    );
  signal.addEventListener("abort", onAbort, { once: true });
  if (signal.aborted) onAbort();
  try {
    for (let attempt = 0; ; attempt++) {
      if (signal.aborted) return await aborted;
      try {
        return await Promise.race([
          read({ signal, canvasRecovery: { deadline } }),
          aborted,
        ]);
      } catch (error) {
        if (signal.aborted) return await aborted;
        const failure = error as Failure;
        const rateLimited = failure?.code === 42900;
        const retry = options.retry
          ? options.retry(error)
          : rateLimited ||
            failure?.code === 50301 ||
            (options.transientErrors === true && isTransientReadFailure(error));
        if (
          !retry ||
          attempt + 1 >= (options.maxAttempts ?? RECOVERY_MAX_ATTEMPTS)
        )
          throw error;
        last = error;
        const wait = failure.retryAfterMs ?? delays[attempt] ?? 0;
        let waiting: ReturnType<typeof setTimeout> | undefined;
        try {
          await Promise.race([
            new Promise<void>((resolve) => {
              waiting = setTimeout(resolve, Math.max(0, wait));
            }),
            aborted,
          ]);
        } finally {
          clearTimeout(waiting);
        }
      }
    }
  } finally {
    clearTimeout(timer);
    signal.removeEventListener("abort", onAbort);
  }
}
