"use client";

import { useState } from "react";
import { Bell } from "lucide-react";
import { AccountToolbarItems } from "@/components/account/account-toolbar-items";
import { AccountLogoutDialog } from "@/components/account/account-logout-dialog";
import { useOptionalSession } from "@/components/auth/session";
import { CreditWalletSummary } from "@/components/billing/credit-popover";
import { useCreditWallet } from "@/components/billing/use-credit-wallet";
import { useNotificationCenter } from "@/components/notifications/notification-provider";
import { DropdownMenuItem } from "@/components/ui/dropdown-menu";

export function CanvasAccountControls({
  scopeID,
  compact,
  canNavigate,
  onHome,
}: {
  scopeID: string;
  compact: boolean;
  canNavigate: boolean;
  onHome: () => void;
}) {
  const session = useOptionalSession();
  const [logout, setLogout] = useState(false);

  if (!session) return null;

  return (
    <>
      <AccountToolbarItems
        actor={session.actor}
        canvasScopeID={scopeID}
        onLogin={onHome}
        compact={compact}
        canNavigate={canNavigate}
        utilityItems={
          compact ? <CanvasAccountUtilities scopeID={scopeID} /> : undefined
        }
        onLogout={() => setLogout(true)}
      />
      <AccountLogoutDialog
        open={logout}
        onOpenChange={setLogout}
        disabled={!canNavigate}
      />
    </>
  );
}

function CanvasAccountUtilities({ scopeID }: { scopeID: string }) {
  const center = useNotificationCenter();
  const { data, isLoading, error } = useCreditWallet(
    undefined,
    true,
    undefined,
    scopeID,
  );
  const unread = center?.summary?.unread_count;
  return (
    <>
      <DropdownMenuItem
        className="min-h-10 gap-2 px-2 max-md:min-h-11"
        disabled={!center?.signedIn}
        onSelect={() => center?.setInboxOpen(true)}
      >
        <Bell />
        消息中心
        {unread ? (
          <span className="ml-auto text-xs text-muted-foreground">
            {unread} 条未读
          </span>
        ) : null}
      </DropdownMenuItem>
      <div className="px-2 py-1.5">
        <CreditWalletSummary
          wallet={data}
          isLoading={isLoading}
          error={error}
        />
      </div>
    </>
  );
}
