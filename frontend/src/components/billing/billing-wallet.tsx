"use client";

import { useState } from "react";
import { useOptionalSession } from "@/components/auth/session";
import { useSpace } from "@/components/spaces/space-context";
import { BillingView } from "./billing-view";
import { useCreditWallet } from "./use-credit-wallet";

export function BillingWallet({
  section,
}: {
  section: "overview" | "transactions";
}) {
  const session = useOptionalSession();
  const { selected, pending } = useSpace();
  if (pending) return <BillingView section={section} walletLoading />;
  if (!selected?.id || !session) return <BillingView section={section} />;
  const cacheScope = `${session.session_id}:${selected.id}:${selected.role}`;
  return <WalletQuery key={cacheScope} section={section} />;
}

function WalletQuery({ section }: { section: "overview" | "transactions" }) {
  const [cursors, setCursors] = useState<string[]>([]);
  const cursor = cursors.at(-1);
  const query = useCreditWallet(cursor);
  const next = query.data?.next_cursor;
  return (
    <BillingView
      section={section}
      wallet={query.data}
      walletLoading={query.isLoading}
      walletError={Boolean(query.error)}
      onNext={next ? () => setCursors([...cursors, next]) : undefined}
      onPrevious={cursor ? () => setCursors(cursors.slice(0, -1)) : undefined}
    />
  );
}
