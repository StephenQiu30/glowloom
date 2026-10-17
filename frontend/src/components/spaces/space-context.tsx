"use client";
import { FeedbackToast } from "@/components/feedback/feedback-toast";
import {
  Empty,
  EmptyHeader,
  EmptyTitle,
  EmptyContent,
} from "@/components/ui/empty";
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
  NativeSelect,
  NativeSelectOption,
} from "@/components/ui/native-select";
const SpaceContext = createContext<{
  spaces: API.SpaceView[];
  selected: API.SpaceView | null;
  pending: boolean;
  refresh: () => Promise<void>;
  select: (id: string) => Promise<void>;
}>({
  spaces: [],
  selected: null,
  pending: false,
  refresh: async () => {},
  select: async () => {},
});
export const useSpace = () => useContext(SpaceContext);
let remembered: { actor: string; id: string } | null = null;
export function SpaceProvider({ children }: { children: ReactNode }) {
  const session = useOptionalSession();
  const [spaces, setSpaces] = useState<API.SpaceView[]>([]);
  const [selected, setSelected] = useState<API.SpaceView | null>(null);
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
        next.id !== getCreationScope() ||
        (selected &&
          (selected.role !== next.role || selected.writable !== next.writable))
      ) {
        setCreationScope(null);
        await clearPrivateResources();
        if (controller.signal.aborted) return;
        setCreationScope(next.id);
        setGeneration((n) => n + 1);
      }
      remembered = { actor: session.actor.id, id: next.id };
      setSpaces(result.items);
      setSelected(next);
    } catch (e) {
      if (!controller.signal.aborted) setError(authErrorMessage(e));
    } finally {
      locked.current = false;
      if (!controller.signal.aborted) {
        setScopeSwitching(false);
        setPending(false);
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
        spaces,
        selected,
        pending,
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
        {session && !selected ? (
          error && !pending ? (
            <Empty>
              <EmptyHeader>
                <EmptyTitle>暂时无法打开空间</EmptyTitle>
              </EmptyHeader>
              <EmptyContent>
                <Button variant="outline" onClick={() => void load()}>
                  重试
                </Button>
              </EmptyContent>
            </Empty>
          ) : (
            <p role="status" className="p-6">
              正在读取空间…
            </p>
          )
        ) : (
          children
        )}
      </div>
    </SpaceContext.Provider>
  );
}
export function SpaceSwitcher() {
  const { spaces, selected, pending, select } = useSpace();
  const session = useOptionalSession();
  if (!session) return null;
  return (
    <div className="flex min-w-0 flex-wrap items-center gap-2">
      <NativeSelect
        className="max-w-48 truncate"
        aria-label="当前创作空间"
        disabled={pending || !selected}
        value={selected?.id ?? ""}
        onChange={(e) => void select(e.target.value)}
      >
        {!selected ? (
          <NativeSelectOption value="">正在读取空间</NativeSelectOption>
        ) : null}
        {spaces.map((s) => (
          <NativeSelectOption key={s.id} value={s.id}>
            {s.name}
            {!s.writable ? "（只读）" : ""}
          </NativeSelectOption>
        ))}
      </NativeSelect>
      <Link href="/teams" className="text-sm underline">
        团队
      </Link>
      <Link href="/invitations" className="text-sm underline">
        邀请与授权
      </Link>
    </div>
  );
}
