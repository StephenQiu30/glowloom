"use client";

import {
  createContext,
  useContext,
  useMemo,
  useState,
  type ReactNode,
} from "react";
import { SWRConfig, type Cache } from "swr";
import { useAccount } from "../account-provider";
import { accountScope } from "@/components/canvas/runtime/store/account-slice";

/** Identity changes rebuild private content and its cache, preserving public drafts. */
const Context = createContext<string | null>(null);
const CacheContext = createContext<{ scope: string; cache: Cache } | null>(
  null,
);

export function PrivateAccountCacheProvider({
  children,
}: {
  children: ReactNode;
}) {
  const { account } = useAccount();
  const scope = accountScope(account);
  const value = useMemo(() => ({ scope, cache: new Map() }), [scope]);
  return (
    <CacheContext.Provider value={value}>
      {/* Own SWR's cache lifecycle while old and new route boundaries overlap. */}
      <SWRConfig key={scope} value={{ provider: () => value.cache }} />
      {children}
    </CacheContext.Provider>
  );
}

export function PrivateAccountBoundary({ children }: { children: ReactNode }) {
  const { account } = useAccount();
  const parentScope = useContext(Context);
  const shared = useContext(CacheContext);
  const scope = accountScope(account);
  if (parentScope === scope) return children;
  return (
    <PrivateCache
      key={scope}
      scope={scope}
      cache={shared?.scope === scope ? shared.cache : undefined}
    >
      {children}
    </PrivateCache>
  );
}

function PrivateCache({
  children,
  scope,
  cache: sharedCache,
}: {
  children: ReactNode;
  scope: string;
  cache?: Cache;
}) {
  const [localCache] = useState(() => new Map());
  const cache = sharedCache ?? localCache;
  return (
    <Context.Provider value={scope}>
      <SWRConfig value={{ provider: () => cache }}>{children}</SWRConfig>
    </Context.Provider>
  );
}
