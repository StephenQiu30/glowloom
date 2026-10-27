"use client";
import { FeedbackToast } from "@/components/feedback/feedback-toast";
import { PageLoading } from "@/components/layout/loading-state";
import { ContentState } from "@/components/layout/content-state";
import { Button } from "@/components/ui/button";
import {
  createContext,
  useContext,
  useEffect,
  useEffectEvent,
  useRef,
  useState,
  type ReactNode,
} from "react";
import Link from "next/link";
import { listMySpaces } from "@/api/identity";
import {
  useOptionalSession,
  clearPrivateResources,
} from "@/components/auth/session";
import { authErrorMessage } from "@/components/auth/contracts";
import {
  getCreationScope,
  setCreationScope,
  setScopeSwitching,
  subscribeScopeDenied,
} from "@/lib/request";
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
export const SpaceContext = createContext<{
  spaces: API.SpaceView[];
  selected: API.SpaceView | null;
  pending: boolean;
  error: string;
  refresh: () => Promise<void>;
  select: (id: string) => Promise<void>;
}>({
  spaces: [],
  selected: null,
  pending: false,
  error: "",
  refresh: async () => {},
  select: async () => {},
});
export const useSpace = () => useContext(SpaceContext);
let remembered: { actor: string; id: string } | null = null;
export function SpaceProvider({
  children,
  blockUntilReady = true,
}: {
  children: ReactNode;
  blockUntilReady?: boolean;
}) {
  const session = useOptionalSession();
  const [spaces, setSpaces] = useState<API.SpaceView[]>([]);
  const [selected, setSelected] = useState<API.SpaceView | null>(null);
  const [selectedActor, setSelectedActor] = useState<string | null>(null);
  const [pending, setPending] = useState(false);
  const [generation, setGeneration] = useState(0);
  const [notice, setNotice] = useState("");
  const [error, setError] = useState("");
  const flight = useRef<AbortController | null>(null);
  const locked = useRef(false);
  async function load(id?: string) {
    if (!session || locked.current) return;
    locked.current = true;
    setPending(true);
    setError("");
    setScopeSwitching(true);
    const controller = new AbortController();
    flight.current = controller;
    try {
      const result = await listMySpaces({ signal: controller.signal });
      if (controller.signal.aborted) return;
      if (!Array.isArray(result.items))
        throw new Error("空间信息不完整，请刷新重试。");
      const desired =
        id ??
        (remembered?.actor === session.actor.id ? remembered.id : undefined);
      const next =
        result.items.find((s) => s.id === desired) ??
        result.items.find((s) => s.kind === "personal");
      if (!next) throw new Error("暂时无法读取个人空间。");
      if (desired && next.id !== desired)
        setNotice("您已无法访问原空间，已返回个人空间。未重新提交任何创作。");
      if (
        (getCreationScope() !== null && next.id !== getCreationScope()) ||
        (selected &&
          (selected.role !== next.role || selected.writable !== next.writable))
      ) {
        setCreationScope(null);
        await clearPrivateResources();
        if (controller.signal.aborted) return;
        setGeneration((n) => n + 1);
      }
      setCreationScope(next.id);
      remembered = { actor: session.actor.id, id: next.id };
      setSpaces(result.items);
      setSelected(next);
      setSelectedActor(session.actor.id);
    } catch (e) {
      if (!controller.signal.aborted) setError(authErrorMessage(e));
    } finally {
      if (flight.current === controller) {
        locked.current = false;
        if (!controller.signal.aborted) {
          setScopeSwitching(false);
          setPending(false);
        }
      }
    }
  }
  const loadEvent = useEffectEvent(() => load());
  useEffect(() => {
    let active = true;
    queueMicrotask(() => {
      if (active) void loadEvent();
    });
    return () => {
      active = false;
      flight.current?.abort();
      locked.current = false;
      setScopeSwitching(false);
    };
  }, [session?.actor.id]);
  useEffect(
    () =>
      subscribeScopeDenied(() => {
        if (!locked.current) void loadEvent();
      }),
    [],
  );
  return (
    <SpaceContext.Provider
      value={{
        spaces: selectedActor === session?.actor.id ? spaces : [],
        selected: selectedActor === session?.actor.id ? selected : null,
        pending,
        error,
        refresh: () => load(),
        select: (id) => load(id),
      }}
    >
      {notice ? <FeedbackToast message={notice} tone="info" /> : null}
      {error ? (
        <FeedbackToast
          message={error}
          action={{
            label: "重新读取空间",
            onClick: () => {
              setError("");
              void load();
            },
          }}
        />
      ) : null}
      <div
        key={`${session?.actor.id ?? "guest"}:${generation}`}
        className="contents"
      >
        {blockUntilReady &&
        session &&
        (!selected || selectedActor !== session.actor.id) ? (
          error && !pending ? (
            <ContentState
              state="error"
              title="暂时无法打开空间"
              description={error}
              actions={<Button onClick={() => void load()}>重试</Button>}
            />
          ) : (
            <PageLoading message="正在加载…" />
          )
        ) : (
          children
        )}
      </div>
    </SpaceContext.Provider>
  );
}

export function SpaceGate({ children }: { children: ReactNode }) {
  const session = useOptionalSession(),
    space = useSpace();
  if (!session || space.selected) return children;
  if (space.error && !space.pending)
    return (
      <ContentState
        state="error"
        title="暂时无法打开空间"
        description={space.error}
        actions={<Button onClick={() => void space.refresh()}>重试</Button>}
      />
    );
  return <PageLoading message="正在确认创作归属…" />;
}
export function SpaceSwitcher({ compact = false }: { compact?: boolean }) {
  const { spaces, selected, pending, select } = useSpace();
  const session = useOptionalSession();
  if (!session) return null;
  return (
    <div className="flex min-w-0 flex-wrap items-center gap-2">
      <Select
        disabled={pending || !selected}
        value={selected?.id ?? ""}
        onValueChange={(selectedValue) => void select(selectedValue)}
      >
        <SelectTrigger className="max-w-48 truncate" aria-label="当前创作空间">
          <SelectValue placeholder="正在读取空间" />
        </SelectTrigger>
        <SelectContent>
          <SelectGroup>
            {spaces.map((s) => (
              <SelectItem key={s.id} value={s.id}>
                {s.name}
                {!s.writable ? "（只读）" : ""}
              </SelectItem>
            ))}
          </SelectGroup>
        </SelectContent>
      </Select>
      {!compact && (
        <>
          <Link href="/teams" className="text-sm underline">
            团队
          </Link>
          <Link href="/invitations" className="text-sm underline">
            邀请与授权
          </Link>
        </>
      )}
    </div>
  );
}
