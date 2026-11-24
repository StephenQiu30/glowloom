"use client";

import { DEFAULT_PAGE_SIZE } from "@/lib/pagination";
import { PageSizeSelect } from "@/components/ui/page-size-select";
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
  const [pageSize, setPageSize] = useState<number>(DEFAULT_PAGE_SIZE);
  const [cursors, setCursors] = useState<string[]>([]);
  const cursor = cursors.at(-1);
  const query = useCreditWallet(cursor, true, pageSize);
  const next = query.data?.next_cursor;
  return (
    <BillingView
      section={section}
      wallet={query.data}
      walletLoading={query.isLoading || (!query.data && query.isValidating)}
      walletError={Boolean(query.error)}
      sizeControl={
        <PageSizeSelect
          label="积分流水每页数量"
          value={pageSize}
          onChange={(size) => {
            setCursors([]);
            setPageSize(size);
          }}
        />
      }
      onNext={next ? () => setCursors([...cursors, next]) : undefined}
      onPrevious={cursor ? () => setCursors(cursors.slice(0, -1)) : undefined}
      onRetry={() => void query.mutate()}
    />
  );
}
