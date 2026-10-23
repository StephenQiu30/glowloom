"use client";

import dynamic from "next/dynamic";
import {
  createContext,
  useContext,
  useRef,
  useState,
  type ReactNode,
} from "react";
import { useAccount } from "../account-provider";

const AuthDialog = dynamic(
  () => import("./auth-dialog").then((m) => m.AuthDialog),
  { ssr: false },
);
export type AuthMode = "login" | "register";
const Context = createContext<((mode?: AuthMode) => void) | null>(null);

export function AuthDialogProvider({ children }: { children: ReactNode }) {
  const auth = useAccount();
  const [mode, setMode] = useState<AuthMode | null>(null);
  const [accountId, setAccountId] = useState(auth.account?.id);
  if (accountId !== auth.account?.id) {
    setAccountId(auth.account?.id);
    setMode(null);
  }
  const trigger = useRef<HTMLElement | null>(null);

  function open(mode: AuthMode = "login") {
    const active = document.activeElement;
    trigger.current = active instanceof HTMLElement ? active : null;
    setMode(mode);
  }
  function close() {
    setMode(null);
  }
  return (
    <Context.Provider value={open}>
      {children}
      {mode && !auth.account && (
        <AuthDialog
          mode={mode}
          onModeChange={setMode}
          onClose={close}
          onCloseAutoFocus={(event) => {
            event.preventDefault();
            if (trigger.current?.isConnected) trigger.current.focus();
            else document.getElementById("app-main-content")?.focus();
          }}
          onAuthenticated={(session) => {
            close();
            auth.adopt(session);
          }}
        />
      )}
    </Context.Provider>
  );
}

export function useAuthDialog() {
  const value = useContext(Context);
  if (!value) throw new Error("AuthDialogProvider missing");
  return value;
}
