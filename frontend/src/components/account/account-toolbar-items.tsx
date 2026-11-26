"use client";

import type { ReactNode } from "react";
import { CreditPopover } from "@/components/billing/credit-popover";
import { ThemeToggle } from "@/components/controls/theme-controls";
import { NotificationBell } from "@/components/notifications/notification-bell";
import { Button } from "@/components/ui/button";
import { Separator } from "@/components/ui/separator";
import { AccountMenu } from "./account-menu";

export function AccountToolbarItems({
  actor,
  onLogin,
  onLogout,
  canvasScopeID,
  compact = false,
  canNavigate = true,
  utilityItems,
}: {
  actor?: API.EmailSessionView["actor"];
  onLogin: () => void;
  onLogout: () => void;
  canvasScopeID?: string;
  compact?: boolean;
  canNavigate?: boolean;
  utilityItems?: ReactNode;
}) {
  return (
    <>
      {!actor && <ThemeToggle toolbar />}
      {!compact && <NotificationBell onLogin={onLogin} />}
      {actor && !compact && (
        <>
          <Separator
            orientation="vertical"
            className="account-toolbar-divider"
          />
          <CreditPopover
            navigation={!canvasScopeID}
            showBalance
            canvasScopeID={canvasScopeID}
            canNavigate={canNavigate}
          />
        </>
      )}
      {actor ? (
        <AccountMenu
          actor={actor}
          compact={compact || undefined}
          navigationDisabled={!canNavigate}
          utilityItems={utilityItems}
          onLogout={onLogout}
        />
      ) : (
        <Button size="toolbar" onClick={onLogin}>
          登录
        </Button>
      )}
    </>
  );
}
