"use client";
import { type ReactNode, useEffect, useState } from "react";
import { Provider } from "react-redux";
import { useTheme } from "next-themes";
import { makeAppStore, themeActions } from "./runtime/store/app-store";
import { AppFeedbackProvider } from "./runtime/components/feedback/app-feedback-provider";
import { getSession } from "@/api/identity";
import { revokeSession, useOptionalSession } from "@/components/auth/session";
import { CanvasAccountContext } from "./runtime/components/account/account-provider";

export function CanvasProviders({ children }: { children: ReactNode }) {
  const { resolvedTheme } = useTheme();
  const session = useOptionalSession();
  const [store] = useState(() =>
    makeAppStore(resolvedTheme === "dark" ? "dark" : "light"),
  );
  useEffect(() => {
    if (resolvedTheme === "light" || resolvedTheme === "dark") {
      store.dispatch(themeActions.setTheme(resolvedTheme));
    }
  }, [resolvedTheme, store]);
  return (
    <Provider store={store}>
      <CanvasAccountContext.Provider
        value={{
          account: session
            ? { id: session.actor.id, userName: session.actor.display_name }
            : null,
          ready: !!session,
          refresh: async () => {
            await getSession();
            return undefined;
          },
          adopt: () => {
            throw new Error("请通过当前项目登录入口更新会话");
          },
          leave: async () => {
            if (session) await revokeSession(session);
          },
          signOut: async () => {
            if (session) await revokeSession(session);
          },
          signingOut: false,
        }}
      >
        <AppFeedbackProvider>{children}</AppFeedbackProvider>
      </CanvasAccountContext.Provider>
    </Provider>
  );
}
