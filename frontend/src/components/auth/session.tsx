"use client";
import { branchJournal } from "@/components/generation/draft-save";
import {
  createContext,
  useContext,
  useEffect,
  useState,
  type ReactNode,
} from "react";
import { useRouter } from "next/navigation";
import { getSession, deleteSession } from "@/api/identity";
import { RequestError } from "@/lib/request";
import { requireSession, authErrorMessage } from "./contracts";
import { Button } from "@/components/ui/button";
const SessionContext = createContext<API.SessionView | null>(null);
const SessionUpdateContext = createContext<(session: API.SessionView) => void>(
  () => {},
);
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
  const [session, setSession] = useState<API.SessionView | null>(null);
  const [error, setError] = useState("");
  const [retry, setRetry] = useState(0);
  const [guest, setGuest] = useState(false);
  useEffect(() => {
    const controller = new AbortController();
    getSession({ signal: controller.signal })
      .then(requireSession)
      .then((s) => {
        if (controller.signal.aborted) return;
        if (s.actor.must_change_password) router.replace("/reset-password");
        else setSession(s);
      })
      .catch((e) => {
        if (controller.signal.aborted) return;
        if (allowGuest) {
          setGuest(true);
        } else if (e instanceof RequestError && e.status === 401) {
          router.replace("/login");
        } else setError(authErrorMessage(e));
      });
    return () => controller.abort();
  }, [router, retry, allowGuest]);
  if (error || (!session && !guest))
    return (
      <main className="session-state">
        <div>
          <h1>织光</h1>
          <p role={error ? "alert" : "status"}>
            {error || "正在验证登录状态…"}
          </p>
          {error && (
            <Button
              onClick={() => {
                setError("");
                setRetry((n) => n + 1);
              }}
            >
              重新检查会话
            </Button>
          )}
        </div>
      </main>
    );
  if (
    admin &&
    session?.actor.role !== "admin" &&
    session?.actor.platform_role !== "platform_admin"
  )
    return (
      <main className="session-state">
        <div>
          <h1>无权访问</h1>
          <p>此页面仅向管理员开放。</p>
          <Button onClick={() => router.replace("/")}>返回创作</Button>
        </div>
      </main>
    );
  return (
    <SessionContext.Provider value={session}>
      <SessionUpdateContext.Provider value={setSession}>
        {children}
      </SessionUpdateContext.Provider>
    </SessionContext.Provider>
  );
}
export async function revokeSession(session: API.SessionView) {
  await deleteSession({ session_id: session.session_id });
  await branchJournal.clear().catch(() => {});
}
