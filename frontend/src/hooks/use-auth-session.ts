"use client";

import { useSyncExternalStore } from "react";

import { hasAccessToken, subscribeAuthSession } from "@/lib/auth-session";

export type AuthSessionState = "checking" | "authenticated" | "anonymous";

const getServerSnapshot = (): AuthSessionState => "checking";

function getBrowserSnapshot(): AuthSessionState {
  return hasAccessToken() ? "authenticated" : "anonymous";
}

export function useAuthSessionState(): AuthSessionState {
  return useSyncExternalStore(
    subscribeAuthSession,
    getBrowserSnapshot,
    getServerSnapshot,
  );
}
