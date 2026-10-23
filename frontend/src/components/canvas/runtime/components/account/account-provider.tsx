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
import * as api from "@/components/canvas/runtime/api/accountBffController";
import {
  ApiError,
  setCsrfToken,
  unwrap,
} from "@/components/canvas/runtime/lib/request";
import { notifyError } from "@/components/canvas/runtime/components/feedback/error-notification";
import { subscribeAccountEvents } from "./lib/account-events";
import {
  useAppDispatch,
  useAppSelector,
  useAppStore,
} from "@/components/canvas/runtime/store/hooks";
import { accountActions } from "@/components/canvas/runtime/store/account-slice";
import {
  AlertDialog,
  AlertDialogContent,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogCancel,
  AlertDialogAction,
} from "@/components/ui/alert-dialog";

type AccountContextValue = {
  account: CanvasContract.AccountVO | null;
  ready: boolean;
  error?: Error;
  refresh: () => Promise<CanvasContract.BrowserSessionVO | undefined>;
  adopt: (session: CanvasContract.BrowserSessionVO) => void;
  leave: () => Promise<void>;
  signOut: () => Promise<void>;
  signingOut: boolean;
};
export const CanvasAccountContext = createContext<AccountContextValue | null>(
  null,
);

export function AccountProvider({ children }: { children: ReactNode }) {
  const {
    account,
    ready,
    error: message,
    signingOut,
  } = useAppSelector((state) => state.account);
  const error = useMemo(
    () => (message ? new Error(message) : undefined),
    [message],
  );
  const dispatch = useAppDispatch();
  const store = useAppStore();
  const logoutRequest = useRef<Promise<void> | null>(null);
  const [discardPrompt, setDiscardPrompt] = useState(false);
  const discardAnswer = useRef<((confirmed: boolean) => void) | null>(null);
  const answerDiscard = (confirmed: boolean) => {
    setDiscardPrompt(false);
    discardAnswer.current?.(confirmed);
    discardAnswer.current = null;
  };
  const sessionReads = useRef(new Set<ReturnType<typeof api.browserSession>>());
  const guestRead = useRef<ReturnType<typeof api.browserSession> | null>(null);
  const epoch = useRef(0);
  const channel = useRef<BroadcastChannel | null>(null);
  const apply = useCallback(
    (session: CanvasContract.BrowserSessionVO) => {
      setCsrfToken(session.csrfToken);
      dispatch(accountActions.accepted(session.account ?? null));
    },
    [dispatch],
  );
  const read = useCallback(
    async (silent = false) => {
      if (silent && store.getState().account.signingOut) return;
      const current = ++epoch.current;
      let request = guestRead.current;
      if (store.getState().account.account || !request) {
        const pending = api.browserSession().finally(() => {
          sessionReads.current.delete(pending);
          if (guestRead.current === pending) guestRead.current = null;
        });
        if (!store.getState().account.account) guestRead.current = pending;
        sessionReads.current.add(pending);
        request = pending;
      }
      try {
        const session = unwrap(await request);
        if (current !== epoch.current) return;
        apply(session);
        return session;
      } catch (cause) {
        if (current !== epoch.current) return;
        if (cause instanceof ApiError && Math.floor(cause.code / 100) === 401) {
          dispatch(accountActions.accepted(null));
          setCsrfToken();
        } else if (
          !store.getState().account.account &&
          (!silent || !store.getState().account.ready)
        ) {
          notifyError(cause, "账户操作未完成");
          dispatch(
            accountActions.failed(
              cause instanceof Error ? cause.message : "账户读取失败",
            ),
          );
        }
      }
    },
    [apply, dispatch, store],
  );
  const refresh = useCallback(() => read(), [read]);
  const refreshSilently = useCallback(() => read(true), [read]);
  const invalidate = useCallback(() => {
    ++epoch.current;
    guestRead.current = null;
  }, []);
  const adopt = useCallback(
    (session: CanvasContract.BrowserSessionVO) => {
      invalidate();
      apply(session);
      channel.current?.postMessage("changed");
    },
    [apply, invalidate],
  );
  const leave = useCallback(async () => {
    invalidate();
    dispatch(accountActions.accepted(null));
    setCsrfToken();
    channel.current?.postMessage("changed");
    await refresh();
  }, [refresh, dispatch, invalidate]);
  const signOut = useCallback(() => {
    if (logoutRequest.current) return logoutRequest.current;
    logoutRequest.current = (async () => {
      try {
        const detail = { pending: false, discard: [] as Array<() => void> };
        window.dispatchEvent(
          new CustomEvent("glowloom-before-signout", { detail }),
        );
        const actorKey = store.getState().account.account
          ? `USER:${store.getState().account.account!.id}`
          : null;
        let log:
          | typeof import("@glowloom/canvas-collab/journal").collaborationJournal
          | null = null;
        let count = 0;
        let cleanupUnavailable = false;
        try {
          if (typeof indexedDB !== "undefined" && actorKey) {
            log = (await import("@glowloom/canvas-collab/journal"))
              .collaborationJournal;
            count = await log.countActor(window.location.origin, actorKey);
          }
        } catch {
          // Account logout must remain usable when the browser denies device storage.
          // Unknown records stay isolated under their original actor; never silently delete them.
          cleanupUnavailable = true;
          log = null;
        }
        if (detail.pending || count > 0) {
          setDiscardPrompt(true);
          const confirmed = await new Promise<boolean>((resolve) => {
            discardAnswer.current = resolve;
          });
          if (!confirmed) throw new Error("已取消退出，草稿继续保留");
        }
        dispatch(accountActions.signingOut(true));
        ++epoch.current;
        for (const discard of detail.discard) discard();
        if (log && actorKey) {
          try {
            await log.removeActor(window.location.origin, actorKey);
          } catch {
            cleanupUnavailable = true;
          }
        }
        // 等待响应 Cookie 落定；取消读取不能撤回服务器的 Set-Cookie。
        await Promise.allSettled([...sessionReads.current]);
        unwrap(await api.logoutAccount());
        await leave();
        if (cleanupUnavailable)
          notifyError(
            new Error(
              "设备存储不可用，恢复记录清理未完成。残留记录仍归原账户，其他账户无法接管。",
            ),
            "已退出登录，设备清理未完成",
          );
      } finally {
        logoutRequest.current = null;
        dispatch(accountActions.signingOut(false));
      }
    })();
    return logoutRequest.current;
  }, [leave, dispatch, store]);
  useEffect(() => {
    channel.current = new BroadcastChannel("glowloom-account");
    channel.current.onmessage = () => {
      if (store.getState().account.signingOut) return;
      invalidate();
      void refreshSilently();
    };
    const focus = () => {
      void refreshSilently();
    };
    let activeAt = Date.now();
    const activity = () => {
      activeAt = Date.now();
    };
    const visible = () => {
      if (document.visibilityState === "visible") focus();
    };
    const expired = () => {
      if (store.getState().account.signingOut) return;
      invalidate();
      dispatch(accountActions.accepted(null));
      setCsrfToken();
      void refreshSilently();
    };
    window.addEventListener("focus", focus);
    window.addEventListener("online", focus);
    window.addEventListener("pointerdown", activity, { passive: true });
    window.addEventListener("keydown", activity);
    window.addEventListener("glowloom:session-expired", expired);
    document.addEventListener("visibilitychange", visible);
    const renewal = window.setInterval(() => {
      if (
        document.visibilityState === "visible" &&
        Date.now() - activeAt < 300_000 &&
        store.getState().account.account
      )
        void refreshSilently();
    }, 60_000);
    const startup = window.setTimeout(() => {
      void refresh();
    }, 0);
    const connection = channel.current;
    return () => {
      invalidate();
      window.removeEventListener("focus", focus);
      window.removeEventListener("online", focus);
      window.removeEventListener("pointerdown", activity);
      window.removeEventListener("keydown", activity);
      window.clearInterval(renewal);
      window.removeEventListener("glowloom:session-expired", expired);
      document.removeEventListener("visibilitychange", visible);
      window.clearTimeout(startup);
      connection.close();
    };
  }, [refresh, refreshSilently, invalidate, dispatch, store]);
  const accountId = signingOut ? undefined : account?.id;
  useEffect(() => {
    if (accountId === undefined) return;
    return subscribeAccountEvents({
      ready: () => {
        void refreshSilently();
        window.dispatchEvent(new Event("glowloom:notification:changed"));
      },
      changed: () => {
        invalidate();
        void refreshSilently();
      },
      notificationChanged: () => {
        window.dispatchEvent(new Event("glowloom:notification:changed"));
      },
    });
  }, [accountId, refreshSilently, invalidate]);
  return (
    <CanvasAccountContext.Provider
      value={{
        account,
        ready,
        error,
        refresh,
        adopt,
        leave,
        signOut,
        signingOut,
      }}
    >
      {children}
      <AlertDialog
        open={discardPrompt}
        onOpenChange={(open) => {
          if (!open) answerDiscard(false);
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>放弃设备草稿并退出？</AlertDialogTitle>
            <AlertDialogDescription>
              放弃此账户在本设备的未同步草稿、原命令和未应用参数，并尝试清理其他画布页面的恢复记录。已提交的服务端操作不会撤销；设备存储不可用时会保留隔离记录并提示清理未完成。
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel onClick={() => answerDiscard(false)}>
              继续编辑
            </AlertDialogCancel>
            <AlertDialogAction onClick={() => answerDiscard(true)}>
              放弃设备草稿并退出
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </CanvasAccountContext.Provider>
  );
}
export function useAccount() {
  const value = useContext(CanvasAccountContext);
  if (!value) throw new Error("AccountProvider missing");
  return value;
}
