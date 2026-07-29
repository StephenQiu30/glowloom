"use client";

import { useSyncExternalStore } from "react";

import { hasAccessToken, subscribeAuthSession } from "@/lib/auth-session";

const getServerSnapshot = () => false;

export function useHasAccessToken(): boolean {
  return useSyncExternalStore(
    subscribeAuthSession,
    hasAccessToken,
    getServerSnapshot,
  );
}
