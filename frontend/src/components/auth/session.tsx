"use client";
import { branchJournal } from "@/components/generation/draft-save";
import {
  createContext,
  useContext,
  useId,
  useEffect,
  useEffectEvent,
  useState,
  useRef,
  type ReactNode,
} from "react";
import { useRouter } from "next/navigation";
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
export function SessionGate({
  children,
  admin = false,
  allowGuest = false,
}: {
  children: ReactNode;
  admin?: boolean;
  allowGuest?: boolean;
}) {
  const router = useRouter();
  const noticeID = useId();
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
        toast.dismiss(noticeID);
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
          toast.dismiss(noticeID);
          setSession(null);
          setError("");
          setPublicUnavailable(false);
          void clearPrivateSession();
          if (allowGuest) setGuest(true);
          else router.replace("/login");
        } else {
          setError("暂时无法确认登录状态");
          if (allowGuest) {
            setPublicUnavailable(true);
            toast("暂时无法确认登录状态", {
              id: noticeID,
              action: {
                label: "重试",
                onClick: () => {
                  setError("");
                  setRetry((n) => n + 1);
                },
              },
            });
          }
        }
      });
    return () => {
      controller.abort();
      toast.dismiss(noticeID);
    };
  }, [router, retry, allowGuest, noticeID]);
  useEffect(
    () =>
      subscribeSessionInvalidation(() => {
        toast.dismiss(noticeID);
        ++updateVersion.current;
        setSession(null);
        setPublicUnavailable(false);
        setError("");
        setGuest(allowGuest);
        void clearPrivateSession();
        if (!allowGuest) router.replace("/login");
      }),
    [allowGuest, router, noticeID],
  );
  const accountID = session?.actor.id;
  const restoreTheme = useEffectEvent((value: string) => setTheme(value));
  useEffect(() => {
    if (!accountID) return;
    restoreTheme("system");
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
  if (!allowGuest && error)
    return (
      <ApplicationState title={error} description="请稍后重试。">
        <Button
          onClick={() => {
            setError("");
            setRetry((n) => n + 1);
          }}
        >
          重试
        </Button>
      </ApplicationState>
    );
  if (!session && !guest && !publicUnavailable)
    return <ApplicationLoading message="正在验证登录状态…" />;
  if (admin && session?.actor.platform_role !== "platform_admin")
    return (
      <ApplicationState title="无权访问" description="此页面仅向管理员开放。">
        <Button onClick={() => router.replace("/")}>返回创作</Button>
      </ApplicationState>
    );
  return (
    <SessionContext.Provider value={session}>
      <SessionUpdateContext.Provider
        value={(next) => {
          const version = ++updateVersion.current;
          toast.dismiss(noticeID);
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
        <div key={session?.actor.id ?? "guest"} className="contents">
          {children}
        </div>
      </SessionUpdateContext.Provider>
    </SessionContext.Provider>
  );
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
