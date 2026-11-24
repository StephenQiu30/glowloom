"use client";

import { DEFAULT_PAGE_SIZE } from "@/lib/pagination";
import useSWR from "swr";
import { getCreditWallet } from "@/api/billing";
import { useOptionalSession } from "@/components/auth/session";
import { useSpace } from "@/components/spaces/space-context";

export function useCreditWallet(
  cursor?: string,
  enabled = true,
  limit: number = DEFAULT_PAGE_SIZE,
) {
  const session = useOptionalSession();
  const { selected, pending } = useSpace();
  const key =
    enabled && !pending && session && selected?.id
      ? [
          "credit-wallet",
          session.session_id,
          selected.id,
          selected.role,
          cursor,
          limit,
        ]
      : null;
  return useSWR(key, () => getCreditWallet({ cursor, limit }), {
    errorRetryCount: 0,
  });
}
