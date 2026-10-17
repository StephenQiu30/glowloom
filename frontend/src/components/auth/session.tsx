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
} from "@/lib/request";
import { requireSession } from "./contracts";
import { Button } from "@/components/ui/button";
import {
  ApplicationLoading,
  ApplicationState,
} from "@/components/layout/application-state";
import { toast } from "sonner";
let privateCleanup = Promise.resolve();
function queuePrivateCleanup() {
  privateCleanup = privateCleanup
    .then(() => branchJournal.clear())
    .catch(() => {});
  return privateCleanup;
}
const SessionContext = createContext<API.EmailSessionView | null>(null);
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
  retry: () => void;
} | null>(null);

/** Root-owned identity; route checks never discard an already confirmed session. */
export function SessionProvider({ children }: { children: ReactNode }) {
  const pathname = usePathname();
  const { setTheme } = useTheme();
  const [session, setSession] = useState<API.EmailSessionView | null>(null);
  const [error, setError] = useState("");
  const [retry, setRetry] = useState(0);
  const [guest, setGuest] = useState(false);
  const updateVersion = useRef(0);
  const [publicUnavailable, setPublicUnavailable] = useState(false);
  useEffect(() => {
    const controller = new AbortController();
    const version = ++updateVersion.current;
    getSession({ signal: controller.signal })
      .then(requireSession)
      .then(async (s) => {
        await privateCleanup;
        if (controller.signal.aborted || updateVersion.current !== version)
          return;
        setError("");
        setPublicUnavailable(false);
        setGuest(false);
        setCSRFToken(s.csrf_token);
        setSession(s);
      })
      .catch((e) => {
        if (controller.signal.aborted || updateVersion.current !== version)
          return;
        if (e instanceof RequestError && e.status === 401) {
          setSession(null);
          setError("");
          setPublicUnavailable(false);
          void clearPrivateSession();
          setGuest(true);
        } else {
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
        setSession(null);
        setPublicUnavailable(false);
        setError("");
        setGuest(true);
        void clearPrivateSession();
      }),
    [],
  );
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
          const version = ++updateVersion.current;
          setError("");
          setPublicUnavailable(false);
          if (session?.actor.id !== next.actor.id) {
            setCSRFToken(null);
            setSession(null);
            if (session) void queuePrivateCleanup();
          }
          void privateCleanup.then(() => {
            if (updateVersion.current !== version) return;
            setCSRFToken(next.csrf_token);
            setGuest(false);
            setSession(next);
          });
        }}
      >
        <SessionStatusContext.Provider
          value={{ guest, publicUnavailable, error, retry: retrySession }}
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
  const { guest, publicUnavailable, error, retry } = status;
  useEffect(() => {
    if (guest && !allowGuest) router.replace("/login");
  }, [guest, allowGuest, router]);
  useEffect(() => {
    if (error && allowGuest)
      toast("暂时无法确认登录状态", {
        id: noticeID,
        action: { label: "重试", onClick: retry },
      });
    return () => {
      toast.dismiss(noticeID);
    };
  }, [error, allowGuest, retry, noticeID]);
  if (!allowGuest && error)
    return (
      <ApplicationState title={error} description="请稍后重试。">
        <Button onClick={retry}>重试</Button>
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
  return children;
}
export function clearPrivateResources() {
  return queuePrivateCleanup();
}
export async function clearPrivateSession() {
  setCSRFToken(null);
  await queuePrivateCleanup();
}
export async function revokeSession(session: API.EmailSessionView) {
  await deleteSession({ id: session.session_id });
  setCSRFToken(null);
  await queuePrivateCleanup();
}
