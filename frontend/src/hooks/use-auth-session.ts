"use client";

import { useSyncExternalStore } from "react";
import { useEffect, useState } from "react";

import { hasAccessToken, subscribeAuthSession } from "@/lib/auth-session";
import { refreshAccessToken } from "@/lib/request";

export type AuthSessionState = "checking" | "authenticated" | "anonymous";

const getServerSnapshot = (): AuthSessionState => "checking";
const isTestEnvironment = process.env.NODE_ENV === "test";

function getBrowserSnapshot(): AuthSessionState {
  return hasAccessToken() ? "authenticated" : "anonymous";
}

export function useAuthSessionState(): AuthSessionState {
  const token = useSyncExternalStore(
    subscribeAuthSession,
    getBrowserSnapshot,
    getServerSnapshot,
  );
  const [restoring, setRestoring] = useState(
    () => !isTestEnvironment && !hasAccessToken(),
  );

  useEffect(() => {
    if (!restoring || token === "authenticated") return;
    let active = true;
    void refreshAccessToken().finally(() => {
      if (active) setRestoring(false);
    });
    return () => {
      active = false;
    };
  }, [restoring, token]);

  if (restoring && token !== "authenticated") return "checking";
  return token;
}
