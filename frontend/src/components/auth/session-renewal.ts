import { getSession, refreshSession, upgradeSession } from "@/api/identity";
import {
  getCSRFToken,
  trackSessionRenewal,
  notifySessionInvalidation,
  getScopeGeneration,
  sessionCancellation,
  getSessionGeneration,
  RequestError,
  setCSRFToken,
  waitForSession,
} from "@/lib/request";
import { isOlderSession, requireSession } from "./contracts";

export const sessionIdentitySource = crypto.randomUUID();
let current: API.EmailSessionView | null = null;
let activate: ((session: API.EmailSessionView) => void) | null = null;
let flight: Promise<API.EmailSessionView> | null = null;
let controller = new AbortController();
let lifecycle = 0;

export function configureSessionRenewal(
  session: API.EmailSessionView | null,
  onRenew: (session: API.EmailSessionView) => void,
) {
  if (
    current?.actor.id !== session?.actor.id ||
    current?.session_id !== session?.session_id
  ) {
    controller.abort();
    controller = new AbortController();
    ++lifecycle;
    flight = null;
    trackSessionRenewal(null);
  }
  if (session && isOlderSession(session, current)) return;
  current = session;
  activate = onRenew;
}

/** Read only the protocol's public CSRF cookie; never inspect access/refresh. */
function readableProof() {
  if (typeof document === "undefined") return null;
  const name =
    location.protocol === "https:" ? "__Host-glowloom_csrf" : "glowloom_csrf";
  const entries = document.cookie
    .split(";")
    .map((v) => v.trim())
    .filter((v) => v.startsWith(`${name}=`));
  if (entries.length !== 1) return null;
  try {
    return decodeURIComponent(entries[0].slice(name.length + 1)) || null;
  } catch {
    return null;
  }
}

export function ensureSessionFresh(
  signal?: AbortSignal,
  foreground = false,
): Promise<API.EmailSessionView> {
  if (signal?.aborted) return Promise.reject(sessionCancellation());
  if (!flight) {
    if (!getCSRFToken()) {
      const proof = readableProof();
      if (!proof)
        return Promise.reject(new RequestError("session_invalid", 401));
      setCSRFToken(proof);
    }
    const expected = current;
    const epoch = lifecycle;
    const generation = getSessionGeneration();
    const scope = getScopeGeneration();
    const activeController = controller;
    const check = () => {
      if (
        epoch !== lifecycle ||
        activeController.signal.aborted ||
        generation !== getSessionGeneration() ||
        scope !== getScopeGeneration()
      )
        throw sessionCancellation();
    };
    const run = async () => {
      check();
      // Cookie rotation by another tab is visible only after obtaining the lock.
      const cookie = readableProof();
      if (expected?.session_protocol === "v2" && !cookie)
        throw new RequestError("session_invalid", 401);
      if (cookie && cookie !== getCSRFToken()) throw sessionCancellation();
      if (
        expected?.session_protocol !== "v2" &&
        !expected?.upgrade_available &&
        expected
      )
        return expected;
      const upgrading =
        expected?.session_protocol === "v1" &&
        expected.upgrade_available === true;
      const options = {
        signal: activeController.signal,
        sessionRecovery: "never" as const,
      };
      const readAppliedUpgrade = async () => {
        check();
        if (!foreground) throw new RequestError("request_unconfirmed");
        if (readableProof() !== expected?.csrf_token)
          throw sessionCancellation();
        return getSession(options);
      };
      let value: unknown;
      if (upgrading && cookie) {
        // A previous response may have applied Cookie headers but lost its body.
        value = await readAppliedUpgrade();
      } else {
        try {
          for (let attempt = 0; ; attempt++) {
            check();
            if (!upgrading) {
              const proof = readableProof();
              if (!proof) throw new RequestError("session_invalid", 401);
              if (proof !== getCSRFToken()) throw sessionCancellation();
            }
            try {
              value = await (upgrading ? upgradeSession : refreshSession)(
                options,
              );
              break;
            } catch (error) {
              if (
                upgrading ||
                attempt >= 2 ||
                !(error instanceof RequestError) ||
                error.status !== 409 ||
                !["refresh_busy", "refresh_raced"].includes(error.code)
              )
                throw error;
              await new Promise<void>((resolve, reject) => {
                const abort = () => {
                  clearTimeout(timer);
                  reject(sessionCancellation());
                };
                const timer = setTimeout(
                  () => {
                    activeController.signal.removeEventListener("abort", abort);
                    resolve();
                  },
                  attempt === 0 ? 100 : 5000,
                );
                activeController.signal.addEventListener("abort", abort, {
                  once: true,
                });
                if (activeController.signal.aborted) abort();
              });
            }
          }
        } catch (error) {
          check();
          if (!upgrading || !readableProof()) throw error;
          value = await readAppliedUpgrade();
        }
      }
      const next = requireSession(value);
      if (next.session_protocol !== "v2")
        throw new RequestError("invalid_receipt");
      check();
      if (
        expected &&
        (next.actor.id !== expected.actor.id ||
          next.session_id !== expected.session_id ||
          next.csrf_token !== expected.csrf_token)
      )
        throw new RequestError("invalid_receipt");
      if (isOlderSession(next, current)) throw sessionCancellation();
      current = next;
      setCSRFToken(next.csrf_token);
      activate?.(next);
      return next;
    };
    const pending: Promise<API.EmailSessionView> = Promise.resolve(
      typeof navigator !== "undefined" && navigator.locks
        ? navigator.locks.request(
            "glowloom-session-renewal",
            { signal: activeController.signal },
            run,
          )
        : run(),
    ).catch((error: unknown) => {
      check();
      if (
        error instanceof RequestError &&
        error.status === 401 &&
        error.code !== "access_expired"
      )
        notifySessionInvalidation(generation);
      throw error;
    });
    flight = pending;
    trackSessionRenewal(pending);
    void pending
      .finally(() => {
        if (flight === pending) flight = null;
      })
      .catch(() => {});
  }
  return waitForSession(flight!, signal);
}

/** Reject stale logout callbacks before they touch current private state. */
export function isCurrentSession(session: API.EmailSessionView) {
  return (
    current?.actor.id === session.actor.id &&
    current.session_id === session.session_id &&
    getCSRFToken() === session.csrf_token
  );
}

export function disposeSessionRenewal() {
  ++lifecycle;
  controller.abort();
  controller = new AbortController();
  flight = null;
  trackSessionRenewal(null);
  current = null;
  activate = null;
}

export function announceSessionIdentityChange() {
  if (typeof BroadcastChannel === "undefined") return;
  const channel = new BroadcastChannel("glowloom-session-identity");
  channel.postMessage({
    type: "identity-changed",
    source: sessionIdentitySource,
  });
  channel.close();
}
