"use client";

import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from "react";
import { SWRConfig } from "swr";
import useSWR from "@/components/canvas/runtime/lib/use-live-query";
import { getMyNotificationSummary } from "@/components/canvas/runtime/api/notification-bff";
import { useAccount } from "@/components/canvas/runtime/components/account/account-provider";
import { accountScope } from "@/components/canvas/runtime/store/account-slice";
import { unwrap } from "@/components/canvas/runtime/lib/request";
import { reconcileMailbox } from "./lib/notification-state";

type Snapshot = Pick<
  CanvasContract.NotificationSummaryVO,
  "dataEpoch" | "mailboxVersion" | "maxInboxSeq" | "unreadCount"
>;
type InboxContext = {
  scope: string;
  summary?: CanvasContract.NotificationSummaryVO;
  error?: Error;
  loading: boolean;
  readTick: number;
  refresh: () => Promise<unknown>;
  observe: (incoming: Snapshot, requestedEpoch?: string) => boolean;
};
const Context = createContext<InboxContext | null>(null);

/** One account-scoped authority for versioned summaries; no second event stream. */
export function NotificationProvider({ children }: { children: ReactNode }) {
  const { account, signingOut } = useAccount();
  const scope = accountScope(account);
  const identity = useMemo(() => ({ scope, cache: new Map() }), [scope]);
  const cache = identity.cache;
  const [session, setSession] = useState<InboxContext>();
  const enabled = !!account?.id && !signingOut;
  const current = enabled && session?.scope === scope ? session : undefined;
  return (
    <Context.Provider
      value={{
        scope,
        summary: current?.summary,
        error: current?.error,
        loading: current?.loading ?? enabled,
        readTick: current?.readTick ?? 0,
        refresh: current?.refresh ?? (async () => {}),
        observe: current?.observe ?? (() => false),
      }}
    >
      <SWRConfig key={scope} value={{ provider: () => cache }}>
        <NotificationSession
          scope={scope}
          enabled={enabled}
          onValue={setSession}
        />
      </SWRConfig>
      {children}
    </Context.Provider>
  );
}

function NotificationSession({
  scope,
  enabled,
  onValue,
}: {
  scope: string;
  enabled: boolean;
  onValue: (value: InboxContext) => void;
}) {
  const [requests] = useState(() => new Set<AbortController>());
  const [readTick, setReadTick] = useState(0);
  const accepted = useRef<CanvasContract.NotificationSummaryVO | undefined>(
    undefined,
  );
  const summary = useSWR(
    enabled ? ["notification-summary", scope] : null,
    async (_key, options) => {
      const epoch = accepted.current?.dataEpoch;
      const controller = new AbortController();
      requests.add(controller);
      try {
        const incoming = unwrap(
          await getMyNotificationSummary({
            ...options,
            signal: AbortSignal.any([
              ...(options.signal instanceof AbortSignal
                ? [options.signal]
                : []),
              controller.signal,
            ]),
            timeout: 10000,
          }),
        );
        const next = reconcileMailbox(accepted.current, incoming, epoch);
        accepted.current = next;
        return next;
      } finally {
        requests.delete(controller);
      }
    },
    {
      onSuccess: (next) => {
        accepted.current = next;
        setReadTick((value) => value + 1);
      },
      shouldRetryOnError: false,

      refreshWhenHidden: false,
      refreshWhenOffline: false,
      revalidateOnFocus: true,
      revalidateOnReconnect: true,
      focusThrottleInterval: 5000,
      dedupingInterval: 1000,
    },
  );
  const { mutate } = summary;
  useEffect(
    () => () => {
      requests.forEach((request) => request.abort());
      requests.clear();
    },
    [requests],
  );
  const observe = useCallback(
    (incoming: Snapshot, epoch?: string) => {
      const current = accepted.current;
      const candidate: CanvasContract.NotificationSummaryVO = {
        ...current,
        ...incoming,
      };
      const next = reconcileMailbox(current, candidate, epoch);
      if (next === current) return false;
      accepted.current = next;
      void mutate(next, { revalidate: false });
      return true;
    },
    [mutate],
  );
  const refresh = useCallback(() => mutate(), [mutate]);
  const value = useMemo(
    () => ({
      scope,
      summary: enabled ? summary.data : undefined,
      error: summary.error,
      loading: enabled && summary.isLoading,
      readTick,
      refresh,
      observe,
    }),
    [
      scope,
      enabled,
      summary.data,
      summary.error,
      summary.isLoading,
      refresh,
      observe,
      readTick,
    ],
  );
  useEffect(() => onValue(value), [onValue, value]);
  return null;
}

export function useNotifications() {
  const context = useContext(Context);
  if (!context) throw new Error("NotificationProvider is required");
  return context;
}

export function useOptionalNotifications() {
  return useContext(Context);
}
