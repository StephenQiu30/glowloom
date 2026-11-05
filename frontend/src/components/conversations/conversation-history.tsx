"use client";
import { useEffect, useRef, useState } from "react";
import Link from "next/link";
import { usePathname } from "next/navigation";
import { listAgentSessions } from "@/api/agent";
import { useOptionalSession } from "@/components/auth/session";
import { Button } from "@/components/ui/button";
import { SidebarMenu } from "@/components/ui/sidebar";
import { SidebarEmpty } from "@/components/layout/sidebar-empty";
import { SidebarNavLink } from "@/components/layout/sidebar-nav-link";
import { readErrorMessage } from "@/lib/read-error";
import { useConversationContext } from "./conversation-context";
export function ConversationHistory({
  onNavigate,
}: {
  onNavigate: () => void;
}) {
  const memory = useConversationContext(),
    session = useOptionalSession(),
    path = usePathname(),
    [refresh, setRefresh] = useState(0);
  const identity = `${memory?.ownerKey}/${memory?.project?.id ?? "none"}`,
    pid = memory?.project?.id;
  const [view, setView] = useState<{
    identity: string;
    items: API.SessionView[];
    cursor?: string;
    loading: boolean;
    error: string;
  }>({ identity: "", items: [], loading: false, error: "" });
  const generation = useRef<AbortController | null>(null);
  const current =
    view.identity === identity
      ? view
      : { identity, items: [], loading: !!pid, error: "" };
  useEffect(() => {
    const controller = new AbortController();
    generation.current = controller;
    if (!pid || !session) return () => controller.abort();
    setView({ identity, items: [], loading: true, error: "" });
    void listAgentSessions({ pid }, { signal: controller.signal })
      .then((page) => {
        if (!controller.signal.aborted)
          setView({
            identity,
            items: page.items,
            cursor: page.next_cursor,
            loading: false,
            error: "",
          });
      })
      .catch((error) => {
        if (!controller.signal.aborted)
          setView({
            identity,
            items: [],
            loading: false,
            error: readErrorMessage(error),
          });
      });
    return () => controller.abort();
  }, [identity, pid, session, memory?.listRevision, refresh]);
  async function more() {
    if (!pid || !current.cursor || current.loading) return;
    const controller = generation.current;
    if (!controller || controller.signal.aborted) return;
    setView((v) => ({ ...v, loading: true }));
    try {
      const page = await listAgentSessions(
        { pid, cursor: current.cursor },
        { signal: controller.signal },
      );
      if (!controller.signal.aborted)
        setView((v) => ({
          ...v,
          items: [
            ...new Map(
              [...v.items, ...page.items].map((s) => [s.id, s]),
            ).values(),
          ],
          cursor: page.next_cursor,
          loading: false,
          error: "",
        }));
    } catch (error) {
      if (!controller.signal.aborted)
        setView((v) => ({
          ...v,
          loading: false,
          error: readErrorMessage(error),
        }));
    }
  }
  if (!session || !pid)
    return (
      <SidebarEmpty>
        {session ? "选择项目后查看最近对话" : "登录后查看最近对话"}
      </SidebarEmpty>
    );
  return (
    <>
      <SidebarMenu>
        {current.items.map((item) => (
          <SidebarNavLink
            key={item.id}
            href={`/conversations/${item.id}?projectId=${pid}`}
            label={item.title}
            active={path === `/conversations/${item.id}`}
            variant="history"
            size="history"
            onNavigate={onNavigate}
          />
        ))}
      </SidebarMenu>
      {current.loading ? (
        <p role="status" className="px-2 py-3 text-xs text-muted-foreground">
          正在读取对话…
        </p>
      ) : current.error ? (
        <div className="px-2 py-2 text-xs">
          <p>{current.error}</p>
          <Button
            size="sm"
            variant="ghost"
            onClick={() => setRefresh((n) => n + 1)}
          >
            重试读取
          </Button>
        </div>
      ) : !current.items.length ? (
        <SidebarEmpty>暂无历史对话</SidebarEmpty>
      ) : null}
      {current.cursor ? (
        <Button
          variant="ghost"
          size="sm"
          disabled={current.loading}
          onClick={() => void more()}
        >
          更多对话
        </Button>
      ) : null}
    </>
  );
}
export function NewConversationLink({
  children,
  onNavigate,
  className,
}: {
  children: React.ReactNode;
  onNavigate: () => void;
  className?: string;
}) {
  const memory = useConversationContext();
  return (
    <Button asChild variant="navigation" size="icon-xs" className={className}>
      <Link
        href={
          memory?.project
            ? `/conversations?projectId=${memory.project.id}`
            : "/conversations"
        }
        aria-label="新建对话"
        onClick={onNavigate}
      >
        {children}
      </Link>
    </Button>
  );
}
