"use client";
import {
  createContext,
  useContext,
  useEffect,
  useState,
  type ReactNode,
} from "react";
import { useRouter } from "next/navigation";
import { getSession, deleteSession } from "./generated/auth";
import { AuthError, requireSession } from "./request";
import { Button } from "@/components/ui/button";
const SessionContext = createContext<AuthAPI.SessionView | null>(null);
export function useSession() {
  const session = useContext(SessionContext);
  if (!session) throw new Error("会话尚未就绪");
  return session;
}
export function SessionGate({
  children,
  admin = false,
}: {
  children: ReactNode;
  admin?: boolean;
}) {
  const router = useRouter();
  const [session, setSession] = useState<AuthAPI.SessionView | null>(null);
  const [error, setError] = useState("");
  const [retry, setRetry] = useState(0);
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
        if (e instanceof AuthError && e.status === 401)
          router.replace("/login");
        else
          setError(e instanceof Error ? e.message : "暂时无法验证登录状态。");
      });
    return () => controller.abort();
  }, [router, retry]);
  if (error || !session)
    return (
      <main className="session-state">
        <div>
          <h1>浮光</h1>
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
  if (admin && session.actor.role !== "admin")
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
      {children}
    </SessionContext.Provider>
  );
}
export async function revokeSession(session: AuthAPI.SessionView) {
  await deleteSession({ session_id: session.session_id });
}
