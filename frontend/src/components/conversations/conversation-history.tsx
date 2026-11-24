"use client";
import { Suspense, useEffect, useRef, useState } from "react";
import Link from "next/link";
import { usePathname, useRouter, useSearchParams } from "next/navigation";
import { listAgentSessions } from "@/api/agent";
import { useOptionalSession } from "@/components/auth/session";
import { Button } from "@/components/ui/button";
import { SidebarMenu } from "@/components/ui/sidebar";
import {
  SidebarEmpty,
  SidebarHistoryLoading,
} from "@/components/layout/sidebar-empty";
import { useSpace } from "@/components/spaces/space-context";
import { ConversationHistoryItem } from "./conversation-history-actions";
import { readErrorMessage } from "@/lib/read-error";
import { useConversationContext } from "./conversation-context";
import { useConversationScope } from "./use-conversation-scope";
import { isProjectID } from "@/components/projects/project";

export function ConversationHistory(props: { onNavigate: () => void }) {
  return (
    <Suspense fallback={<SidebarHistoryLoading message="正在读取对话…" />}>
      <ScopedHistory {...props} />
    </Suspense>
  );
}
function ScopedHistory({ onNavigate }: { onNavigate: () => void }) {
  const memory = useConversationContext(),
    session = useOptionalSession(),
    path = usePathname(),
    router = useRouter(),
    space = useSpace(),
    [refresh, setRefresh] = useState(0);
  const requested = useSearchParams()?.get("projectId");
  const legacy =
    path?.startsWith("/conversations") && isProjectID(requested)
      ? requested
      : undefined;
  const scope = useConversationScope(legacy);
  const pid = scope.data?.id;
  const sessionId = session?.session_id;
  const identity = JSON.stringify([
    session?.actor.id,
    sessionId,
    session?.actor.revision,
    session?.actor.platform_role,
    space.selected?.id,
    space.selected?.role,
    space.selected?.writable,
    pid,
  ]);
  const [view, setView] = useState<{
    identity: string;
    items?: API.SessionView[];
    cursor?: string;
    loading: boolean;
    error: string;
  }>({ identity: "", loading: false, error: "" });
  const generation = useRef<AbortController | null>(null);
  const location = useRef({ identity, path });
  useEffect(() => {
    location.current = { identity, path };
  }, [identity, path]);
  const current =
    view.identity === identity ? view : { identity, loading: !!pid, error: "" };
  useEffect(() => {
    const controller = new AbortController();
    generation.current = controller;
    if (!pid || !sessionId) return () => controller.abort();
    setView((v) => ({
      ...(v.identity === identity ? v : {}),
      identity,
      loading: true,
      error: "",
    }));
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
          setView((v) => ({
            identity,
            ...(v.identity === identity ? v : {}),
            loading: false,
            error: readErrorMessage(error),
          }));
      });
    return () => controller.abort();
  }, [identity, pid, sessionId, memory?.listRevision, refresh]);
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
              [...(v.items ?? []), ...page.items].map((s) => [s.id, s]),
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
  if (!session) return <SidebarEmpty>登录后查看最近对话</SidebarEmpty>;
  if (!space.selected) return null;
  if (scope.error)
    return (
      <div className="px-2 text-xs">
        <p>{readErrorMessage(scope.error)}</p>
        <Button size="sm" variant="ghost" onClick={() => void scope.mutate()}>
          重试读取
        </Button>
      </div>
    );
  if (!pid)
    return (
      <SidebarMenu>
        <SidebarHistoryLoading message="正在读取对话…" />
      </SidebarMenu>
    );
  return (
    <>
      <SidebarMenu>
        {current.items?.map((item) => (
          <ConversationHistoryItem
            key={`${identity}/${item.id}`}
            item={item}
            projectID={pid}
            independent={!legacy}
            active={path === `/conversations/${item.id}`}
            writable={!!space.selected?.writable}
            onNavigate={onNavigate}
            onChange={(action, updated) => {
              if (location.current.identity !== identity) return;
              setView((v) =>
                v.identity === identity
                  ? {
                      ...v,
                      cursor: undefined,
                      items:
                        action === "delete"
                          ? v.items?.filter((row) => row.id !== item.id)
                          : v.items?.map((row) =>
                              row.id === item.id ? { ...row, ...updated } : row,
                            ),
                    }
                  : v,
              );
              memory?.changed();
              if (
                action === "delete" &&
                location.current.path === `/conversations/${item.id}`
              )
                router.replace(
                  legacy ? `/conversations?projectId=${pid}` : "/conversations",
                );
            }}
          />
        ))}
        {current.loading && !current.items ? (
          <SidebarHistoryLoading message="正在读取对话…" />
        ) : null}
      </SidebarMenu>
      {current.loading && current.items ? (
        <span role="status" className="sr-only">
          正在读取对话…
        </span>
      ) : null}
      {current.error ? (
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
      ) : current.items?.length === 0 ? (
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
  return (
    <Button asChild variant="navigation" size="icon-xs" className={className}>
      <Link href="/conversations" aria-label="新建对话" onClick={onNavigate}>
        {children}
      </Link>
    </Button>
  );
}
