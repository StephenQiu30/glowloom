"use client";
import { useEffect, useRef, useState } from "react";
import Link from "next/link";
import { usePathname } from "next/navigation";
import { listAgentSessions } from "@/api/agent";
import { useOptionalSession } from "@/components/auth/session";
import { Button } from "@/components/ui/button";
import {
  SidebarMenu,
  SidebarMenuItem,
  SidebarMenuButton,
} from "@/components/ui/sidebar";
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
      <p className="sidebar-history-empty px-2 py-3 text-xs text-muted-foreground">
        {session ? "选择项目后查看最近对话" : "登录后查看最近对话"}
      </p>
    );
  return (
    <>
      <SidebarMenu>
        {current.items.map((item) => (
          <SidebarMenuItem key={item.id}>
            <SidebarMenuButton
              asChild
              isActive={path === `/conversations/${item.id}`}
              tooltip={item.title}
            >
              <Link
                href={`/conversations/${item.id}?projectId=${pid}`}
                onClick={onNavigate}
                aria-current={
                  path === `/conversations/${item.id}` ? "page" : undefined
                }
              >
                <span className="truncate">{item.title}</span>
              </Link>
            </SidebarMenuButton>
          </SidebarMenuItem>
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
        <p className="px-2 py-3 text-xs text-muted-foreground">暂无历史对话</p>
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
    <Link
      href={
        memory?.project
          ? `/conversations?projectId=${memory.project.id}`
          : "/conversations"
      }
      aria-label="新建对话"
      className={className}
      onClick={onNavigate}
    >
      {children}
    </Link>
  );
}
