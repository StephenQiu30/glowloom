"use client";
import { branchJournal } from "@/components/generation/draft-save";
import {
  createContext,
  useContext,
  useCallback,
  useId,
  useEffect,
  useEffectEvent,
  useState,
  useRef,
  type ReactNode,
} from "react";
import { usePathname, useRouter } from "next/navigation";
import { useTheme } from "next-themes";
import { getSession, deleteSession, getMyPreferences } from "@/api/identity";
import {
  RequestError,
  setCSRFToken,
  subscribeSessionInvalidation,
  getSessionGeneration,
  notifySessionInvalidation,
  setSessionRecovery,
  sessionCancellation,
} from "@/lib/request";
import { requireSession, canonicalAuthLink, isOlderSession } from "./contracts";
import {
  sessionIdentitySource,
  isCurrentSession,
  announceSessionIdentityChange,
  configureSessionRenewal,
  disposeSessionRenewal,
  ensureSessionFresh,
} from "./session-renewal";
export { ensureSessionFresh, disposeSessionRenewal } from "./session-renewal";
import { Button } from "@/components/ui/button";
import {
  ApplicationLoading,
  ApplicationState,
} from "@/components/layout/application-state";
import { toast } from "sonner";
import { projectIntentStore } from "@/components/projects/project-intent-store";
import { projectPresentationStore } from "@/components/projects/project-presentation-store";

let privateCleanup = Promise.resolve();
function queuePrivateCleanup(
  options?: Parameters<typeof branchJournal.clear>[0],
) {
  projectPresentationStore.clear();
  const intentCleanup = projectIntentStore
    .invalidate(
      options?.preserveInput || options?.retainWriter
        ? "passive_expiry"
        : "active_logout",
    )
    .catch(() => {});
  privateCleanup = privateCleanup
    .then(async () => {
      await Promise.all([branchJournal.clear(options), intentCleanup]);
    })
    .catch(() => {});
  return privateCleanup;
}
export const SessionContext = createContext<API.EmailSessionView | null>(null);
const SessionUpdateContext = createContext<
  (session: API.EmailSessionView) => void
>(() => {});
export function useOptionalSession() {
  return useContext(SessionContext);
}
export function useSessionUpdate() {
  return useContext(SessionUpdateContext);
}
export function useSession() {
  const session = useContext(SessionContext);
  if (!session) throw new Error("会话尚未就绪");
  return session;
}
const SessionStatusContext = createContext<{
  guest: boolean;
  publicUnavailable: boolean;
  error: string;
  canonicalLink: string | null;
  retry: () => void;
} | null>(null);

/** Root-owned identity; route checks never discard an already confirmed session. */
export function SessionProvider({ children }: { children: ReactNode }) {
  const pathname = usePathname();
  const { setTheme } = useTheme();
  const [session, setSession] = useState<API.EmailSessionView | null>(null);
  const [error, setError] = useState("");
  const [canonicalLink, setCanonicalLink] = useState<string | null>(null);
  const [retry, setRetry] = useState(0);
  const [guest, setGuest] = useState(false);
  const updateVersion = useRef(0);
  const guestCleaned = useRef(false);
  const [publicUnavailable, setPublicUnavailable] = useState(false);
  const staleSession = useEffectEvent((next: API.EmailSessionView) =>
    isOlderSession(next, session),
  );
  const prepareIdentity = useEffectEvent((next: API.EmailSessionView) => {
    if (
      session &&
      (session.actor.id !== next.actor.id ||
        session.session_id !== next.session_id)
    ) {
      disposeSessionRenewal();
      setCSRFToken(null);
      setSession(null);
      void queuePrivateCleanup();
      return true;
    }
    return false;
  });
  useEffect(() => {
    const controller = new AbortController();
    const version = ++updateVersion.current;
    let journalCheckpoint: string | null | undefined;
    branchJournal
      .checkpoint()
      .then((value) => {
        journalCheckpoint = value;
      })
      .catch(() => {})
      .then(() =>
        getSession({ signal: controller.signal, sessionRecovery: "never" }),
      )
      .catch(async (reason: unknown) => {
        if (
          reason instanceof RequestError &&
          reason.status === 401 &&
          ["access_expired", "unauthenticated"].includes(reason.code)
        ) {
          if (
            reason.code === "unauthenticated" &&
            !document.cookie
              .split(";")
              .some((v) => /^(?:__Host-)?glowloom_csrf=/.test(v.trim()))
          )
            throw reason;
          await ensureSessionFresh(controller.signal, true);
          return getSession({
            signal: controller.signal,
            sessionRecovery: "never",
          });
        }
        throw reason;
      })
      .then(requireSession)
      .then(async (s) => {
        if (controller.signal.aborted || updateVersion.current !== version)
          return;
        if (staleSession(s)) return;
        const identityChanged = prepareIdentity(s);
        await privateCleanup;
        if (controller.signal.aborted || updateVersion.current !== version)
          return;
        if (staleSession(s)) return;
        setError("");
        setCanonicalLink(null);
        setPublicUnavailable(false);
        setGuest(false);
        guestCleaned.current = false;
        setCSRFToken(s.csrf_token);
        await branchJournal
          .activateAccount(
            s.actor.id,
            s.session_id,
            getSessionGeneration(),
            identityChanged ? undefined : journalCheckpoint,
          )
          .catch(() => {});
        if (controller.signal.aborted || updateVersion.current !== version)
          return;
        setSession((current) => (isOlderSession(s, current) ? current : s));
      })
      .catch((e) => {
        if (controller.signal.aborted || updateVersion.current !== version)
          return;
        if (e instanceof RequestError && e.status === 401) {
          setSession(null);
          setError("");
          setPublicUnavailable(false);
          if (!guestCleaned.current) {
            guestCleaned.current = true;
            void clearPrivateSession(true);
          }
          setGuest(true);
        } else {
          setCanonicalLink(
            e instanceof RequestError &&
              ["origin_forbidden", "origin_mismatch"].includes(e.code)
              ? canonicalAuthLink(e.meta, window.location.href)
              : null,
          );
          setError("暂时无法确认登录状态");
          setGuest(false);
          setPublicUnavailable(true);
        }
      });
    return () => {
      controller.abort();
    };
  }, [pathname, retry]);
  useEffect(
    () =>
      subscribeSessionInvalidation(() => {
        ++updateVersion.current;
        guestCleaned.current = true;
        setSession(null);
        setPublicUnavailable(false);
        setError("");
        setGuest(true);
        void clearPrivateSession(true);
      }),
    [],
  );
  useEffect(() => {
    configureSessionRenewal(null, () => {});
    setSessionRecovery(async (signal) => {
      await ensureSessionFresh(signal);
    });
    return () => {
      setSessionRecovery(null);
      disposeSessionRenewal();
    };
  }, []);
  useEffect(() => {
    configureSessionRenewal(session, (next) => {
      setSession((current) => (isOlderSession(next, current) ? current : next));
      setError("");
      setPublicUnavailable(false);
      setGuest(false);
    });
  }, [session]);
  useEffect(() => {
    let timer: ReturnType<typeof setTimeout> | undefined;
    let active = true;
    let failures = 0;
    const observedAt = Date.now();
    const schedule = (backoff?: number) => {
      clearTimeout(timer);
      if (
        !active ||
        !session ||
        document.visibilityState === "hidden" ||
        navigator.onLine === false
      )
        return;
      if (session.session_protocol !== "v2" && !session.upgrade_available)
        return;
      const remaining =
        session.session_protocol === "v2"
          ? Math.min(
              Date.parse(session.refresh_after_at!),
              Date.parse(session.idle_expires_at),
            ) -
            Date.parse(session.server_time) -
            (Date.now() - observedAt)
          : 0;
      // Past/coalesced receipts get a bounded cooldown, never a zero-delay loop.
      const delay =
        session.session_protocol === "v2"
          ? remaining > 0
            ? remaining
            : 60000
          : 0;
      timer = setTimeout(() => {
        void ensureSessionFresh()
          .then(() => {
            failures = 0;
            // New receipts reschedule via [session]; identical/coalesced ones cool down.
            schedule(60000);
          })
          .catch((reason: unknown) => {
            if (
              !active ||
              document.visibilityState === "hidden" ||
              navigator.onLine === false
            )
              return;
            if (reason instanceof RequestError && reason.status === 401) return;
            if (reason instanceof RequestError)
              setError("登录续期暂不可用，正在自动重试。");
            failures = Math.min(failures + 1, 5);
            const budget =
              reason instanceof RequestError
                ? reason.meta?.retry_after_seconds
                : undefined;
            const retryAfter =
              typeof budget === "number" &&
              Number.isInteger(budget) &&
              budget >= 0 &&
              budget <= 86400
                ? budget * 1000
                : 0;
            schedule(
              Math.max(Math.min(5000 * 2 ** (failures - 1), 60000), retryAfter),
            );
          });
      }, backoff ?? delay);
    };
    const foreground = () => {
      schedule();
      if (document.visibilityState !== "hidden" && navigator.onLine !== false)
        setRetry((n) => n + 1);
    };
    const visibility = () => {
      if (document.visibilityState === "hidden") clearTimeout(timer);
      else foreground();
    };
    const offline = () => clearTimeout(timer);
    schedule();
    document.addEventListener("visibilitychange", visibility);
    window.addEventListener("pageshow", foreground);
    window.addEventListener("online", foreground);
    window.addEventListener("offline", offline);
    window.addEventListener("focus", foreground);
    const channel =
      typeof BroadcastChannel !== "undefined"
        ? new BroadcastChannel("glowloom-session-identity")
        : null;
    if (channel)
      channel.onmessage = (event) => {
        if (
          event.data?.type === "identity-changed" &&
          event.data.source !== sessionIdentitySource
        ) {
          branchJournal.invalidateObservedIdentity();
          void projectIntentStore.invalidate("passive_expiry");
          foreground();
        }
      };
    return () => {
      active = false;
      clearTimeout(timer);
      document.removeEventListener("visibilitychange", visibility);
      window.removeEventListener("pageshow", foreground);
      window.removeEventListener("online", foreground);
      window.removeEventListener("offline", offline);
      window.removeEventListener("focus", foreground);
      channel?.close();
    };
  }, [session]);
  const accountID = session?.actor.id;
  const restoreTheme = useEffectEvent((value: string) => setTheme(value));
  useEffect(() => {
    if (!accountID) return;
    const controller = new AbortController();
    getMyPreferences({ signal: controller.signal })
      .then((value) => {
        if (
          !controller.signal.aborted &&
          ["light", "dark", "system"].includes(value.theme)
        )
          restoreTheme(value.theme);
      })
      .catch(() => {});
    return () => controller.abort();
  }, [accountID]);
  const retrySession = useCallback(() => {
    setError("");
    setRetry((n) => n + 1);
  }, []);
  return (
    <SessionContext.Provider value={session}>
      <SessionUpdateContext.Provider
        value={(next) => {
          if (isOlderSession(next, session)) return;
          const version = ++updateVersion.current;
          setError("");
          setPublicUnavailable(false);
          const identityChanged =
            session?.actor.id !== next.actor.id ||
            session?.session_id !== next.session_id;
          if (identityChanged) {
            disposeSessionRenewal();
            setCSRFToken(null);
            setSession(null);
            if (session) void queuePrivateCleanup();
          }
          void privateCleanup.then(async () => {
            if (updateVersion.current !== version) return;
            guestCleaned.current = false;
            if (identityChanged) announceSessionIdentityChange();
            setCSRFToken(next.csrf_token);
            await branchJournal
              .activateAccount(
                next.actor.id,
                next.session_id,
                getSessionGeneration(),
              )
              .catch(() => {});
            if (updateVersion.current !== version) return;
            setGuest(false);
            setSession((current) =>
              isOlderSession(next, current) ? current : next,
            );
          });
        }}
      >
        <SessionStatusContext.Provider
          value={{
            guest,
            publicUnavailable,
            error,
            canonicalLink,
            retry: retrySession,
          }}
        >
          <div key={session?.actor.id ?? "guest"} className="contents">
            {children}
          </div>
        </SessionStatusContext.Provider>
      </SessionUpdateContext.Provider>
    </SessionContext.Provider>
  );
}
/** Route policy stays local; identity and theme survive a page remount. */
export function SessionGate({
  children,
  admin = false,
  allowGuest = false,
}: {
  children: ReactNode;
  admin?: boolean;
  allowGuest?: boolean;
}) {
  const session = useOptionalSession();
  const status = useContext(SessionStatusContext);
  const router = useRouter();
  const noticeID = useId();
  if (!status) throw new Error("SessionGate requires SessionProvider");
  const { guest, publicUnavailable, error, canonicalLink, retry } = status;
  useEffect(() => {
    if (guest && !allowGuest) router.replace("/login");
  }, [guest, allowGuest, router]);
  useEffect(() => {
    if (error && (allowGuest || session))
      toast(error, {
        id: noticeID,
        action: { label: "重试", onClick: retry },
      });
    return () => {
      toast.dismiss(noticeID);
    };
  }, [error, allowGuest, session, retry, noticeID]);
  if (!allowGuest && error && !session)
    return (
      <ApplicationState title={error} description="请稍后重试。">
        {canonicalLink ? (
          <a href={canonicalLink}>使用可信网站地址打开</a>
        ) : (
          <Button onClick={retry}>重试</Button>
        )}
      </ApplicationState>
    );
  if (!session && (!allowGuest || (!guest && !publicUnavailable)))
    return <ApplicationLoading message="正在验证登录状态…" />;
  if (admin && session?.actor.platform_role !== "platform_admin")
    return (
      <ApplicationState title="无权访问" description="此页面仅向管理员开放。">
        <Button onClick={() => router.replace("/")}>返回创作</Button>
      </ApplicationState>
    );
  return (
    <>
      {error && canonicalLink ? (
        <a href={canonicalLink}>使用可信网站地址打开</a>
      ) : null}
      {children}
    </>
  );
}
export function clearPrivateResources() {
  return queuePrivateCleanup({ retainWriter: true });
}
export async function clearPrivateSession(preserveInput = false) {
  disposeSessionRenewal();
  setCSRFToken(null);
  await queuePrivateCleanup(
    preserveInput ? { preserveInput: true } : undefined,
  );
}
export async function revokeSession(session: API.EmailSessionView) {
  if (!isCurrentSession(session)) throw sessionCancellation();
  const generation = getSessionGeneration();
  // Logout intent destroys local recovery even if the server reports it already expired.
  // This queue does not block the DELETE or remove its Origin/CSRF proof.
  void queuePrivateCleanup({ logoutSession: session.session_id });
  await deleteSession({ id: session.session_id }, { sessionRecovery: "never" });
  if (generation !== getSessionGeneration()) return;
  announceSessionIdentityChange();
  notifySessionInvalidation(generation);
  // The root subscriber clears proof immediately and queues private draft cleanup.
  if (generation === getSessionGeneration()) await clearPrivateSession();
  else await privateCleanup;
}
